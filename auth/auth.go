// Package auth — signing in to the admin panel: a session in a signed cookie,
// CSRF, ready-made /login and /logout handlers and a permission check on a
// route.
//
// All it needs from an application is a Store: where users are kept, how a
// password is stored and how permissions are worked out is the application's
// business, and the library knows nothing about it.
//
//	a := auth.New(db, key, "/articles")
//	a.Mount(mux)                        // GET/POST /login, POST /logout
//	mux.Handle("/", a.Require(private)) // everything else needs a session
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dbond762/gojiffy"
	"github.com/dbond762/gojiffy/view"
)

const (
	sessionCookie = "sid"
	csrfCookie    = "csrf"
	deviceCookie  = "device"

	// what New puts in the Auth fields of the same name
	sessionTTL    = 24 * time.Hour
	deviceTTL     = 90 * 24 * time.Hour
	loginAttempts = 5
	loginWindow   = 15 * time.Minute

	// The paths are fixed: the theme's templates know them (the sign-in form
	// posts to /login, the button in the header to POST /logout), so both ends
	// of that arrangement are held by the library.
	loginPath  = "/login"
	logoutPath = "/logout"
)

// User — what the library knows about whoever signed in: what to sign the
// session with, what to call them in the header and what they are allowed to
// do. An application's own idea of a user stays with the application.
type User struct {
	ID    int
	Name  string
	Perms gojiffy.Perms
	// Session — what a session is signed with along with the id: a cookie
	// signed with another value is refused. It changes through
	// Store.EndSessions, and with it every session of the user is over. It is
	// readable in the cookie, so a counter will do and a secret will not.
	Session string
}

// Store — everything needed from the application.
type Store interface {
	// Authenticate — the id of the user for this login and password. Any error
	// means refusal: "no such login" and "wrong password" produce the same
	// message, so the form cannot be used to enumerate logins. Nor should the
	// time it takes: check the password against some hash even when there is
	// no such login, or the missing bcrypt gives the login away.
	Authenticate(ctx context.Context, login, password string) (int, error)
	// User — the user for the id out of the session cookie. Called on every
	// request, so permissions are always fresh: one taken away is gone at once.
	User(ctx context.Context, id int) (User, error)
	// EndSessions changes the user's Session, ending every session they have:
	// the one signing out and any copy of its cookie stolen before. Signing out
	// calls it; an application calls it too when a password is changed.
	EndSessions(ctx context.Context, id int) error
}

// Auth holds the store and the signing key. Created once, at startup.
type Auth struct {
	store    Store
	key      []byte
	home     string
	attempts limiter // sign-in attempts per login, see limit.go

	// Insecure lets the cookies go over plain HTTP too. By default they are
	// Secure and travel only over HTTPS, whoever terminates it — this process or
	// a proxy in front, which the request alone cannot tell apart from no HTTPS
	// at all. Browsers count http://localhost as secure, so local development
	// needs no Insecure; a panel reached over HTTP across a network does, and
	// gives the session away to anyone listening on it.
	Insecure bool

	// ClientAddr — where a request came from, for the log of signing in and
	// out. Nil means the address of the connection; behind a proxy that is the
	// proxy itself, and only the application knows which proxies to believe.
	ClientAddr func(*http.Request) string

	// SessionTTL — how long a session lasts after signing in; 24 hours by
	// default.
	SessionTTL time.Duration
	// DeviceTTL — how long a browser that has signed in keeps a count of its
	// own at the login, see attemptKey; 90 days by default.
	DeviceTTL time.Duration
	// LoginAttempts tries at one login in LoginWindow, after that it is refused
	// until the window ends; 5 in 15 minutes by default, see limit.go.
	LoginAttempts int
	LoginWindow   time.Duration
}

// New: key signs the session cookie (change the key and every session is
// gone), home is where to send someone after signing in, empty means "/". The
// exported fields hold the defaults; change them before Mount.
func New(store Store, key []byte, home string) *Auth {
	if home == "" {
		home = "/"
	}
	return &Auth{
		store: store, key: key, home: home,
		SessionTTL: sessionTTL, DeviceTTL: deviceTTL,
		LoginAttempts: loginAttempts, LoginWindow: loginWindow,
	}
}

// Mount hangs sign-in and sign-out on the mux. They sit outside Require: there
// is no session yet before signing in, and both check CSRF themselves, so
// where they are mounted makes no difference.
func (a *Auth) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET "+loginPath, a.loginForm)
	mux.HandleFunc("POST "+loginPath, a.login)
	mux.HandleFunc("POST "+logoutPath, a.logout)
}

// Require lets through only a valid session, and checks CSRF on top of that
// for a POST. The user is put in the context — take it out with UserFrom.
func (a *Auth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := a.session(r)
		if err != nil {
			// With no cookie there is nothing to clear, and clearing would take
			// the CSRF cookie of a sign-in form open in another tab along.
			if !errors.Is(err, http.ErrNoCookie) {
				a.clearSession(w)
			}
			http.Redirect(w, r, loginPath, http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost && !CheckCSRF(r) {
			http.Error(w, "csrf token mismatch", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

// Can lets through only someone holding the named permission. Wrap what
// already stands behind Require: without it the context is empty and nobody
// holds anything.
func (a *Auth) Can(perm string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !UserFrom(r.Context()).Perms.Can(perm) {
			http.Error(w, view.T("Access denied"), http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// session — the user whose session the request carries, or why there is none:
// no cookie, a forged or expired one, a user deleted or a database silent — the
// session is over either way — or a session ended since the cookie was signed.
func (a *Auth) session(r *http.Request) (User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return User{}, err
	}
	id, session, err := a.verify(c.Value)
	if err != nil {
		return User{}, err
	}
	u, err := a.store.User(r.Context(), id)
	if err != nil {
		return User{}, err
	}
	if session != u.Session {
		return User{}, errors.New("session ended")
	}
	return u, nil
}

type ctxKey struct{}

// UserFrom — the user who signed in. Outside Require it hands back an empty
// one: no permissions, so Can lets nobody through.
func UserFrom(ctx context.Context) User {
	u, _ := ctx.Value(ctxKey{}).(User)
	return u
}

// CSRF — the token for the hidden form field (view.Page.CSRF, Resource.Form).
func CSRF(r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil {
		return c.Value
	}
	return ""
}

// CheckCSRF — double submit: the hidden form field has to match the cookie.
// Require calls it on every POST; it is needed on its own only for handlers
// standing outside Require.
func CheckCSRF(r *http.Request) bool {
	token := CSRF(r)
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(r.PostFormValue("_csrf"))) == 1
}

func (a *Auth) loginForm(w http.ResponseWriter, r *http.Request) {
	// The CSRF cookie is needed before signing in, or there is nothing to
	// check the sign-in form against.
	token := CSRF(r)
	if token == "" {
		token = hex.EncodeToString(randomBytes(32))
		a.setCookie(w, csrfCookie, token, a.SessionTTL)
	}
	view.Render(w, http.StatusOK, "login.html", loginPage(token, "", ""))
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	if !CheckCSRF(r) {
		http.Error(w, "csrf token mismatch", http.StatusForbidden)
		return
	}
	login := strings.TrimSpace(r.PostFormValue("login"))
	key := a.attemptKey(r, login)

	if ok, wait, first := a.attempts.try(key, a.LoginAttempts, a.LoginWindow); !ok {
		// the rest of the refusals in the window say nothing new
		if first {
			a.logf(r, "login %q locked for %v: too many attempts", clip(login), wait.Round(time.Second))
		}
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		view.Render(w, http.StatusTooManyRequests, "login.html",
			loginPage(CSRF(r), login, view.T("Too many attempts, try again later")))
		return
	}

	id, err := a.store.Authenticate(r.Context(), login, r.PostFormValue("password"))
	if err != nil {
		// the log tells a missing login from a wrong password, the page does not
		a.logf(r, "login %q refused: %v", clip(login), err)
		// One message for both cases — we do not hint whether the login exists.
		view.Render(w, http.StatusUnauthorized, "login.html",
			loginPage(CSRF(r), login, view.T("Wrong login or password")))
		return
	}
	a.attempts.forget(key)

	u, err := a.store.User(r.Context(), id)
	if err != nil {
		log.Printf("auth: user %d: %v", id, err)
		http.Error(w, view.T("internal error"), http.StatusInternalServerError)
		return
	}
	a.setCookie(w, sessionCookie, a.sign(id, u.Session, time.Now().Add(a.SessionTTL)), a.SessionTTL)
	a.setCookie(w, csrfCookie, hex.EncodeToString(randomBytes(32)), a.SessionTTL)
	a.setDevice(w, login)
	a.logf(r, "login %q signed in as user %d", clip(login), id)
	http.Redirect(w, r, a.home, http.StatusSeeOther)
}

// logf logs signing in or out along with the address it came from. A login is
// whatever was typed, so it goes in with %q: a line break in it cannot forge a
// line of its own.
func (a *Auth) logf(r *http.Request, format string, args ...any) {
	addr := r.RemoteAddr
	if a.ClientAddr != nil {
		addr = a.ClientAddr(r)
	}
	log.Printf("auth: %s: "+format, append([]any{addr}, args...)...)
}

// clip keeps a typed login in the log no longer than in the table of attempts.
func clip(login string) string {
	if len(login) > loginKeyMax {
		return login[:loginKeyMax]
	}
	return login
}

// attemptKey — what a sign-in attempt is counted under. A browser that has
// signed in at this login before carries a device cookie for it and is counted
// on its own: whoever guesses at the login from elsewhere uses up the login's
// count, not the owner's, and cannot lock them out. The key of a device is
// longer than any login key, so the two never meet in the table.
func (a *Auth) attemptKey(r *http.Request, login string) string {
	if c, err := r.Cookie(deviceCookie); err == nil {
		if l, sig, ok := a.verifyDevice(c.Value); ok && l == login {
			return "device:" + sig
		}
	}
	return clip(login)
}

// setDevice remembers the browser as one that has signed in at login. It is
// not a session and gives no access: all it does is keep the browser off the
// login's count, see attemptKey. It goes only to the sign-in form, and signing
// out leaves it be. One per browser: signing in at another login replaces it.
func (a *Auth) setDevice(w http.ResponseWriter, login string) {
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(login + "|" + strconv.FormatInt(time.Now().Add(a.DeviceTTL).Unix(), 10)))
	http.SetCookie(w, &http.Cookie{
		Name:     deviceCookie,
		Value:    payload + "." + a.deviceMAC(payload),
		Path:     loginPath,
		MaxAge:   int(a.DeviceTTL.Seconds()),
		HttpOnly: true,
		Secure:   !a.Insecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// verifyDevice gives back the login a device cookie was signed for and its
// signature, if the cookie is ours and has not run out.
func (a *Auth) verifyDevice(value string) (login, sig string, ok bool) {
	payload, sig, found := strings.Cut(value, ".")
	if !found || !hmac.Equal([]byte(sig), []byte(a.deviceMAC(payload))) {
		return "", "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", "", false
	}
	// the login goes first and may hold a | of its own
	i := strings.LastIndex(string(raw), "|")
	if i < 0 {
		return "", "", false
	}
	exp, err := strconv.ParseInt(string(raw[i+1:]), 10, 64)
	if err != nil || time.Now().After(time.Unix(exp, 0)) {
		return "", "", false
	}
	return string(raw[:i]), sig, true
}

// deviceMAC signs a device cookie. The prefix keeps it apart from a session
// signed with the same key: neither cookie passes for the other.
func (a *Auth) deviceMAC(payload string) string {
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte("device|" + payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if !CheckCSRF(r) {
		http.Error(w, "csrf token mismatch", http.StatusForbidden)
		return
	}
	// Clearing the cookie is not enough: a copy taken before would still be
	// good. A session already over has nothing left to end.
	if u, err := a.session(r); err == nil {
		if err := a.store.EndSessions(r.Context(), u.ID); err != nil {
			// the cookie stays, so signing out can be tried again
			log.Printf("auth: ending sessions of user %d: %v", u.ID, err)
			http.Error(w, view.T("internal error"), http.StatusInternalServerError)
			return
		}
		a.logf(r, "user %d signed out", u.ID)
	}
	a.clearSession(w)
	http.Redirect(w, r, loginPath, http.StatusSeeOther)
}

func loginPage(token, login, errMsg string) view.FormView {
	return view.FormView{
		Action: loginPath,
		Submit: view.T("Sign in"),
		CSRF:   token,
		Error:  errMsg,
		Fields: []view.FieldView{
			{Name: "login", Label: view.T("Login"), Type: "text", Value: login, Autofill: "username", Required: true},
			{Name: "password", Label: view.T("Password"), Type: "password", Autofill: "current-password", Required: true},
		},
	}
}

// sign returns a cookie shaped base64(userID|exp|session).hmac
func (a *Auth) sign(userID int, session string, exp time.Time) string {
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(strconv.Itoa(userID) + "|" + strconv.FormatInt(exp.Unix(), 10) + "|" + session))
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

// verify gives back the user id and the session a cookie was signed with.
func (a *Auth) verify(value string) (id int, session string, err error) {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok {
		return 0, "", errors.New("malformed cookie")
	}
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	want, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(want, mac.Sum(nil)) {
		return 0, "", errors.New("signature does not match")
	}

	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return 0, "", err
	}
	// the session goes last and may hold a | of its own
	parts := strings.SplitN(string(raw), "|", 3)
	if len(parts) != 3 {
		return 0, "", errors.New("malformed cookie")
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, "", err
	}
	if time.Now().After(time.Unix(exp, 0)) {
		return 0, "", errors.New("session expired")
	}
	id, err = strconv.Atoi(parts[0])
	return id, parts[2], err
}

func (a *Auth) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   !a.Insecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *Auth) clearSession(w http.ResponseWriter) {
	a.setCookie(w, sessionCookie, "", -time.Second)
	a.setCookie(w, csrfCookie, "", -time.Second)
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

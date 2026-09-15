// Package auth — signing in to the admin panel: a session in a signed cookie,
// CSRF, ready-made /login and /logout handlers and a permission check on a
// route.
//
// All it needs from an application is a Store: where users are kept, how a
// password is stored and how permissions are worked out is the application's
// business, and the library knows nothing about it.
//
//	a := auth.New(db, key, "/clients")
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
	sessionTTL    = 24 * time.Hour

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
}

// Auth holds the store and the signing key. Created once, at startup.
type Auth struct {
	store    Store
	key      []byte
	home     string
	attempts limiter // sign-in attempts per login, see limit.go
}

// New: key signs the session cookie (change the key and every session is
// gone), home is where to send someone after signing in, empty means "/".
func New(store Store, key []byte, home string) *Auth {
	if home == "" {
		home = "/"
	}
	return &Auth{store: store, key: key, home: home}
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
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			http.Redirect(w, r, loginPath, http.StatusSeeOther)
			return
		}
		id, err := a.verify(c.Value)
		if err != nil {
			clearSession(w, r)
			http.Redirect(w, r, loginPath, http.StatusSeeOther)
			return
		}
		u, err := a.store.User(r.Context(), id)
		if err != nil {
			// The user was deleted or the database is silent — either way the
			// session is over, and telling the two apart here buys nothing.
			clearSession(w, r)
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
		setCookie(w, r, csrfCookie, token, sessionTTL)
	}
	view.Render(w, http.StatusOK, "login.html", loginPage(token, "", ""))
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	if !CheckCSRF(r) {
		http.Error(w, "csrf token mismatch", http.StatusForbidden)
		return
	}
	login := strings.TrimSpace(r.PostFormValue("login"))

	if ok, wait := a.attempts.try(login); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		view.Render(w, http.StatusTooManyRequests, "login.html",
			loginPage(CSRF(r), login, view.T("Too many attempts, try again later")))
		return
	}

	id, err := a.store.Authenticate(r.Context(), login, r.PostFormValue("password"))
	if err != nil {
		// One message for both cases — we do not hint whether the login exists.
		view.Render(w, http.StatusUnauthorized, "login.html",
			loginPage(CSRF(r), login, view.T("Wrong login or password")))
		return
	}
	a.attempts.forget(login)

	setCookie(w, r, sessionCookie, a.sign(id, time.Now().Add(sessionTTL)), sessionTTL)
	setCookie(w, r, csrfCookie, hex.EncodeToString(randomBytes(32)), sessionTTL)
	http.Redirect(w, r, a.home, http.StatusSeeOther)
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if !CheckCSRF(r) {
		http.Error(w, "csrf token mismatch", http.StatusForbidden)
		return
	}
	clearSession(w, r)
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

// sign returns a cookie shaped base64(userID|exp).hmac
func (a *Auth) sign(userID int, exp time.Time) string {
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(strconv.Itoa(userID) + "|" + strconv.FormatInt(exp.Unix(), 10)))
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func (a *Auth) verify(value string) (int, error) {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok {
		return 0, errors.New("malformed cookie")
	}
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	want, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(want, mac.Sum(nil)) {
		return 0, errors.New("signature does not match")
	}

	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return 0, err
	}
	idStr, expStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return 0, errors.New("malformed cookie")
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return 0, err
	}
	if time.Now().After(time.Unix(exp, 0)) {
		return 0, errors.New("session expired")
	}
	return strconv.Atoi(idStr)
}

func setCookie(w http.ResponseWriter, r *http.Request, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSession(w http.ResponseWriter, r *http.Request) {
	setCookie(w, r, sessionCookie, "", -time.Second)
	setCookie(w, r, csrfCookie, "", -time.Second)
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

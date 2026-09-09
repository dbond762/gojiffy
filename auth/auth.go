// Package auth — вход в админку: сессия в подписанной куке, CSRF, готовые
// обработчики /login и /logout и проверка прав на маршруте.
//
// От приложения нужен только Store: где лежат пользователи, как хранится
// пароль и как считаются права — дело приложения, либа об этом не знает.
//
//	a := auth.New(db, key, "/clients")
//	a.Mount(mux)                        // GET/POST /login, POST /logout
//	mux.Handle("/", a.Require(private)) // всё остальное — только с сессией
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

	// Пути зашиты: их знают шаблоны темы (форма входа шлёт на /login, кнопка в
	// шапке — POST /logout), поэтому договорённость держит либа с обоих концов.
	loginPath  = "/login"
	logoutPath = "/logout"
)

// User — то, что либа знает о вошедшем: чем подписать сессию, как назвать в
// шапке и что ему позволено. Своя сущность пользователя остаётся у приложения.
type User struct {
	ID    int
	Name  string
	Perms gojiffy.Perms
}

// Store — всё, что нужно от приложения.
type Store interface {
	// Authenticate — id пользователя по логину и паролю. Любая ошибка означает
	// отказ: «нет такого логина» и «пароль не тот» дают одно сообщение, чтобы
	// по форме нельзя было перебрать логины.
	Authenticate(ctx context.Context, login, password string) (int, error)
	// User — пользователь по id из сессионной куки. Зовётся на каждый запрос,
	// поэтому права всегда свежие: отобрали право — отобрали сразу.
	User(ctx context.Context, id int) (User, error)
}

// Auth держит хранилище и ключ подписи. Создаётся один раз при запуске.
type Auth struct {
	store Store
	key   []byte
	home  string
}

// New: key — ключ подписи сессионной куки (сменился ключ — все сессии слетели),
// home — куда отправлять после входа, пусто — на «/».
func New(store Store, key []byte, home string) *Auth {
	if home == "" {
		home = "/"
	}
	return &Auth{store: store, key: key, home: home}
}

// Mount вешает вход и выход. Они снаружи Require: до входа сессии ещё нет, а
// CSRF обе проверяют сами, поэтому от места монтирования это не зависит.
func (a *Auth) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET "+loginPath, a.loginForm)
	mux.HandleFunc("POST "+loginPath, a.login)
	mux.HandleFunc("POST "+logoutPath, a.logout)
}

// Require пускает дальше только с валидной сессией, POST дополнительно
// проверяет CSRF. Пользователь кладётся в контекст — достать UserFrom.
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
			// Пользователя удалили или база молчит — в обоих случаях сессии
			// больше нет, разбирать разницу здесь незачем.
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

// Can пускает дальше только с указанным правом. Оборачивать нужно то, что уже
// стоит за Require: без него в контексте пусто и прав нет ни у кого.
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

// UserFrom — вошедший пользователь. Вне Require отдаёт пустого: прав у него
// нет, значит Can никого не пустит.
func UserFrom(ctx context.Context) User {
	u, _ := ctx.Value(ctxKey{}).(User)
	return u
}

// CSRF — токен для скрытого поля формы (view.Page.CSRF, Resource.Form).
func CSRF(r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil {
		return c.Value
	}
	return ""
}

// CheckCSRF — double-submit: скрытое поле формы должно совпасть с кукой.
// Require зовёт её на каждый POST; отдельно она нужна только своим
// обработчикам вне Require.
func CheckCSRF(r *http.Request) bool {
	token := CSRF(r)
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(r.PostFormValue("_csrf"))) == 1
}

func (a *Auth) loginForm(w http.ResponseWriter, r *http.Request) {
	// CSRF-кука нужна ещё до входа, иначе нечего сверять с формой логина.
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

	id, err := a.store.Authenticate(r.Context(), login, r.PostFormValue("password"))
	if err != nil {
		// Одно сообщение на оба случая — не подсказываем, существует ли логин.
		view.Render(w, http.StatusUnauthorized, "login.html",
			loginPage(CSRF(r), login, view.T("Wrong login or password")))
		return
	}

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
			{Name: "login", Label: view.T("Login"), Type: "text", Value: login, Autocomplete: "username", Required: true},
			{Name: "password", Label: view.T("Password"), Type: "password", Autocomplete: "current-password", Required: true},
		},
	}
}

// sign возвращает куку вида base64(userID|exp).hmac
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
		return 0, errors.New("битая кука")
	}
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	want, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(want, mac.Sum(nil)) {
		return 0, errors.New("подпись не совпала")
	}

	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return 0, err
	}
	idStr, expStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return 0, errors.New("битая кука")
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return 0, err
	}
	if time.Now().After(time.Unix(exp, 0)) {
		return 0, errors.New("сессия истекла")
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

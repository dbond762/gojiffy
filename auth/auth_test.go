package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dbond762/gojiffy"
)

// fakeStore — one user whose password is "secret".
type fakeStore struct {
	perms   gojiffy.Perms
	session int // what EndSessions moves on
}

func (s *fakeStore) Authenticate(_ context.Context, login, password string) (int, error) {
	if login != "petr" || password != "secret" {
		return 0, errors.New("not that one")
	}
	return 42, nil
}

func (s *fakeStore) User(_ context.Context, id int) (User, error) {
	if id != 42 {
		return User{}, errors.New("no such user")
	}
	return User{ID: 42, Name: "Petr", Perms: s.perms, Session: strconv.Itoa(s.session)}, nil
}

func (s *fakeStore) EndSessions(_ context.Context, id int) error {
	if id == 42 {
		s.session++
	}
	return nil
}

func testAuth(perms ...string) *Auth {
	p := gojiffy.Perms{}
	for _, s := range perms {
		p[s] = true
	}
	return New(&fakeStore{perms: p}, []byte("test-key"), "/clients")
}

func TestSessionCookie(t *testing.T) {
	a := testAuth()
	valid := a.sign(42, "7|x", time.Now().Add(time.Hour))

	if id, session, err := a.verify(valid); err != nil || id != 42 || session != "7|x" {
		t.Fatalf("valid cookie: id=%d session=%q err=%v", id, session, err)
	}

	// signature tampered with
	broken := valid[:len(valid)-1] + string(valid[len(valid)-1]^1)
	if _, _, err := a.verify(broken); err == nil {
		t.Error("tampered signature accepted")
	}

	// payload swapped, signature from another value
	payload, sig, _ := strings.Cut(valid, ".")
	if _, _, err := a.verify(payload + "x." + sig); err == nil {
		t.Error("swapped payload accepted")
	}

	// someone else's key
	if _, _, err := New(&fakeStore{}, []byte("another"), "").verify(valid); err == nil {
		t.Error("cookie signed with another key accepted")
	}

	if _, _, err := a.verify(a.sign(42, "0", time.Now().Add(-time.Minute))); err == nil {
		t.Error("expired cookie accepted")
	}

	if _, _, err := a.verify("rubbish"); err == nil {
		t.Error("rubbish accepted")
	}
}

func TestCheckCSRF(t *testing.T) {
	post := func(cookie, field string) bool {
		body := url.Values{"_csrf": {field}}.Encode()
		r := httptest.NewRequest("POST", "/users", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: csrfCookie, Value: cookie})
		}
		return CheckCSRF(r)
	}

	if !post("tok123", "tok123") {
		t.Error("matching token rejected")
	}
	if post("tok123", "someone else's") {
		t.Error("foreign token accepted")
	}
	if post("tok123", "") {
		t.Error("empty field accepted")
	}
	if post("", "") {
		t.Error("request without a cookie accepted")
	}
}

// Signing in end to end: the form hands out a CSRF cookie, the right pair of
// login and password starts a session, and with it Require lets the request
// through and puts the user in the context.
func TestLoginFlow(t *testing.T) {
	a := testAuth()
	mux := http.NewServeMux()
	a.Mount(mux)

	// GET /login — the page and a CSRF cookie
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/login", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("sign-in form: %d", w.Code)
	}
	csrf := cookie(w.Result().Cookies(), csrfCookie)
	if csrf == "" {
		t.Fatal("sign-in form set no csrf cookie")
	}

	post := func(login, password string) *httptest.ResponseRecorder {
		body := url.Values{"login": {login}, "password": {password}, "_csrf": {csrf}}.Encode()
		r := httptest.NewRequest("POST", "/login", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: csrfCookie, Value: csrf})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}

	if w := post("petr", "not it"); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong password gave %d", w.Code)
	} else if cookie(w.Result().Cookies(), sessionCookie) != "" {
		t.Error("wrong password started a session")
	}

	w = post("petr", "secret")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/clients" {
		t.Fatalf("signing in gave %d to %q", w.Code, w.Header().Get("Location"))
	}
	sid := cookie(w.Result().Cookies(), sessionCookie)
	if sid == "" {
		t.Fatal("signing in started no session")
	}

	// with a session Require lets through and puts the user in the context
	var got User
	h := a.Require(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = UserFrom(r.Context())
	}))
	r := httptest.NewRequest("GET", "/clients", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: sid})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got.ID != 42 || got.Name != "Petr" {
		t.Errorf("context holds %+v", got)
	}

	// without a session — back to the sign-in form
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/clients", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Errorf("no cookie gave %d to %q", w.Code, w.Header().Get("Location"))
	}
}

// A POST without CSRF does not pass even with a valid session: otherwise the
// double submit would only ever guard signing in.
func TestRequireChecksCSRF(t *testing.T) {
	a := testAuth()
	h := a.Require(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	r := httptest.NewRequest("POST", "/clients", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: a.sign(42, "0", time.Now().Add(time.Hour))})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("POST without csrf gave %d", w.Code)
	}
}

// Can looks at the permissions in the context, not at a role.
func TestCan(t *testing.T) {
	next := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }

	for _, tc := range []struct {
		perms []string
		want  int
	}{
		{[]string{"clients.list"}, http.StatusTeapot},
		{[]string{"users.list"}, http.StatusForbidden},
		{nil, http.StatusForbidden},
	} {
		a := testAuth(tc.perms...)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/clients", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: a.sign(42, "0", time.Now().Add(time.Hour))})
		a.Require(a.Can("clients.list", next)).ServeHTTP(w, r)

		if w.Code != tc.want {
			t.Errorf("permissions %v gave %d, expected %d", tc.perms, w.Code, tc.want)
		}
	}
}

// Guessing is refused after loginAttempts tries at one login, the right
// password included, while other logins go on as before; the window ending or
// signing in successfully clears the count.
func TestLoginLimit(t *testing.T) {
	a := testAuth()
	mux := http.NewServeMux()
	a.Mount(mux)
	post := func(login, password string) *httptest.ResponseRecorder {
		body := url.Values{"login": {login}, "password": {password}, "_csrf": {"tok"}}.Encode()
		r := httptest.NewRequest("POST", "/login", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "tok"})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}

	// a success clears the typos before it
	for range loginAttempts - 1 {
		post("petr", "typo")
	}
	if w := post("petr", "secret"); w.Code != http.StatusSeeOther {
		t.Fatalf("signing in after typos gave %d", w.Code)
	}

	for i := range loginAttempts {
		if w := post("petr", "guess"); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d gave %d", i+1, w.Code)
		}
	}
	w := post("petr", "secret")
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Errorf("attempt over the limit gave %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
	if cookie(w.Result().Cookies(), sessionCookie) != "" {
		t.Error("attempt over the limit started a session")
	}
	if w := post("ivan", "guess"); w.Code != http.StatusUnauthorized {
		t.Errorf("another login gave %d", w.Code)
	}

	a.attempts.reset = time.Now().Add(-time.Second) // the window is over
	if w := post("petr", "secret"); w.Code != http.StatusSeeOther {
		t.Errorf("signing in after the window gave %d", w.Code)
	}
}

// A session ends for good: signing out refuses a copy of the cookie kept from
// before, and so does the store moving the session on (a new password), while
// a session begun after that goes through.
func TestSessionsEnd(t *testing.T) {
	a := testAuth()
	store := a.store.(*fakeStore)
	mux := http.NewServeMux()
	a.Mount(mux)

	signIn := func() string {
		body := url.Values{"login": {"petr"}, "password": {"secret"}, "_csrf": {"tok"}}.Encode()
		r := httptest.NewRequest("POST", "/login", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "tok"})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return cookie(w.Result().Cookies(), sessionCookie)
	}
	through := func(sid string) bool {
		passed := false
		r := httptest.NewRequest("GET", "/clients", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: sid})
		a.Require(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { passed = true })).
			ServeHTTP(httptest.NewRecorder(), r)
		return passed
	}

	sid := signIn()
	if !through(sid) {
		t.Fatal("a fresh session refused")
	}
	store.EndSessions(context.Background(), 42) // the password changed
	if through(sid) {
		t.Error("a session from before the new password let through")
	}

	sid = signIn()
	if !through(sid) {
		t.Fatal("a session begun after the new password refused")
	}
	body := url.Values{"_csrf": {"tok"}}.Encode()
	r := httptest.NewRequest("POST", "/logout", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "tok"})
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: sid})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("signing out gave %d", w.Code)
	}
	if through(sid) {
		t.Error("a copy of the cookie kept from before signing out let through")
	}

	// signing out with no session at all ends nothing
	before := store.session
	r = httptest.NewRequest("POST", "/logout", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "tok"})
	mux.ServeHTTP(httptest.NewRecorder(), r)
	if store.session != before {
		t.Error("signing out with no session ended sessions")
	}
}

func cookie(cs []*http.Cookie, name string) string {
	for _, c := range cs {
		if c.Name == name && c.MaxAge > 0 {
			return c.Value
		}
	}
	return ""
}

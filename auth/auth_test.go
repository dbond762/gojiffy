package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dbond762/gojiffy"
)

// fakeStore — один пользователь с паролем «secret».
type fakeStore struct{ perms gojiffy.Perms }

func (s fakeStore) Authenticate(_ context.Context, login, password string) (int, error) {
	if login != "petr" || password != "secret" {
		return 0, errors.New("не тот")
	}
	return 42, nil
}

func (s fakeStore) User(_ context.Context, id int) (User, error) {
	if id != 42 {
		return User{}, errors.New("нет такого")
	}
	return User{ID: 42, Name: "Пётр", Perms: s.perms}, nil
}

func testAuth(perms ...string) *Auth {
	p := gojiffy.Perms{}
	for _, s := range perms {
		p[s] = true
	}
	return New(fakeStore{p}, []byte("test-key"), "/clients")
}

func TestSessionCookie(t *testing.T) {
	a := testAuth()
	valid := a.sign(42, time.Now().Add(time.Hour))

	if id, err := a.verify(valid); err != nil || id != 42 {
		t.Fatalf("валидная кука: id=%d err=%v", id, err)
	}

	// подпись испорчена
	broken := valid[:len(valid)-1] + string(valid[len(valid)-1]^1)
	if _, err := a.verify(broken); err == nil {
		t.Error("испорченная подпись принята")
	}

	// payload подменён, подпись от другого значения
	payload, sig, _ := strings.Cut(valid, ".")
	if _, err := a.verify(payload + "x." + sig); err == nil {
		t.Error("подменённый payload принят")
	}

	// ключ чужой
	if _, err := New(fakeStore{}, []byte("другой"), "").verify(valid); err == nil {
		t.Error("кука от чужого ключа принята")
	}

	if _, err := a.verify(a.sign(42, time.Now().Add(-time.Minute))); err == nil {
		t.Error("протухшая кука принята")
	}

	if _, err := a.verify("мусор"); err == nil {
		t.Error("мусор принят")
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
		t.Error("совпадающий токен отвергнут")
	}
	if post("tok123", "чужой") {
		t.Error("чужой токен принят")
	}
	if post("tok123", "") {
		t.Error("пустое поле принято")
	}
	if post("", "") {
		t.Error("запрос без куки принят")
	}
}

// Вход целиком: форма отдаёт CSRF-куку, верная пара логин/пароль заводит
// сессию, с ней Require пускает дальше и кладёт пользователя в контекст.
func TestLoginFlow(t *testing.T) {
	a := testAuth()
	mux := http.NewServeMux()
	a.Mount(mux)

	// GET /login — страница и CSRF-кука
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/login", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("форма входа: %d", w.Code)
	}
	csrf := cookie(w.Result().Cookies(), csrfCookie)
	if csrf == "" {
		t.Fatal("форма входа не поставила csrf-куку")
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

	if w := post("petr", "нетот"); w.Code != http.StatusUnauthorized {
		t.Errorf("неверный пароль дал %d", w.Code)
	} else if cookie(w.Result().Cookies(), sessionCookie) != "" {
		t.Error("неверный пароль завёл сессию")
	}

	w = post("petr", "secret")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/clients" {
		t.Fatalf("вход дал %d → %q", w.Code, w.Header().Get("Location"))
	}
	sid := cookie(w.Result().Cookies(), sessionCookie)
	if sid == "" {
		t.Fatal("вход не завёл сессию")
	}

	// с сессией Require пускает и кладёт пользователя в контекст
	var got User
	h := a.Require(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = UserFrom(r.Context())
	}))
	r := httptest.NewRequest("GET", "/clients", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: sid})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got.ID != 42 || got.Name != "Пётр" {
		t.Errorf("в контексте %+v", got)
	}

	// без сессии — на форму входа
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/clients", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Errorf("без куки %d → %q", w.Code, w.Header().Get("Location"))
	}
}

// POST без CSRF не проходит, даже с валидной сессией: иначе double-submit
// защищал бы только вход.
func TestRequireChecksCSRF(t *testing.T) {
	a := testAuth()
	h := a.Require(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	r := httptest.NewRequest("POST", "/clients", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: a.sign(42, time.Now().Add(time.Hour))})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("POST без csrf дал %d", w.Code)
	}
}

// Can смотрит на права из контекста, а не на роль.
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
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: a.sign(42, time.Now().Add(time.Hour))})
		a.Require(a.Can("clients.list", next)).ServeHTTP(w, r)

		if w.Code != tc.want {
			t.Errorf("права %v дали %d, expected %d", tc.perms, w.Code, tc.want)
		}
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

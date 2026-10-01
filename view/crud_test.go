package view

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/dbond762/gojiffy"
)

type note struct {
	ID     int
	Title  string
	Secret string // in the form only with the "secret" permission
	Owner  string // never in the form: BeforeSave sets it
}

var errTaken = errors.New("title taken")

// notes — a store in memory with the contract of a real one: ErrNotFound for a
// missing id, an error of its own for a title already there.
type notes struct {
	rows map[int]note
	next int
}

func (s *notes) List(context.Context, gojiffy.Search, gojiffy.Order, gojiffy.Paging) ([]note, int, error) {
	var out []note
	for id := 1; id <= s.next; id++ {
		if n, ok := s.rows[id]; ok {
			out = append(out, n)
		}
	}
	return out, len(out), nil
}

func (s *notes) Get(_ context.Context, id int) (note, error) {
	n, ok := s.rows[id]
	if !ok {
		return note{}, gojiffy.ErrNotFound
	}
	return n, nil
}

func (s *notes) taken(n note) bool {
	for _, o := range s.rows {
		if o.Title == n.Title && o.ID != n.ID {
			return true
		}
	}
	return false
}

func (s *notes) Create(_ context.Context, n note) (int, error) {
	if s.taken(n) {
		return 0, errTaken
	}
	s.next++
	n.ID = s.next
	s.rows[n.ID] = n
	return n.ID, nil
}

func (s *notes) Update(_ context.Context, n note) error {
	if _, ok := s.rows[n.ID]; !ok {
		return gojiffy.ErrNotFound
	}
	if s.taken(n) {
		return errTaken
	}
	s.rows[n.ID] = n
	return nil
}

func (s *notes) Delete(_ context.Context, id int) error {
	if _, ok := s.rows[id]; !ok {
		return gojiffy.ErrNotFound
	}
	delete(s.rows, id)
	return nil
}

func noteHref(n note) string { return "/notes/" + strconv.Itoa(n.ID) }

// section — a CRUD over notes, mounted behind a guard that lets through only
// the permissions in perms; the request's own permissions come in X-Perms.
func section(store *notes, perms ...string) (*http.ServeMux, *CRUD[note]) {
	c := &CRUD[note]{
		Resource: Resource[note]{
			Path: "/notes", Title: "Notes", NewTitle: "New note", Href: noteHref,
			Fields: []Field[note]{
				{Name: "title", Caption: "Title", Required: true, Text: func(n note) string { return n.Title }},
				{Name: "secret", Caption: "Secret", EditOnly: true, Permission: "secret", Value: func(n note) string { return n.Secret }},
			},
		},
		Frame: frame,
		Store: store,
		Perms: func(r *http.Request) gojiffy.Perms {
			p := gojiffy.Perms{}
			for _, name := range strings.Fields(r.Header.Get("X-Perms")) {
				p[name] = true
			}
			return p
		},
		New:        func(*http.Request) note { return note{Title: "untitled"} },
		StoreError: func(err error) map[string]string { return fieldErr(errors.Is(err, errTaken), "title", "taken") },
	}
	mux := http.NewServeMux()
	allowed := map[string]bool{}
	for _, p := range perms {
		allowed[p] = true
	}
	can := func(perm string, next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !allowed[perm] {
				http.Error(w, "denied", http.StatusForbidden)
				return
			}
			next(w, r)
		}
	}
	// Mount reads the hooks when a request comes, not when it is called: the
	// tests set them on c after mounting
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		m := http.NewServeMux()
		c.Mount(m, can, "notes")
		m.ServeHTTP(w, r)
	})
	return mux, c
}

func fieldErr(ok bool, field, msg string) map[string]string {
	if !ok {
		return nil
	}
	return map[string]string{field: msg}
}

func do(h http.Handler, method, path string, values url.Values, perms string) *httptest.ResponseRecorder {
	var r *http.Request
	if method == "POST" {
		r = post(values)
		r.URL.Path = path
		r.RequestURI = path
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("X-Perms", perms)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// A section goes the whole way round: created, listed, edited, deleted, and
// gone after that.
func TestCRUDRoundTrip(t *testing.T) {
	store := &notes{rows: map[int]note{}}
	h, _ := section(store, "notes")

	if w := do(h, "GET", "/notes/new", nil, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `value="untitled"`) {
		t.Fatalf("new form: code %d, or New gave no defaults", w.Code)
	}
	if w := do(h, "POST", "/notes", url.Values{"title": {"first"}}, ""); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/notes" {
		t.Fatalf("create: code %d to %q", w.Code, w.Header().Get("Location"))
	}
	if w := do(h, "GET", "/notes", nil, ""); !strings.Contains(w.Body.String(), "first") {
		t.Fatal("the list does not show what was created")
	}
	if w := do(h, "GET", "/notes/1", nil, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `value="first"`) {
		t.Fatalf("edit form: code %d", w.Code)
	}
	if w := do(h, "POST", "/notes/1", url.Values{"title": {"renamed"}}, ""); w.Code != http.StatusSeeOther || store.rows[1].Title != "renamed" {
		t.Fatalf("save: code %d, stored %+v", w.Code, store.rows[1])
	}
	if w := do(h, "POST", "/notes/1/delete", nil, ""); w.Code != http.StatusSeeOther || len(store.rows) != 0 {
		t.Fatalf("delete: code %d, %d rows left", w.Code, len(store.rows))
	}
	for _, path := range []string{"/notes/1", "/notes/abc"} {
		if w := do(h, "GET", path, nil, ""); w.Code != http.StatusNotFound {
			t.Errorf("%s: code %d, want 404", path, w.Code)
		}
	}
	if w := do(h, "POST", "/notes/1", url.Values{"title": {"x"}}, ""); w.Code != http.StatusNotFound {
		t.Errorf("saving what is gone: code %d, want 404", w.Code)
	}
}

// Saving parses over the stored record: what the form does not send — a field
// hidden by a permission, one that is never in the form — keeps its value, and
// so does the id.
func TestCRUDSaveKeepsWhatTheFormDoesNotSend(t *testing.T) {
	store := &notes{rows: map[int]note{7: {ID: 7, Title: "a", Secret: "s", Owner: "olya"}}, next: 7}
	h, _ := section(store, "notes")

	do(h, "POST", "/notes/7", url.Values{"title": {"b"}, "secret": {"forged"}}, "")
	if got := store.rows[7]; got != (note{ID: 7, Title: "b", Secret: "s", Owner: "olya"}) {
		t.Errorf("without the permission: %+v", got)
	}
	do(h, "POST", "/notes/7", url.Values{"title": {"b"}, "secret": {"new"}}, "secret")
	if got := store.rows[7].Secret; got != "new" {
		t.Errorf("with the permission the field was not taken: %q", got)
	}
}

// BeforeSave sets what the form does not and may say no under a field; the
// store's own error for a field is shown the same way. Either keeps the form
// on the page with a 422 and leaves the store as it was.
func TestCRUDFieldErrorsFromHooksAndStore(t *testing.T) {
	store := &notes{rows: map[int]note{}}
	h, c := section(store, "notes")
	c.BeforeSave = func(_ *http.Request, n *note, creating bool) (map[string]string, error) {
		n.Owner = "petr"
		return fieldErr(n.Title == "forbidden", "title", "not that one"), nil
	}

	do(h, "POST", "/notes", url.Values{"title": {"one"}}, "")
	if store.rows[1].Owner != "petr" {
		t.Errorf("BeforeSave did not set the owner: %+v", store.rows[1])
	}
	for title, msg := range map[string]string{"forbidden": "not that one", "one": "taken"} {
		w := do(h, "POST", "/notes", url.Values{"title": {title}}, "")
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), msg) {
			t.Errorf("%s: code %d, want 422 with %q", title, w.Code, msg)
		}
	}
	if len(store.rows) != 1 {
		t.Errorf("a refused form reached the store: %d rows", len(store.rows))
	}

	c.StoreError = nil
	if w := do(h, "POST", "/notes", url.Values{"title": {"one"}}, ""); w.Code != http.StatusInternalServerError {
		t.Errorf("a store error nobody maps: code %d, want 500", w.Code)
	}
}

// BeforeDelete may refuse with words of its own; AfterDelete hears of the
// record once it is gone; deleting asks for its own permission when it has one.
func TestCRUDDeleteHooks(t *testing.T) {
	store := &notes{rows: map[int]note{1: {ID: 1, Title: "keep"}, 2: {ID: 2, Title: "drop"}}, next: 2}
	h, c := section(store, "notes")
	var after []note
	c.BeforeDelete = func(_ *http.Request, n note) error {
		if n.Title == "keep" {
			return Refuse("this one stays")
		}
		return nil
	}
	c.AfterDelete = func(_ *http.Request, n note) { after = append(after, n) }

	if w := do(h, "POST", "/notes/1/delete", nil, ""); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "this one stays") {
		t.Errorf("refused delete: code %d %q", w.Code, w.Body.String())
	}
	do(h, "POST", "/notes/2/delete", nil, "")
	if _, ok := store.rows[1]; !ok || len(store.rows) != 1 {
		t.Errorf("rows left: %+v", store.rows)
	}
	if len(after) != 1 || after[0].Title != "drop" {
		t.Errorf("AfterDelete heard of %+v", after)
	}

	c.DeletePerm = "notes.delete"
	if w := do(h, "POST", "/notes/1/delete", nil, ""); w.Code != http.StatusForbidden {
		t.Errorf("delete without its permission: code %d, want 403", w.Code)
	}
}

// StoreFor picks the store per request and wins over Store: what one sees —
// the list, a record by its address, what is saved and deleted — is what the
// store of that request holds.
func TestCRUDStoreFor(t *testing.T) {
	everyone := &notes{rows: map[int]note{1: {ID: 1, Title: "common"}}, next: 1}
	mine := &notes{rows: map[int]note{2: {ID: 2, Title: "my own"}}, next: 2}
	h, c := section(everyone, "notes")
	c.StoreFor = func(r *http.Request) gojiffy.Lister[note] {
		if r.Header.Get("X-Perms") == "all" {
			return everyone
		}
		return mine
	}

	if body := do(h, "GET", "/notes", nil, "").Body.String(); !strings.Contains(body, "my own") || strings.Contains(body, "common") {
		t.Error("the list did not come from the request's store")
	}
	if w := do(h, "GET", "/notes/1", nil, ""); w.Code != http.StatusNotFound {
		t.Errorf("a record outside the request's store: code %d, want 404", w.Code)
	}
	if w := do(h, "POST", "/notes/1/delete", nil, ""); w.Code != http.StatusNotFound || len(everyone.rows) != 1 {
		t.Errorf("deleting outside the request's store: code %d", w.Code)
	}
	if w := do(h, "GET", "/notes/1", nil, "all"); w.Code != http.StatusOK {
		t.Errorf("the same record through the store that holds it: code %d", w.Code)
	}
}

// A section with nowhere to keep its records fails when it is mounted, not on
// the first request.
func TestCRUDMountWithoutStorePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	CRUD[note]{Resource: Resource[note]{Path: "/notes"}}.Mount(http.NewServeMux(), nil, "notes")
}

// listOnly — a store that gives a list and nothing else.
type listOnly struct{ n *notes }

func (l listOnly) List(ctx context.Context, s gojiffy.Search, o gojiffy.Order, p gojiffy.Paging) ([]note, int, error) {
	return l.n.List(ctx, s, o, p)
}

// undeletable — a store that does everything but Delete.
type undeletable struct{ listOnly }

func (u undeletable) Get(ctx context.Context, id int) (note, error)   { return u.n.Get(ctx, id) }
func (u undeletable) Create(ctx context.Context, x note) (int, error) { return u.n.Create(ctx, x) }
func (u undeletable) Update(ctx context.Context, x note) error        { return u.n.Update(ctx, x) }

// A section that leaves a part out puts none of its addresses up, and its
// store need not do that part; one whose store falls short of what it does
// fails at Mount, saying what is missing and how to put it right.
func TestCRUDLeavesPartsOut(t *testing.T) {
	store := &notes{rows: map[int]note{1: {ID: 1, Title: "one"}}, next: 1}
	mount := func(c CRUD[note]) *http.ServeMux {
		c.Resource = Resource[note]{Path: "/notes", Href: noteHref,
			Fields: []Field[note]{{Name: "title", Caption: "Title", Text: func(n note) string { return n.Title }}}}
		c.Frame = frame
		mux := http.NewServeMux()
		c.Mount(mux, func(_ string, next http.HandlerFunc) http.HandlerFunc { return next }, "notes")
		return mux
	}

	ro := mount(CRUD[note]{Store: listOnly{store}, NoCreate: true, NoEdit: true, NoDelete: true})
	if w := do(ro, "GET", "/notes", nil, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "one") {
		t.Errorf("read-only list: code %d", w.Code)
	}
	for _, req := range [][2]string{{"GET", "/notes/new"}, {"POST", "/notes"}, {"GET", "/notes/1"}, {"POST", "/notes/1"}, {"POST", "/notes/1/delete"}} {
		if w := do(ro, req[0], req[1], url.Values{"title": {"x"}}, ""); w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Errorf("read-only %s %s: code %d, want no such address", req[0], req[1], w.Code)
		}
	}

	nd := mount(CRUD[note]{Store: undeletable{listOnly{store}}, NoDelete: true})
	if w := do(nd, "POST", "/notes/1", url.Values{"title": {"edited"}}, ""); w.Code != http.StatusSeeOther || store.rows[1].Title != "edited" {
		t.Errorf("editing without deleting: code %d", w.Code)
	}
	if w := do(nd, "POST", "/notes/1/delete", nil, ""); w.Code != http.StatusNotFound || len(store.rows) != 1 {
		t.Errorf("delete left out: code %d", w.Code)
	}

	for want, c := range map[string]CRUD[note]{
		"NoDelete": {Store: undeletable{listOnly{store}}},
		"NoCreate": {Store: listOnly{store}, NoEdit: true, NoDelete: true},
		"NoEdit":   {Store: listOnly{store}, NoCreate: true},
	} {
		func() {
			defer func() {
				if msg, _ := recover().(string); !strings.Contains(msg, want) {
					t.Errorf("a store short of the section: panic %q, want it to name %s", msg, want)
				}
			}()
			mount(c)
		}()
	}
}

// StoreFor cannot be checked before a request: a store of it that falls short
// is a server error, not a panic in the middle of serving.
func TestCRUDStoreForShortOfTheSection(t *testing.T) {
	store := &notes{rows: map[int]note{1: {ID: 1, Title: "one"}}, next: 1}
	h, c := section(store, "notes")
	c.StoreFor = func(*http.Request) gojiffy.Lister[note] { return undeletable{listOnly{store}} }
	if w := do(h, "POST", "/notes/1/delete", nil, ""); w.Code != http.StatusInternalServerError || len(store.rows) != 1 {
		t.Errorf("delete through a store with no Delete: code %d", w.Code)
	}
}

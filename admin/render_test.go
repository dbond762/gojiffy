package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/dbond762/gojiffy"
	"github.com/dbond762/gojiffy/view"
)

// frame — the wrapping every test page shares.
func frame(*http.Request) (view.Page, error) { return view.Page{UserName: "Petr"}, nil }

// A form page is the form with the way back to its list: 200 when it is drawn
// fresh, 422 when it came back with errors, 500 when its choices could not be
// had — never a form with an empty select.
func TestRenderForm(t *testing.T) {
	rs := probeRes(Field[probe]{Name: "login", Caption: "Login"})
	rs.Title, rs.NewTitle = "Accounts", "New account"
	draw := func(rs Resource[probe], errs map[string]string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		rs.RenderForm(w, httptest.NewRequest("GET", "/x/new", nil), frame, nil, probe{}, true, errs)
		return w
	}

	w := draw(rs, nil)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, "<title>New account") || !strings.Contains(body, "Accounts") {
		t.Errorf("fresh form: code %d, title or crumb to the list missing", w.Code)
	}

	if w := draw(rs, map[string]string{"login": "already taken"}); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "already taken") {
		t.Errorf("form with errors: code %d", w.Code)
	}

	rs.Fields[0].LookupChoices = true // and no store to ask
	if w := draw(rs, nil); w.Code != http.StatusInternalServerError {
		t.Errorf("choices that could not be had: code %d", w.Code)
	}
}

// lister — a store handing out the same records whatever is asked.
type lister []probe

func (l lister) List(context.Context, gojiffy.Search, gojiffy.Order, gojiffy.Paging) ([]probe, int, error) {
	return l, len(l), nil
}

// A nested resource says once what it is nested in, and the list and the form
// put themselves after it; a page that wants a trail of its own still says so.
func TestCrumbsFollowTheResource(t *testing.T) {
	rs := probeRes(Field[probe]{Name: "login", Caption: "Login"})
	rs.Title = "Accounts"
	rs.Crumbs = []view.Link{{Title: "Acme", Href: "/acme"}}

	list, err := rs.ListBlock(httptest.NewRequest("GET", "/x", nil), lister{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []view.Link{{Title: "Acme", Href: "/acme"}, {Title: "Accounts"}}; !slices.Equal(list.Crumbs, want) {
		t.Errorf("list crumbs: %+v, want %+v", list.Crumbs, want)
	}
	fb := rs.FormBlock(form(rs, probe{}))
	if want := []view.Link{{Title: "Acme", Href: "/acme"}, {Title: "Accounts", Href: "/x"}, {Title: "Accounts"}}; !slices.Equal(fb.Crumbs, want) {
		t.Errorf("form crumbs: %+v, want %+v", fb.Crumbs, want)
	}

	page := func(crumbs []view.Link) string {
		w := httptest.NewRecorder()
		view.RenderPage(w, httptest.NewRequest("GET", "/x", nil), frame, http.StatusOK, "", crumbs, view.NoticeBlock(nil), fb)
		return w.Body.String()
	}
	if !strings.Contains(page(nil), `href="/acme"`) {
		t.Error("the block's crumbs did not reach the page")
	}
	if body := page([]view.Link{{Title: "Elsewhere"}}); strings.Contains(body, `href="/acme"`) || !strings.Contains(body, "Elsewhere") {
		t.Error("crumbs given to the page did not win over the block's")
	}
}

// A field that suggests as you type offers its empty value when it may stay
// empty, the way a select does: an optional form field, and a filter with All.
// A required field does not.
func TestLookupOffersBlankWhenOptional(t *testing.T) {
	store := chooser{"author_id": {{Value: "1", Label: "Olya"}}}
	field := Field[probe]{
		Name: "author_id", LookupChoices: true, LookupLimit: 5, Search: true, Filter: FilterLookup,
		Value: func(p probe) string { return p.AuthorID }, Text: func(p probe) string { return p.AuthorID },
	}
	form := func(required bool) string {
		f := field
		f.Required = required
		w := httptest.NewRecorder()
		probeRes(f).RenderForm(w, httptest.NewRequest("GET", "/x/1", nil), frame, store, probe{}, false, nil)
		return w.Body.String()
	}
	if !strings.Contains(form(false), `data-blank="`+view.T("— none —")+`"`) {
		t.Error("an optional field offers no empty value")
	}
	if strings.Contains(form(true), "data-blank") {
		t.Error("a required field offers an empty value")
	}

	list, err := probeRes(field).ListBlock(httptest.NewRequest("GET", "/x", nil), listStore{store, lister{}})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	view.RenderPage(w, httptest.NewRequest("GET", "/x", nil), frame, http.StatusOK, "", nil, list)
	if !strings.Contains(w.Body.String(), `data-blank="`+view.T("All")+`"`) {
		t.Error("a lookup filter offers no All")
	}
}

// A Link button is a plain link to a page of its own; the others still post
// their own form with the token.
func TestFormButtonLink(t *testing.T) {
	rs := probeRes(Field[probe]{Name: "login", Caption: "Login"})
	form, err := rs.Form(context.Background(), nil, probe{}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	form.Buttons = []view.Button{
		{Title: "Extend", Action: "/x/1/extend", Link: true},
		{Title: "Regenerate", Action: "/x/1/generate"},
	}
	w := httptest.NewRecorder()
	view.RenderPage(w, httptest.NewRequest("GET", "/x/1", nil), frame, http.StatusOK, "", nil, rs.FormBlock(form))
	body := w.Body.String()

	if !strings.Contains(body, `<a class="btn " href="/x/1/extend">Extend</a>`) {
		t.Error("the link button is not a link")
	}
	if strings.Contains(body, `action="/x/1/extend"`) {
		t.Error("the link button still posts a form")
	}
	if !strings.Contains(body, `<form method="post" action="/x/1/generate"`) {
		t.Error("the plain button lost its form")
	}
}

// The library does not translate labels out of a resource description — they
// arrive finished. The text below is deliberately not English: it stands for an
// application writing in its own language, and it has to come out untouched.
func TestResourceLabelsPassThrough(t *testing.T) {
	rs := Resource[string]{
		Title:     "Довільний заголовок",
		EditTitle: func(string) string { return "Запис" },
		NewTitle:  "Новий запис",
		Empty:     "Порожньо",
		Href:      func(string) string { return "/x" },
		Fields: []Field[string]{{
			Name: "v", Caption: "Значення", Help: "Підказка",
			Text: func(s string) string { return s },
		}},
		Actions: []Action[string]{{Title: "Змінити", Href: func(string) string { return "/x" }}},
	}

	lv := rs.List([]string{"a"}, 1, gojiffy.Paging{Page: 1, PerPage: 20}, nil, gojiffy.Order{})
	if lv.Title != rs.Title || lv.Table.Columns[0].Title != "Значення" {
		t.Errorf("list headings changed: %q, %q", lv.Title, lv.Table.Columns[0].Title)
	}
	if got := lv.Table.Rows[0][1].Actions[0].Title; got != "Змінити" {
		t.Errorf("the action caption changed: %q", got)
	}

	fv := form(rs, "a")
	if fv.Title != rs.EditTitle("a") || fv.Fields[0].Label != "Значення" || fv.Fields[0].Help != "Підказка" {
		t.Errorf("form labels changed: %+v", fv)
	}
}

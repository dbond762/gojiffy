package view

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/dbond762/gojiffy"
)

func frame(*http.Request) (Page, error) { return Page{UserName: "Petr"}, nil }

func block(b Block) Component {
	return func(http.ResponseWriter, *http.Request) (Block, error) { return b, nil }
}

// A page is its blocks: every one of them is drawn, in the order given, and the
// title comes from the first block that names one.
func TestHandlerDrawsEveryBlock(t *testing.T) {
	h := Handler(frame,
		block(NoticeBlock(&Notice{Title: "the secret"})),
		block(Block{Name: "list", Title: "Clients", Data: ListView{Title: "the client panel"}}),
	)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/clients", nil))
	body := w.Body.String()

	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	notice, list := strings.Index(body, "the secret"), strings.Index(body, "the client panel")
	if notice < 0 || list < 0 {
		t.Fatalf("a block did not reach the page: notice=%d list=%d", notice, list)
	}
	if notice > list {
		t.Error("blocks are drawn out of order")
	}
	if !strings.Contains(body, "<title>Clients") {
		t.Error("the title did not come from the block")
	}
	if !strings.Contains(body, "Petr") {
		t.Error("the frame did not reach the page")
	}
}

// Every page, sign-in included, keeps out of the cache and out of other sites'
// frames, and sends no address of the panel to them.
func TestPageSecurityHeaders(t *testing.T) {
	pages := map[string]*httptest.ResponseRecorder{"page": httptest.NewRecorder(), "sign-in": httptest.NewRecorder()}
	Handler(frame, block(Block{Name: "list", Data: ListView{}})).
		ServeHTTP(pages["page"], httptest.NewRequest("GET", "/clients", nil))
	Render(pages["sign-in"], http.StatusOK, "login.html", FormView{})

	for name, w := range pages {
		h := w.Header()
		for header, want := range map[string]string{
			"Cache-Control":          "no-store",
			"X-Content-Type-Options": "nosniff",
			"Referrer-Policy":        "same-origin",
		} {
			if got := h.Get(header); got != want {
				t.Errorf("%s: %s = %q, want %q", name, header, got, want)
			}
		}
		csp := h.Get("Content-Security-Policy")
		for _, directive := range []string{"frame-ancestors 'none'", "form-action 'self'"} {
			if !strings.Contains(csp, directive) {
				t.Errorf("%s: Content-Security-Policy %q has no %s", name, csp, directive)
			}
		}
	}
}

// A component with nothing to show hands back an empty block instead of a
// special case, so the caller has nothing to check.
func TestHandlerSkipsEmptyBlock(t *testing.T) {
	h := Handler(frame, block(NoticeBlock(nil)), block(Block{Name: "list", Data: ListView{}}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/clients", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
}

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
	rs.Crumbs = []Link{{Title: "Acme", Href: "/acme"}}

	list, err := rs.ListBlock(httptest.NewRequest("GET", "/x", nil), lister{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []Link{{Title: "Acme", Href: "/acme"}, {Title: "Accounts"}}; !slices.Equal(list.Crumbs, want) {
		t.Errorf("list crumbs: %+v, want %+v", list.Crumbs, want)
	}
	fb := rs.FormBlock(form(rs, probe{}))
	if want := []Link{{Title: "Acme", Href: "/acme"}, {Title: "Accounts", Href: "/x"}, {Title: "Accounts"}}; !slices.Equal(fb.Crumbs, want) {
		t.Errorf("form crumbs: %+v, want %+v", fb.Crumbs, want)
	}

	page := func(crumbs []Link) string {
		w := httptest.NewRecorder()
		RenderPage(w, httptest.NewRequest("GET", "/x", nil), frame, http.StatusOK, "", crumbs, NoticeBlock(nil), fb)
		return w.Body.String()
	}
	if !strings.Contains(page(nil), `href="/acme"`) {
		t.Error("the block's crumbs did not reach the page")
	}
	if body := page([]Link{{Title: "Elsewhere"}}); strings.Contains(body, `href="/acme"`) || !strings.Contains(body, "Elsewhere") {
		t.Error("crumbs given to the page did not win over the block's")
	}
}

// A crumb to the page it is on is only a label: the same trail, drawn on the
// list it leads to, does not link the list to itself.
func TestCrumbToThisPageIsNotALink(t *testing.T) {
	crumbs := []Link{{Title: "Acme", Href: "/acme"}, {Title: "Here"}}
	page := func(path string) string {
		w := httptest.NewRecorder()
		RenderPage(w, httptest.NewRequest("GET", path, nil), frame, http.StatusOK, "t", crumbs, NoticeBlock(&Notice{Title: "n"}))
		return w.Body.String()
	}
	if strings.Contains(page("/acme"), `href="/acme"`) {
		t.Error("the page links to itself in its crumbs")
	}
	if !strings.Contains(page("/acme/1"), `href="/acme"`) {
		t.Error("a crumb to another page lost its link")
	}
	if crumbs[0].Href != "/acme" {
		t.Error("the crumbs handed in were changed")
	}
}

// A field that suggests as you type offers its empty value when it may stay
// empty, the way a select does: an optional form field, and a filter with All.
// A required field does not.
func TestLookupOffersBlankWhenOptional(t *testing.T) {
	store := chooser{"system_type": {{Value: "1", Label: "Olya"}}}
	field := Field[probe]{
		Name: "system_type", LookupChoices: true, LookupLimit: 5, Search: true, Filter: FilterLookup,
		Value: func(p probe) string { return p.SystemType }, Text: func(p probe) string { return p.SystemType },
	}
	form := func(required bool) string {
		f := field
		f.Required = required
		w := httptest.NewRecorder()
		probeRes(f).RenderForm(w, httptest.NewRequest("GET", "/x/1", nil), frame, store, probe{}, false, nil)
		return w.Body.String()
	}
	if !strings.Contains(form(false), `data-blank="`+T("— none —")+`"`) {
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
	RenderPage(w, httptest.NewRequest("GET", "/x", nil), frame, http.StatusOK, "", nil, list)
	if !strings.Contains(w.Body.String(), `data-blank="`+T("All")+`"`) {
		t.Error("a lookup filter offers no All")
	}
}

// A component that failed gives 500 and nothing else: the page is built in a
// buffer, so half of it never reaches an already-sent 200.
func TestHandlerStopsOnError(t *testing.T) {
	broken := func(http.ResponseWriter, *http.Request) (Block, error) {
		return Block{}, errors.New("the database is silent")
	}
	h := Handler(frame,
		block(Block{Name: "list", Data: ListView{Title: "Clients"}}),
		broken,
	)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/clients", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "Clients") {
		t.Error("half a page was sent")
	}
}

// A page is also built past Handler — with a status code and breadcrumbs of its
// own. An empty block must not break that one either: the rule is the same on
// every path.
func TestRenderSkipsEmptyBlock(t *testing.T) {
	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "page.html", Page{Title: "x", Blocks: []Block{
		NoticeBlock(nil),
		{Name: "list", Data: ListView{Title: "the client panel"}},
	}})

	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "the client panel") {
		t.Error("the page did not survive an empty block")
	}
}

// A form does not ask for the CSRF token: the page holds it, and hands it to
// every form drawn on it. Resource.Form never sees a request and could not know
// it; forgetting to pass it by hand is exactly how a form ends up rejected.
func TestPageGivesFormsItsToken(t *testing.T) {
	withToken := func(*http.Request) (Page, error) { return Page{CSRF: "tok123"}, nil }

	w := httptest.NewRecorder()
	RenderPage(w, httptest.NewRequest("GET", "/clients/1", nil), withToken, http.StatusOK,
		"Client", nil, probeRes().FormBlock(FormView{Action: "/clients/1", Submit: "Save"}))

	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	// Looking inside the form itself: the layout has forms of its own (signing
	// out, row actions) and they carry the same token, so a search over the
	// whole page would pass with the block getting nothing.
	body := w.Body.String()
	form := body[strings.Index(body, `<form class="form"`):]
	form = form[:strings.Index(form, "</form>")]
	if !strings.Contains(form, `name="_csrf" value="tok123"`) {
		t.Errorf("the form went out without the token of its page: %s", form)
	}
}

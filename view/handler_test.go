package view

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
		"Client", nil, FormBlock(FormView{Action: "/clients/1", Submit: "Save"}))

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

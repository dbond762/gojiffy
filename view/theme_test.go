package view

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	tailadmin "github.com/dbond762/gojiffy/themes/TailAdmin"
)

// A layer replaces exactly one file and the rest comes from the theme.
// Otherwise the look would be all or nothing: one small change would mean
// copying every template and then following every change made to them in the
// library.
func TestOverrideReplacesOneFile(t *testing.T) {
	t.Cleanup(func() { SetTheme(tailadmin.FS) })

	err := Override(fstest.MapFS{
		"templates/page.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}<p>markup of my own</p>{{end}}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "page.html", Page{Title: "x"})
	body := w.Body.String()

	if !strings.Contains(body, "markup of my own") {
		t.Error("the page did not take the overridden template")
	}
	if !strings.Contains(body, "<!doctype html>") {
		t.Error("the layer wiped the theme layout instead of replacing page.html alone")
	}
	// the partials of the theme reach the set through a glob over the directory:
	// had overlay handed back the topmost directory instead of merging layers,
	// they would not be here.
	if !strings.Contains(body, "/static/app.css") {
		t.Error("the layout and partials of the theme did not reach the set")
	}
}

// A partial of your own must not drag the others away with it: they sit in one
// directory and reach the set through a glob, and a glob reads the directory —
// so the layers have to be merged, or all that is left of the theme is the one
// file that was replaced.
func TestOverrideKeepsSiblingPartials(t *testing.T) {
	t.Cleanup(func() { SetTheme(tailadmin.FS) })

	err := Override(fstest.MapFS{
		"templates/partials/list_footer.html": &fstest.MapFile{
			Data: []byte(`{{define "list-footer"}}<p>a footer of my own</p>{{end}}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "page.html", Page{Title: "x", Blocks: []Block{{
		Name: "list",
		Data: ListView{Table: Table{
			Columns: []Column{{Title: "Name"}},
			Rows:    [][]Cell{{{Text: "a cell from the theme"}}},
		}},
	}}})
	body := w.Body.String()

	if !strings.Contains(body, "a footer of my own") {
		t.Error("the partial of my own was not picked up")
	}
	if !strings.Contains(body, "a cell from the theme") {
		t.Error("the neighbouring partials of the theme went missing: directory layers are not merged")
	}
}

// An incomplete theme is an error handed to the application, not a panic and
// not empty pages: the previous look keeps working.
func TestSetThemeIncomplete(t *testing.T) {
	if err := SetTheme(fstest.MapFS{}); err == nil {
		t.Fatal("an empty theme was accepted")
	}

	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "page.html", Page{Title: "x"})
	if w.Code != http.StatusOK {
		t.Fatalf("the page broke after a theme that did not parse: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<!doctype html>") {
		t.Error("the previous look did not survive")
	}
}

// The address is the application's to pick, and the one thing that can quietly
// come apart is that a page names it in markup while the mux names it in Go.
// So mount somewhere of our own choosing, take the address back off a rendered
// page, and ask the mux for exactly that.
func TestStaticIsWhereThePageLooks(t *testing.T) {
	t.Cleanup(func() { Static("/static/") })

	mux := http.NewServeMux()
	mux.Handle("GET /files/of/the/theme/", Static("/files/of/the/theme/"))

	page := httptest.NewRecorder()
	Render(page, http.StatusOK, "page.html", Page{Title: "x"})

	const mark = `<link rel="stylesheet" href="`
	body := page.Body.String()
	i := strings.Index(body, mark)
	if i < 0 {
		t.Fatal("the page links no stylesheet at all")
	}
	href := body[i+len(mark):]
	href = href[:strings.IndexByte(href, '"')]

	if !strings.HasPrefix(href, "/files/of/the/theme/") {
		t.Errorf("the page still links %s, not where it was mounted", href)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", href, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("the page links %s, the mux answers %d", href, w.Code)
	}
	if w.Body.Len() == 0 {
		t.Errorf("%s came back empty", href)
	}
}

// A prefix given without its trailing slash is the same prefix: the caller
// writes the address twice on that line, once for the mux and once for us, and
// the two need not be spelled identically.
func TestStaticTakesAPrefixEitherWay(t *testing.T) {
	t.Cleanup(func() { Static("/static/") })

	mux := http.NewServeMux()
	mux.Handle("GET /files/", Static("/files"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/files/app.css", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
}

// A notice is a frame from the theme around content of the application's own:
// the kind picks the colour, HTML goes in as markup and not as text. Only the
// frame is the theme's — a field, a button and the script beside them are
// written by the application.
func TestNoticeFramesAppMarkup(t *testing.T) {
	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "page.html", Page{Title: "x", Blocks: []Block{{
		Name: "notice",
		Data: &Notice{Kind: Warning, Title: "mind you", HTML: `<button id="mine">copy</button>`},
	}}})
	body := w.Body.String()

	if !strings.Contains(body, "bg-warning-50") {
		t.Error("the notice did not take the colour of its kind")
	}
	if !strings.Contains(body, `<button id="mine">copy</button>`) {
		t.Error("the markup of the application did not reach the page as markup")
	}
}

// A stylesheet and a script of the application go onto the page each where it
// belongs: the css after the theme's, the script after the markup it drives.
// Markup an application brings with it has to be styled and driven from
// somewhere, and the theme is built without ever seeing it.
func TestPageLinksApplicationFiles(t *testing.T) {
	t.Cleanup(func() { styles, scripts = nil, nil })
	SetStyle("/assets/site.css")
	SetScript("/assets/site.js?v=3")

	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "page.html", Page{Title: "x"})
	body := w.Body.String()

	if !strings.Contains(body, `<link rel="stylesheet" href="/assets/site.css">`) {
		t.Error("the stylesheet is not on the page")
	}
	if !strings.Contains(body, `<script src="/assets/site.js?v=3"></script>`) {
		t.Error("the script is not on the page")
	}
	if strings.Index(body, "/assets/site.css") < strings.Index(body, "/static/app.css") {
		t.Error("the stylesheet went in before the theme, so the theme overrides it")
	}
	if strings.Index(body, "/assets/site.js") < strings.Index(body, "</main>") {
		t.Error("the script went in before the markup it works on")
	}
}

// Each call adds one, so an application collects its files where it happens to
// have them rather than in one list it has to keep whole.
func TestFilesAddUp(t *testing.T) {
	t.Cleanup(func() { styles, scripts = nil, nil })
	SetStyle("/assets/one.css")
	SetStyle("/assets/two.css")

	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "page.html", Page{Title: "x"})
	body := w.Body.String()

	if !strings.Contains(body, "one.css") || !strings.Contains(body, "two.css") {
		t.Fatal("the second call replaced the first instead of adding to it")
	}
	if strings.Index(body, "one.css") > strings.Index(body, "two.css") {
		t.Error("they went onto the page in the wrong order")
	}
}

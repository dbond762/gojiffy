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
	if !strings.Contains(body, "app.css") {
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

// The address of the static files belongs to the application: it says where it
// mounted them, and the templates link them from there. No address of the
// library's own appears anywhere — that is the point of the argument.
func TestStaticServesThemeWhereMounted(t *testing.T) {
	t.Cleanup(func() { Static("/static/") })

	h := Static("/theme/")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/theme/app.css", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Error("empty css")
	}

	page := httptest.NewRecorder()
	Render(page, http.StatusOK, "page.html", Page{Title: "x"})
	if !strings.Contains(page.Body.String(), `href="/theme/app.css"`) {
		t.Error("the page did not link the css where it was mounted")
	}
}

// A stylesheet of the application goes onto the page after the theme's own:
// markup an application brings with it has to be styled from somewhere, and
// the theme's css is built without ever seeing it.
func TestPageLinksApplicationStyles(t *testing.T) {
	t.Cleanup(func() { SetStyles() })
	SetStyles("/assets/dashboard.css")

	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "page.html", Page{Title: "x"})
	body := w.Body.String()

	if !strings.Contains(body, `href="/assets/dashboard.css"`) {
		t.Fatal("the stylesheet of the application is not on the page")
	}
	if strings.Index(body, "/assets/dashboard.css") < strings.Index(body, "app.css") {
		t.Error("it went in before the theme, so the theme overrides it")
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

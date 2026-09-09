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
// copying all ten templates and then following every change made to them in
// the library.
func TestOverrideReplacesOneFile(t *testing.T) {
	t.Cleanup(func() { SetTheme(tailadmin.FS) })

	err := Override(fstest.MapFS{
		"templates/list.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}<p>markup of my own</p>{{end}}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "list.html", Page{Title: "x"})
	body := w.Body.String()

	if !strings.Contains(body, "markup of my own") {
		t.Error("the page did not take the overridden template")
	}
	if !strings.Contains(body, "<!doctype html>") {
		t.Error("the layer wiped the theme layout instead of replacing list.html alone")
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
	Render(w, http.StatusOK, "list.html", Page{Title: "x", Data: ListView{
		Table: Table{
			Columns: []Column{{Title: "Name"}},
			Rows:    [][]Cell{{{Text: "a cell from the theme"}}},
		},
	}})
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
	Render(w, http.StatusOK, "list.html", Page{Title: "x", Data: ListView{}})
	if w.Code != http.StatusOK {
		t.Fatalf("the page broke after a theme that did not parse: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<!doctype html>") {
		t.Error("the previous look did not survive")
	}
}

// Static files are served by the library and not by the application: the
// /static/app.css address is baked into the templates.
func TestStaticServesTheme(t *testing.T) {
	w := httptest.NewRecorder()
	Static().ServeHTTP(w, httptest.NewRequest("GET", "/static/app.css", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("code %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Error("empty css")
	}
}

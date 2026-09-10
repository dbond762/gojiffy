package view

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"maps"
	"net/http"
	"slices"
	"strings"

	tailadmin "github.com/dbond762/gojiffy/themes/TailAdmin"
)

// A theme is a filesystem with templates/ and static/ at its root. There is
// deliberately no type for it: fs.FS already describes everything needed, and
// a wrapper of our own would force every theme to know about the library.

// layers — the theme and whatever the application laid on top of it. We look
// from the top down, so files are replaced one at a time rather than the whole
// theme at once.
var layers []fs.FS

// SetTheme replaces the theme entirely. fsys is a filesystem with templates/
// and static/ at its root, see themes/TailAdmin. Call it at startup, before the
// first render, as with SetAppName.
//
// An error means the theme is incomplete or a template in it did not parse; the
// theme in use stays as it was and pages keep working. It is also what puts
// everything back after Override: SetTheme(tailadmin.FS).
func SetTheme(fsys fs.FS) error { return apply([]fs.FS{fsys}) }

// Override lays files over the current theme: whatever is in fsys comes from
// there, the rest stays with the theme. The shape is the same (templates/...,
// static/...), and a file has to be replaced at the same path.
//
// It can be called more than once, each layer covering the ones before it.
func Override(fsys fs.FS) error { return apply(slices.Concat(layers, []fs.FS{fsys})) }

// Static serves the static/ of the theme. Mount it at /static/:
//
//	mux.Handle("GET /static/", view.Static())
//
// That address is baked into the templates (<link rel="stylesheet"
// href="/static/app.css">), so both ends of the arrangement are held by the
// library and changing the theme leaves routing alone. The layers are read on
// every request, so the order against SetTheme does not matter.
func Static() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.FileServerFS(overlay(layers)).ServeHTTP(w, r)
	})
}

func init() {
	// The default theme is embedded in the binary: a broken template here is a
	// build error of the library and not of an application, so we fail at once,
	// the way template.Must used to.
	if err := SetTheme(tailadmin.FS); err != nil {
		panic(err)
	}
}

// apply parses the templates afresh and swaps them in one go. Afresh because an
// html/template set cannot be added to after parsing, and the old one lives on
// as long as a Render already under way still refers to it.
func apply(l []fs.FS) error {
	fsys := overlay(l)

	built := make(map[string]*template.Template, len(pages)+1)
	for _, name := range pages {
		// render draws a block by the name of its partial. It closes over a
		// variable filled in below rather than being added afterwards, because
		// Funcs has to be called before parsing. The output is html/template's
		// own, already escaped, so HTML is the right type to hand back.
		var set *template.Template
		fns := maps.Clone(funcs)
		fns["render"] = func(b Block) (template.HTML, error) {
			// A block with no name is one that did not happen: a notice with
			// nothing to say. Skipped here rather than in Handler, because a
			// page is also built by hand and the rule should be in one place.
			if b.Name == "" {
				return "", nil
			}
			var buf bytes.Buffer
			if err := set.ExecuteTemplate(&buf, b.Name, b.Data); err != nil {
				return "", err
			}
			return template.HTML(buf.String()), nil
		}

		tpl, err := template.New(name).Funcs(fns).ParseFS(fsys,
			"templates/layout.html", "templates/partials/*.html", "templates/"+name)
		if err != nil {
			return fmt.Errorf("template %s: %w", name, err)
		}
		set = tpl
		built[name] = tpl
	}
	// The sign-in page is parsed without the layout: it is a document of its own
	// and its entry point is named after the file, see Render.
	login, err := template.New("login.html").Funcs(funcs).ParseFS(fsys,
		"templates/partials/*.html", "templates/login.html")
	if err != nil {
		return fmt.Errorf("template login.html: %w", err)
	}
	built["login.html"] = login

	// The assignment comes last: a theme that did not parse must not leave the
	// application with no look at all — that is the whole point of the error
	// this returns.
	layers, templates = l, built
	return nil
}

// overlay — a stack of filesystems: the theme at the bottom, the layers of the
// application above it. We implement fs.FS instead of parsing templates
// ourselves: that way ParseFS, fs.Glob and http.FileServerFS handle the layers
// on their own, and the ParseFS calls in apply stay exactly what they were
// before themes existed.
type overlay []fs.FS

func (o overlay) Open(name string) (fs.File, error) {
	for i := len(o) - 1; i >= 0; i-- {
		if f, err := o[i].Open(name); err == nil {
			return f, nil
		}
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// ReadDir is needed for the sake of partials/*.html: without it fs.Glob would
// read the directory of one layer and lose the partials of the rest. Files
// sharing a name collapse into the topmost one, which is also the one that
// opens for reading.
func (o overlay) ReadDir(name string) ([]fs.DirEntry, error) {
	var all []fs.DirEntry
	seen := map[string]bool{}
	for i := len(o) - 1; i >= 0; i-- {
		entries, err := fs.ReadDir(o[i], name)
		if err != nil {
			continue // this layer has no such directory, so another one does
		}
		for _, e := range entries {
			if !seen[e.Name()] {
				seen[e.Name()] = true
				all = append(all, e)
			}
		}
	}
	if all == nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	// fs.ReadDir is required to return sorted entries, and here that is not a
	// formality: the parse order decides whose {{define}} comes last.
	slices.SortFunc(all, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return all, nil
}

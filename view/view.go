package view

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
)

// pages — the pages built on the shared frame of layout.html. The list is
// fixed: which pages exist is decided by the library through its own calls to
// Render, not by the theme.
var pages = []string{"list.html", "form.html"}

// templates — the parsed theme, one set per page. Filled in by apply, see
// theme.go: list.html and form.html both define "content", so they cannot be
// parsed into one set.
var templates = map[string]*template.Template{}

// appName — what the application is called: the caption at the top of the
// sidebar and the tail of the tab title. Templates take it through the {{app}}
// function rather than out of the page data: the name is one per process while
// pages are many, and the sign-in page is drawn with no wrapper at all.
var appName = "Admin"

// SetAppName sets the name of the application. Call it before the first
// render, once, at startup.
func SetAppName(name string) { appName = name }

var funcs = template.FuncMap{
	"app":  func() string { return appName },
	"lang": func() string { return lib.Language() },
	"t":    t,
}

// Render builds the page in a buffer so that a template error cannot end up
// inside an already-sent 200.
func Render(w http.ResponseWriter, status int, name string, data any) {
	tpl, ok := templates[name]
	if !ok {
		log.Printf("no template %s", name)
		http.Error(w, t("internal error"), http.StatusInternalServerError)
		return
	}

	entry := "layout"
	if name == "login.html" {
		entry = name
	}

	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, entry, data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, t("internal error"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Admin pages are not kept in browser history: no-store switches off the
	// bfcache too, or the back button would show a page again with everything
	// that was on it. What is meant to be shown once — a secret, a confirmation
	// — would otherwise come back from the cache any number of times.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

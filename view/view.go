package view

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
)

// pages — the pages built on the shared frame of layout.html. One is enough:
// what a page shows is decided by the blocks handed to it, not by a template
// per kind of page.
var pages = []string{"page.html"}

// templates — the parsed theme, one set per page. Filled in by apply, see
// theme.go: sign-in is a document of its own and cannot share a set with the
// rest.
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
	// asset — the address of a file of the theme, wherever it was mounted;
	// styles and scripts — what the application added of its own, see SetStyle
	// and SetScript.
	"asset":   func(name string) string { return staticPath + name },
	"styles":  func() []string { return styles },
	"scripts": func() []string { return scripts },
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
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	// Admin pages are not kept in browser history: no-store switches off the
	// bfcache too, or the back button would show a page again with everything
	// that was on it. What is meant to be shown once — a secret, a confirmation
	// — would otherwise come back from the cache any number of times.
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	// A page of the admin panel is never drawn inside someone else's page, where
	// a click on a decoy would land on a button here; its forms post nowhere but
	// here. Scripts are left alone: the theme's own are inline.
	h.Set("Content-Security-Policy", "frame-ancestors 'none'; form-action 'self'; base-uri 'self'; object-src 'none'")
	// Addresses of the panel do not leave it: another site learns nothing of
	// them from a link followed out of here.
	h.Set("Referrer-Policy", "same-origin")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

// Component builds one block of a page for one request. It takes a
// ResponseWriter because a block can have a side effect of its own: a one-time
// notice puts out the cookie it came from.
type Component func(http.ResponseWriter, *http.Request) (Block, error)

// Frame — the wrapping of a page that only an application knows: who signed in,
// what menu they get, which csrf token their forms carry. Everything else on
// the page comes from the components.
type Frame func(*http.Request) (Page, error)

// Handler draws a page out of components, in the order they are given.
//
//	mux.HandleFunc("GET /clients", Handler(app.Frame, app.Notice, app.ClientsList))
//
// A block with no name is skipped, so a component with nothing to show returns
// an empty one instead of a special case. The page title comes from the first
// block that names one, unless the frame set it already.
func Handler(frame Frame, comps ...Component) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := frame(r)
		if err != nil {
			serverError(w, err)
			return
		}
		blocks := make([]Block, 0, len(comps))
		for _, c := range comps {
			b, err := c(w, r)
			if err != nil {
				serverError(w, err)
				return
			}
			blocks = append(blocks, b)
		}
		draw(w, r, page, http.StatusOK, blocks)
	}
}

// RenderPage draws a page that Handler cannot: one with a status code of its
// own (422 under a form that did not validate), or a 404 decided before any of
// this. The title and the crumbs come from the blocks, as on any page; a title
// or crumbs given here win over them — the way to put a trail of its own on one
// page. Crumbs that are empty but not nil leave just the title in the header.
//
//	view.RenderPage(w, r, a.Frame, 422, "", nil, notice, rs.FormBlock(form))
func RenderPage(w http.ResponseWriter, r *http.Request, frame Frame, status int,
	title string, crumbs []Link, blocks ...Block) {

	page, err := frame(r)
	if err != nil {
		serverError(w, err)
		return
	}
	if title != "" {
		page.Title = title
	}
	if crumbs != nil {
		page.Crumbs = crumbs
	}
	draw(w, r, page, status, blocks)
}

// draw is where a page comes together whichever way it was built: nameless
// blocks fall away, the title and the crumbs are taken from the first block
// that has them if nothing else set them, a crumb that leads to the page it is
// on is drawn as plain text, and every form on the page gets the token of that
// page.
func draw(w http.ResponseWriter, r *http.Request, page Page, status int, blocks []Block) {
	for _, b := range blocks {
		if b.Name == "" {
			continue
		}
		if page.Title == "" {
			page.Title = b.Title
		}
		if page.Crumbs == nil {
			page.Crumbs = b.Crumbs
		}
		// A form asks for the token no more than any other block does: the page
		// holds it, and the form is told on the way in. Resource.Form has never
		// seen a request and cannot know it.
		if v, ok := b.Data.(FormView); ok {
			v.CSRF = page.CSRF
			b.Data = v
		}
		page.Blocks = append(page.Blocks, b)
	}
	// One trail serves a list and its form: the client above the tokens is a
	// way back from a token, but only a label on the client's own list. A new
	// slice, not the one handed in — that one may be a resource's own.
	crumbs := make([]Link, len(page.Crumbs))
	for i, c := range page.Crumbs {
		if c.Href == r.URL.Path {
			c.Href = ""
		}
		crumbs[i] = c
	}
	page.Crumbs = crumbs
	Render(w, status, "page.html", page)
}

// NoticeBlock puts a notice above the rest of the page. A nil notice gives an
// empty block, which the page skips — so the caller has nothing to check.
func NoticeBlock(n *Notice) Block {
	if n == nil {
		return Block{}
	}
	return Block{Name: "notice", Data: n}
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("page: %v", err)
	http.Error(w, t("internal error"), http.StatusInternalServerError)
}

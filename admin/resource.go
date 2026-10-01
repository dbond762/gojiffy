// Package admin — the sections of an admin panel: an entity described once as
// a Resource — its fields, where they show and how they are checked — gives the
// list with its filters, the form and the parsing of what was sent. CRUD makes
// a whole section of it: the handlers, the addresses, the store behind them.
// What it all is drawn with is package view.
package admin

import (
	"log"
	"net/http"

	"github.com/dbond762/gojiffy"
	"github.com/dbond762/gojiffy/view"
)

// Resource — the description of an entity: some fields show up in the table,
// others in the form, others in both. The machinery below is the same for every
// entity.
//
// T is the record as a page sees it — usually the model itself. Where a form
// needs more than the model holds (a password, say), T is a wider record and
// the list is built by MapList.
type Resource[T any] struct {
	Path         string         // /users — address of the list and action of the search form
	Title        string         // Users — heading of the list
	EditTitle    func(T) string // User: Olya — heading of the edit form; nil takes Title instead
	NewTitle     string         // New user — heading of the create form, when Form draws it creating
	Empty        string         // the text of an empty list; empty is "Nothing found"
	PerPage      int            // rows per page; 0 means gojiffy.PerPageDefault
	DefaultOrder gojiffy.Order  // sorting when the request has none or names an unknown field
	Href         func(T) string // address of a record: action of the edit form
	Fields       []Field[T]
	Actions      []Action[T]
	Header       []HeaderLink // buttons above the list, in order — see HeaderLink

	// Crumbs — the trail above this resource, for one nested in another: the
	// author a list of articles belongs to. The list and the form put themselves
	// after it, and a root resource leaves it empty. Like Path, it may be filled
	// in once the parent is known.
	Crumbs []view.Link
}

// For hands back a copy of the description without whatever the user has no
// permission for. A handler works with that copy, so a hidden field does not
// show up in the list, does not appear in the form and is not accepted out of a
// request.
func (rs Resource[T]) For(perms gojiffy.Perms) Resource[T] {
	fields := make([]Field[T], 0, len(rs.Fields))
	for _, f := range rs.Fields {
		if f.Permission == "" || perms.Can(f.Permission) {
			fields = append(fields, f)
		}
	}
	actions := make([]Action[T], 0, len(rs.Actions))
	for _, a := range rs.Actions {
		if a.Permission == "" || perms.Can(a.Permission) {
			actions = append(actions, a)
		}
	}
	header := make([]HeaderLink, 0, len(rs.Header))
	for _, h := range rs.Header {
		if h.Permission == "" || perms.Can(h.Permission) {
			header = append(header, h)
		}
	}
	rs.Fields, rs.Actions, rs.Header = fields, actions, header
	return rs
}

// editTitle is EditTitle with a fallback: an application that never edits — a
// log of events, say — has no per-record heading to write, and Title, the one
// heading it does have, is a plain enough thing to fall back on.
func (rs Resource[T]) editTitle(item T) string {
	if rs.EditTitle == nil {
		return rs.Title
	}
	return rs.EditTitle(item)
}

// serverError — a failure that is no one's to fix but the developer's: logged
// whole, answered with a plain 500 in the language of the panel.
func serverError(w http.ResponseWriter, err error) {
	log.Printf("admin: %v", err)
	http.Error(w, view.T("internal error"), http.StatusInternalServerError)
}

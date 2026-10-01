package admin

import (
	"github.com/dbond762/gojiffy/view"
)

// Action — an action on a record: its address is worked out from the record
// itself. By default actions live together in the rightmost column of the list
// (one draws as a button, several as a dropdown). Column moves an action into a
// column of its own — for what should be in plain sight rather than tucked
// under Actions.
type Action[T any] struct {
	Title      string
	TitleFor   func(T) string // caption depends on the record; nil uses Title everywhere
	Href       func(T) string
	Permission string         // "" is open to everyone
	Column     bool           // a column of its own instead of the shared bunch
	Post       bool           // changes state: a button in a form, not a link
	Confirm    func(T) string // the confirmation text; nil or "" asks nothing
	Hide       func(T) bool   // true leaves the action off that row; nil shows it everywhere
}

// rowAction turns the declaration into what the template draws.
func (a Action[T]) rowAction(item T) view.RowAction {
	r := view.RowAction{Title: a.Title, Href: a.Href(item), Post: a.Post}
	if a.TitleFor != nil {
		r.Title = a.TitleFor(item)
	}
	if a.Confirm != nil {
		r.Confirm = a.Confirm(item)
	}
	return r
}

// rowActions — the actions a row shows; hidden ones leave the cell empty.
func rowActions[T any](item T, actions ...Action[T]) []view.RowAction {
	out := make([]view.RowAction, 0, len(actions))
	for _, a := range actions {
		if a.Hide == nil || !a.Hide(item) {
			out = append(out, a.rowAction(item))
		}
	}
	return out
}

// HeaderLink — a button above the list, not tied to any one record: "New",
// say, or something an application adds of its own. Filtered by Permission
// exactly like a row Action — an unmet permission drops the button, not the
// text under it.
//
// Href is resolved against the resource's own Path when the list is drawn,
// not when the resource is declared: a resource may still get its Path
// filled in afterward, the way an author's articles are given their own address
// only once the author is known.
type HeaderLink struct {
	Title, Href string // Href, e.g. "/new", goes after Path
	Permission  string // "" means anyone who sees the list
}

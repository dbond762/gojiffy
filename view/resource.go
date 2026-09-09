package view

import (
	"net/http"
	"strings"

	"github.com/dbond762/gojiffy"
)

// Req — when a field is required.
type Req int

const (
	No       Req = iota // never
	Yes                 // always
	OnCreate            // only when creating (a password may be changed or left alone)
)

// Field — a field of an entity in full: both a list column and a form field.
// An empty Title keeps it out of the list, an empty Label out of the form.
type Field[T any] struct {
	Name string // parameter name: search[Name] in a list, Name in a form

	// list
	Title  string
	Class  string // css class of the column, for its width say
	Search bool
	// SearchChoices, when set, draws the filter as a <select> with a fixed list
	// (with All first) instead of a text field — for enumerations and boolean
	// flags, where a substring match is the wrong question.
	SearchChoices []Option
	Sort          bool           // the heading becomes a sorting link
	Text          func(T) string // the cell value, already formatted
	Href          func(T) string // when set, the cell becomes a link
	Bool          func(T) bool   // when set, the cell draws a tick or a cross instead of Text

	// form
	Label        string
	Type         string
	Help         string // hint under the field
	HelpEdit     string // hint when editing, if it differs
	Required     Req
	Options      string                       // key of an option set, making it a <select>
	Value        func(T) string               // the field value; nil takes Text instead
	Parse        func(*T, string, bool) error // parsing and validation; the bool means creating
	Readonly     func(T) bool                 // true only shows the field; what to accept is still up to Parse
	Autocomplete string

	Permission string // the field is visible only with this permission, in list and form alike
}

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
}

// rowAction turns the declaration into what the template draws.
func (a Action[T]) rowAction(item T) RowAction {
	r := RowAction{Title: a.Title, Href: a.Href(item), Post: a.Post}
	if a.TitleFor != nil {
		r.Title = a.TitleFor(item)
	}
	if a.Confirm != nil {
		r.Confirm = a.Confirm(item)
	}
	return r
}

func (f Field[T]) inList() bool { return f.Title != "" }
func (f Field[T]) inForm() bool { return f.Label != "" }

// Link — a button in the panel header.
type Link struct{ Title, Href string }

// Resource — the description of an entity: some fields show up in the table,
// others in the form, others in both. The machinery below is the same for every
// entity.
//
// M is the model as the database hands it over; T is the form record (the model
// plus what is not in it, a password say). Wrap turns the first into the second
// for the list.
type Resource[M, T any] struct {
	Path         string         // /users — address of the list and action of the search form
	Title        string         // Users — heading of the list
	One          string         // User — heading of the edit form
	NewTitle     string         // New user — heading of the create form
	Empty        string         // the text of an empty list
	PerPage      int            // rows per page; 0 means gojiffy.PerPageDefault
	DefaultOrder gojiffy.Order  // sorting when the request has none or names an unknown field
	Href         func(T) string // address of a record: action of the edit form
	Wrap         func(M) T
	Fields       []Field[T]
	Actions      []Action[T]
}

// For hands back a copy of the description without whatever the user has no
// permission for. A handler works with that copy, so a hidden field does not
// show up in the list, does not appear in the form and is not accepted out of a
// request.
func (rs Resource[M, T]) For(perms gojiffy.Perms) Resource[M, T] {
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
	rs.Fields, rs.Actions = fields, actions
	return rs
}

// Paging — which page the request asks for.
func (rs Resource[M, T]) Paging(r *http.Request) gojiffy.Paging {
	return ParsePaging(r.URL.Query(), rs.PerPage)
}

// ParseOrder takes the sorting out of the request if that field was declared
// sortable. Otherwise DefaultOrder: both when ?sort= is empty and when it names
// something else.
func (rs Resource[M, T]) ParseOrder(r *http.Request) gojiffy.Order {
	o := ParseOrder(r.URL.Query())
	for _, f := range rs.Fields {
		if f.Sort && f.inList() && f.Name == o.Field {
			return o
		}
	}
	return rs.DefaultOrder
}

// ——— list ———

// ParseSearch takes only fields marked Search out of the request: the
// declaration is the validation.
func (rs Resource[M, T]) ParseSearch(r *http.Request) gojiffy.Search {
	q := r.URL.Query()
	s := gojiffy.Search{}
	for _, f := range rs.Fields {
		if !f.Search || !f.inList() {
			continue
		}
		if v := strings.TrimSpace(q.Get("search[" + f.Name + "]")); v != "" {
			s[f.Name] = v
		}
	}
	return s
}

// List builds a page of the list: headings, filter fields, rows in the order
// the fields were declared, and a footer with the counter and page links.
func (rs Resource[M, T]) List(items []M, total int, p gojiffy.Paging, s gojiffy.Search, o gojiffy.Order) ListView {
	t := Table{Empty: rs.Empty}

	filtered := false
	for _, f := range rs.Fields {
		if !f.inList() {
			continue
		}
		col := Column{Title: f.Title, Class: f.Class}
		if f.Search {
			col.Search, col.Query = f.Name, s[f.Name]
			filtered = true
			if len(f.SearchChoices) > 0 {
				col.SearchOptions = selected(withAll(f.SearchChoices), s[f.Name])
			}
		}
		if f.Sort {
			next := gojiffy.Order{Field: f.Name}
			if o.Field == f.Name {
				col.SortDir = "asc"
				if o.Desc {
					col.SortDir = "desc"
				}
				next.Desc = !o.Desc // clicking again flips the order
			}
			// sorting resets the page: there is nothing to do on page seven of a new order
			col.SortHref = listURL(rs.Path, s, next, 1)
		}
		t.Columns = append(t.Columns, col)
	}

	// actions of their own go into their own columns, the rest follow in one bunch
	var own, menu []Action[T]
	for _, a := range rs.Actions {
		if a.Column {
			own = append(own, a)
		} else {
			menu = append(menu, a)
		}
	}
	for range own {
		t.Columns = append(t.Columns, Column{Class: "col-actions"})
	}
	if len(menu) > 0 {
		t.Columns = append(t.Columns, Column{Class: "col-actions"})
	}
	// with no filter at all the search row is pointless, and so is the form around the table
	if filtered {
		t.Action = rs.Path
		t.Columns[len(t.Columns)-1].Actions = true
	}

	for _, item := range items {
		row := rs.Wrap(item)
		cells := make([]Cell, 0, len(t.Columns))
		for _, f := range rs.Fields {
			if !f.inList() {
				continue
			}
			var cell Cell
			switch {
			case f.Bool != nil:
				cell = Cell{Text: "✓", Class: "bool-yes"}
				if !f.Bool(row) {
					cell = Cell{Text: "✗", Class: "bool-no"}
				}
			default:
				cell = Cell{Text: f.Text(row)}
			}
			if f.Href != nil {
				cell.Href = f.Href(row)
			}
			cells = append(cells, cell)
		}
		for _, a := range own {
			cells = append(cells, Cell{Actions: []RowAction{a.rowAction(row)}})
		}
		if len(menu) > 0 {
			actions := make([]RowAction, 0, len(menu))
			for _, a := range menu {
				actions = append(actions, a.rowAction(row))
			}
			cells = append(cells, Cell{Actions: actions})
		}
		t.Rows = append(t.Rows, cells)
	}

	var new_ *Link
	if rs.NewTitle != "" {
		new_ = &Link{Title: rs.NewTitle, Href: rs.Path + "/new"}
	}
	return ListView{
		Title:  rs.Title,
		New:    new_,
		Table:  t,
		Footer: listFooter(rs.Path, s, o, p, total),
	}
}

// ——— form ———

// Parse reads the request field by field. creating changes how strict that is:
// when creating, a required field cannot be empty.
func (rs Resource[M, T]) Parse(r *http.Request, item T, creating bool) (T, map[string]string) {
	errs := map[string]string{}
	for _, f := range rs.Fields {
		if !f.inForm() || f.Parse == nil {
			continue
		}
		if err := f.Parse(&item, r.PostFormValue(f.Name), creating); err != nil {
			errs[f.Name] = err.Error()
		}
	}
	return item, errs
}

// Form builds the form for the template: values out of the record, errors under
// the fields, options for a select out of opts.
func (rs Resource[M, T]) Form(item T, creating bool, opts Options, errs map[string]string, csrf string) FormView {
	v := FormView{CancelURL: rs.Path, CSRF: csrf}
	if creating {
		v.Title, v.Submit, v.Action = rs.NewTitle, t("Create"), rs.Path
	} else {
		v.Title, v.Submit, v.Action = rs.One, t("Save"), rs.Href(item)
	}

	for _, f := range rs.Fields {
		if !f.inForm() {
			continue
		}
		fv := FieldView{
			Name:         f.Name,
			Label:        f.Label,
			Type:         f.Type,
			Help:         f.Help,
			Autocomplete: f.Autocomplete,
			Required:     f.Required == Yes || (creating && f.Required == OnCreate),
			Error:        errs[f.Name],
		}
		if !creating && f.HelpEdit != "" {
			fv.Help = f.HelpEdit
		}
		if f.Readonly != nil {
			fv.Readonly = f.Readonly(item)
		}
		switch {
		case f.Value != nil:
			fv.Value = f.Value(item)
		case f.Text != nil:
			fv.Value = f.Text(item)
		}
		if f.Options != "" {
			fv.Options = selected(opts[f.Options], fv.Value)
			fv.Value = ""
		}
		v.Fields = append(v.Fields, fv)
	}
	return v
}

// Options — option sets for a <select> that are known only at run time (roles
// out of the database and the like): the key matches Field.Options.
type Options map[string][]Option

func selected(opts []Option, value string) []Option {
	out := make([]Option, len(opts))
	for i, o := range opts {
		o.Selected = o.Value == value
		out[i] = o
	}
	return out
}

// withAll puts All first (an empty value, meaning the filter is off).
func withAll(opts []Option) []Option {
	return append([]Option{{Label: t("All")}}, opts...)
}

// ListView — what goes into the list.html template.
type ListView struct {
	Title  string
	New    *Link
	Notice *Notice // a highlighted block above the panel, see form.html
	Table  Table
	Footer ListFooter
}

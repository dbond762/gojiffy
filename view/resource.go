package view

import (
	"net/http"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/dbond762/gojiffy"
)

// FieldErrors — the words for a check that failed. An empty one takes the
// library's own message in the language of the page.
type FieldErrors struct {
	Required, Min, Max string
}

// Field — a field of an entity in full. By default it is in both places at
// once, a column of the list and a field of the form, because most of them
// are: ListOnly and EditOnly are for the few that are not.
type Field[T any] struct {
	Name string // parameter name: search[Name] in a list, Name in a form

	// Caption — the heading of the column and, unless Label says otherwise, the
	// label of the form field: one word for one field, said once.
	Caption  string
	ListOnly bool // a column and nothing more: an id, a date the record carries
	EditOnly bool // a form field and nothing more: a password, a choice from a reference

	// list
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
	Label    string // when the form needs other words than the list; empty takes Caption
	Type     string
	Help     string // hint under the field
	HelpEdit string // hint when editing, if it differs
	Options  string // key of an option set, making it a <select>
	Value    func(T) string
	Readonly func(T) bool // true only shows the field; what to accept is still up to Parse
	// Autofill — the autocomplete attribute of the input: what a browser or a
	// password manager may fill in there ("username", "new-password", "off").
	Autofill string

	// Checks made before Parse, on the value with its spaces trimmed. Length
	// is in characters, not bytes, and is checked only on a value that is
	// there: whether it may be empty at all is Required's question.
	NoTrim   bool // keep the spaces: a password is what it is, spaces and all
	Required bool
	Min, Max int // 0 means no limit
	Errors   FieldErrors

	// The value goes into the string field of the record that Name refers to
	// (status into Status, system_type into SystemType) before Parse is called,
	// so a field that only stores what came in needs no Parse at all. Parse is
	// for the rest: a date, a number, a rule of its own — and it may overwrite
	// what was stored. NoSet leaves the record alone for Parse to decide, for a
	// rule that has to look at what the record held before.
	NoSet bool
	Parse func(*T, string, bool) error // the bool means creating

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

func (f Field[T]) inList() bool { return !f.EditOnly }
func (f Field[T]) inForm() bool { return !f.ListOnly }

// label — what the form calls the field. Saying it twice is the common case,
// so Caption stands in and Label is only for when the two must differ.
func (f Field[T]) label() string {
	if f.Label != "" {
		return f.Label
	}
	return f.Caption
}

// Link — a button in the panel header.
type Link struct{ Title, Href string }

// HeaderLink — a button above the list, not tied to any one record: "New",
// say, or something an application adds of its own. Filtered by Permission
// exactly like a row Action — an unmet permission drops the button, not the
// text under it.
//
// Href is resolved against the resource's own Path when the list is drawn,
// not when the resource is declared: a resource may still get its Path
// filled in afterward, the way a client's tokens are given their own address
// only once the client is known.
type HeaderLink struct {
	Title, Href string // Href, e.g. "/new", goes after Path
	Permission  string // "" means anyone who sees the list
}

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

// Paging — which page the request asks for.
func (rs Resource[T]) Paging(r *http.Request) gojiffy.Paging {
	return ParsePaging(r.URL.Query(), rs.PerPage)
}

// ParseOrder takes the sorting out of the request if that field was declared
// sortable. Otherwise DefaultOrder: both when ?sort= is empty and when it names
// something else.
func (rs Resource[T]) ParseOrder(r *http.Request) gojiffy.Order {
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
func (rs Resource[T]) ParseSearch(r *http.Request) gojiffy.Search {
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
func (rs Resource[T]) List(items []T, total int, p gojiffy.Paging, s gojiffy.Search, o gojiffy.Order) ListView {
	empty := rs.Empty
	if empty == "" {
		empty = t("Nothing found")
	}

	tbl := Table{Empty: empty}

	filtered := false
	for _, f := range rs.Fields {
		if !f.inList() {
			continue
		}
		col := Column{Title: f.Caption, Class: f.Class}
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
		tbl.Columns = append(tbl.Columns, col)
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
		tbl.Columns = append(tbl.Columns, Column{Class: "col-actions"})
	}
	if len(menu) > 0 {
		tbl.Columns = append(tbl.Columns, Column{Class: "col-actions"})
	}
	// with no filter at all the search row is pointless, and so is the form around the table
	if filtered {
		tbl.Action = rs.Path
		tbl.Columns[len(tbl.Columns)-1].Actions = true
	}

	for _, row := range items {
		cells := make([]Cell, 0, len(tbl.Columns))
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
		tbl.Rows = append(tbl.Rows, cells)
	}

	header := make([]Link, 0, len(rs.Header))
	for _, h := range rs.Header {
		header = append(header, Link{Title: h.Title, Href: rs.Path + h.Href})
	}
	return ListView{
		Title:  rs.Title,
		Header: header,
		Table:  tbl,
		Footer: listFooter(rs.Path, s, o, p, total),
	}
}

// ——— form ———

// Parse reads the request field by field: trims the value, stores it in the
// record, makes the declared checks and only then calls the field's own Parse.
// The value is stored even when a check fails — the form comes back with what
// was typed, not with what was there before. A field that fails a check does
// not get to Parse: one error per field is enough, and the first is the one to
// fix.
func (rs Resource[T]) Parse(r *http.Request, item T, creating bool) (T, map[string]string) {
	errs := map[string]string{}
	for _, f := range rs.Fields {
		if !f.inForm() {
			continue
		}
		v := r.PostFormValue(f.Name)
		if !f.NoTrim {
			v = strings.TrimSpace(v)
		}
		// a readonly field is not taken from the request: the input only shows it
		if !f.NoSet && (f.Readonly == nil || !f.Readonly(item)) {
			set(&item, f.Name, v)
		}
		if msg := f.check(v); msg != "" {
			errs[f.Name] = msg
			continue
		}
		if f.Parse != nil {
			if err := f.Parse(&item, v, creating); err != nil {
				errs[f.Name] = err.Error()
			}
		}
	}
	return item, errs
}

// check makes the declared checks and gives back what went wrong, or "".
func (f Field[T]) check(v string) string {
	n := utf8.RuneCountInString(v)
	switch {
	case v == "" && f.Required:
		return or(f.Errors.Required, t("Fill in this field"))
	case v != "" && f.Min > 0 && n < f.Min:
		return or(f.Errors.Min, t(minKey, f.Min))
	case f.Max > 0 && n > f.Max:
		return or(f.Errors.Max, t(maxKey, f.Max))
	}
	return ""
}

func or(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// set puts v into the string field of the record that name refers to: the
// underscores dropped and the case ignored, so role_id finds RoleID as well as
// system_type finds SystemType, embedded structs included. Anything that is
// not a string — a date, a number, a flag — is left for Parse to convert, and
// so is a record that has no such field.
func set[T any](item *T, name, v string) {
	rv := reflect.ValueOf(item).Elem()
	if rv.Kind() != reflect.Struct {
		return
	}
	want := strings.ReplaceAll(name, "_", "")
	sf, ok := rv.Type().FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, want) })
	if !ok {
		return
	}
	// FieldByIndexErr, not FieldByIndex: a nil embedded pointer on the way to
	// the field would otherwise panic
	fv, err := rv.FieldByIndexErr(sf.Index)
	if err != nil || !fv.CanSet() || fv.Kind() != reflect.String {
		return
	}
	fv.SetString(v)
}

// Form builds the form for the template: values out of the record, errors under
// the fields, options for a select out of opts. The CSRF token is not asked for
// here — a page puts it into every form it draws, see Handler and RenderPage.
func (rs Resource[T]) Form(item T, creating bool, opts Options, errs map[string]string) FormView {
	v := FormView{CancelURL: rs.Path}
	if creating {
		v.Title, v.Submit, v.Action = rs.NewTitle, t("Create"), rs.Path
	} else {
		v.Title, v.Submit, v.Action = rs.editTitle(item), t("Save"), rs.Href(item)
	}

	for _, f := range rs.Fields {
		if !f.inForm() {
			continue
		}
		fv := FieldView{
			Name:     f.Name,
			Label:    f.label(),
			Type:     f.Type,
			Help:     f.Help,
			Autofill: f.Autofill,
			Required: f.Required,
			Error:    errs[f.Name],
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

// editTitle is EditTitle with a fallback: an application that never edits — a
// log of events, say — has no per-record heading to write, and Title, the one
// heading it does have, is a plain enough thing to fall back on.
func (rs Resource[T]) editTitle(item T) string {
	if rs.EditTitle == nil {
		return rs.Title
	}
	return rs.EditTitle(item)
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
	Header []Link
	Table  Table
	Footer ListFooter
}

// ListBlock is the whole of a list page: it takes the filters, the sorting and
// the page out of the request, asks the store for that page of records and
// builds the block the theme draws.
func (rs Resource[T]) ListBlock(r *http.Request, store gojiffy.Lister[T]) (Block, error) {
	return MapList(rs, r, store, func(item T) T { return item })
}

// MapList is ListBlock where the page record is wider than the model — a
// password, a computed field: the store hands out models, f makes records of
// them.
//
//	view.MapList(rs, r, db.Users(), func(u models.User) UserForm {
//		return UserForm{User: u}
//	})
func MapList[M, T any](rs Resource[T], r *http.Request, store gojiffy.Lister[M], f func(M) T) (Block, error) {
	s, o, p := rs.ParseSearch(r), rs.ParseOrder(r), rs.Paging(r)

	models, total, err := store.List(r.Context(), s, o, p)
	if err != nil {
		return Block{}, err
	}
	items := make([]T, len(models))
	for i, m := range models {
		items[i] = f(m)
	}
	return Block{Name: "list", Title: rs.Title, Data: rs.List(items, total, p, s, o)}, nil
}

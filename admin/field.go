package admin

import (
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/dbond762/gojiffy/view"
)

// FieldErrors — the words for a check that failed. An empty one takes the
// library's own message in the language of the page.
type FieldErrors struct {
	Required, Min, Max string
	Regex              string // a value that does not match Regex
	Choice             string // a value that is not among Choices
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

	// Choices — the fixed list of values the field takes, for enumerations and
	// boolean flags. It makes the form field a <select>, and the filter too with
	// FilterSelect, and Parse accepts nothing outside it. item is nil for the filter, which offers
	// every value; for the form it is the record, so the list may depend on it —
	// which statuses may follow the current one, say. Parse asks with the record
	// as it came in, before anything from the request is stored.
	Choices func(item *T) []view.Option

	// LookupChoices — Choices that live in the store rather than in code: the
	// form and Parse ask the gojiffy.Chooser they are given, by Name, and the
	// field works as with Choices. A list filter asks the store of the list, see
	// Filter.
	LookupChoices bool
	// LookupLimit makes a LookupChoices field too long for a select a text box
	// that suggests as you type: what matches is asked of the store at most
	// LookupLimit at a time, through WriteChoices. 0 keeps the whole list in a
	// select.
	LookupLimit int

	// list
	Class  string // css class of the column, for its width say
	Search bool
	Filter Filter // what the Search field draws under the heading; a text box by default
	// FilterEmpty — the label of one more choice in a select or lookup filter:
	// the records where the field is empty, see gojiffy.SearchEmpty. "" offers
	// no such choice.
	FilterEmpty string
	Sort        bool           // the heading becomes a sorting link
	Text        func(T) string // the cell value, already formatted
	Href        func(T) string // when set, the cell becomes a link
	Bool        func(T) bool   // when set, the cell draws a tick or a cross instead of Text

	// form
	Label    string // when the form needs other words than the list; empty takes Caption
	Type     string
	Help     string // hint under the field
	HelpEdit string // hint when editing, if it differs
	Value    func(T) string
	Readonly func(T) bool // true only shows the field: the request is not read for it at all
	// Autofill — the autocomplete attribute of the input: what a browser or a
	// password manager may fill in there ("username", "new-password", "off").
	Autofill string

	// Checks made before Parse, on the value with its spaces trimmed. Length
	// is in characters, not bytes, and is checked only on a value that is
	// there: whether it may be empty at all is Required's question.
	NoTrim   bool // keep the spaces: a password is what it is, spaces and all
	Required bool
	Min, Max int // 0 means no limit
	// Regex — what a value that is there must look like. It is matched as it
	// stands, so anchor it with ^ and $ to mean the whole value.
	Regex  *regexp.Regexp
	Errors FieldErrors

	// The value goes into the string field of the record that Name refers to
	// (status into Status, first_name into FirstName) before Parse is called,
	// so a field that only stores what came in needs no Parse at all. Parse is
	// for the rest: a date, a number, a rule of its own — and it may overwrite
	// what was stored. NoSet leaves the record alone for Parse to decide, for a
	// rule that has to look at what the record held before.
	NoSet bool
	Parse func(*T, string, bool) error // the bool means creating

	Permission string // the field is visible only with this permission, in list and form alike
}

// Filter — what a Search field draws under its heading in the list. It is the
// resource's choice, not something guessed from the field: a status may be
// better picked from a select, a name typed in part.
type Filter int

const (
	// FilterText — a text box; how its text is matched is the store's business,
	// by a part of the value usually.
	FilterText Filter = iota
	// FilterSelect — a select, All first, of the field's Choices or of its
	// LookupChoices asked of the store of the list whole.
	FilterSelect
	// FilterLookup — a text box that suggests the field's LookupChoices as you
	// type, at most LookupLimit at a time, the way the form field does; the list
	// is filtered by the value picked, not by the text.
	FilterLookup
)

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

// check makes the declared checks and gives back what went wrong, or "".
func (f Field[T]) check(v string) string {
	n := utf8.RuneCountInString(v)
	switch {
	case v == "" && f.Required:
		return or(f.Errors.Required, view.T("Fill in this field"))
	case v != "" && f.Min > 0 && n < f.Min:
		return or(f.Errors.Min, view.T(minKey, f.Min))
	case f.Max > 0 && n > f.Max:
		return or(f.Errors.Max, view.T(maxKey, f.Max))
	case v != "" && f.Regex != nil && !f.Regex.MatchString(v):
		return or(f.Errors.Regex, view.T("Invalid format"))
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
// first_name finds FirstName, embedded structs included. Anything that is
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

// minKey, maxKey and showingKey — keys of the library catalogue that live in
// view, which registers their plural forms; the text is the key, so the same
// words here find them. TestKeysAreTranslated keeps the two in step.
const (
	minKey     = "At least %d characters"
	maxKey     = "At most %d characters"
	showingKey = "Showing %[1]d–%[2]d of %[3]d records"
)

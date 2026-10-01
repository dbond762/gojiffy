package view

// Data for partials/form.html and partials/field.html — prepared by Form.View.

// Option — one choice of a <select>.
type Option struct {
	Value, Label string
	Selected     bool
}

// A FieldView with Options set renders as a <select>, otherwise as an <input Type>.
type FieldView struct {
	Name, Label, Type, Value, Help, Error string
	Autofill                              string // the autocomplete attribute: "new-password" stops a password manager filling it in
	Options                               []Option
	// Lookup — where a field too long for a select asks for suggestions, see
	// admin.Resource.WriteChoices; ValueText is how its Value reads.
	Lookup, ValueText string
	Required          bool
	// Readonly rather than disabled: the value is still shown and does not
	// break required; the server ignores it, see admin.Resource.Parse.
	Readonly bool
}

// Button — an action beside the form but carrying its own request: nested
// forms are not allowed in HTML, so it is drawn as a separate form after the
// main one.
//
// Link draws it as a plain link instead: for a page of its own, a form say,
// rather than an action carried out at once. Confirm is ignored then.
type Button struct {
	Title, Action, Class string
	Confirm              string // the confirmation text; empty asks nothing
	Link                 bool
}

// FormView — a whole form: where it is sent, its fields and the buttons beside
// it. Error is shown above the fields, for what belongs to no one field.
type FormView struct {
	Action, Submit, CancelURL string
	// CSRF is filled in when the form goes onto a page. Sign-in fills it
	// itself: that page has no Page around it.
	CSRF, Error string
	Title       string
	Fields      []FieldView
	Buttons     []Button
}

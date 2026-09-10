package view

// Data for partials/form.html and partials/field.html — prepared by Form.View.

type Option struct {
	Value, Label string
	Selected     bool
}

// A FieldView with Options set renders as a <select>, otherwise as an <input Type>.
type FieldView struct {
	Name, Label, Type, Value, Help, Error string
	Autocomplete                          string // "new-password" stops a password manager filling it in
	Options                               []Option
	Required                              bool
	// Readonly rather than disabled: the value still reaches the server and
	// does not break required, and whether to accept it is up to Parse.
	Readonly bool
}

// Button — an action beside the form but carrying its own request: nested
// forms are not allowed in HTML, so it is drawn as a separate form after the
// main one.
type Button struct {
	Title, Action, Class string
	Confirm              string // the confirmation text; empty asks nothing
}

type FormView struct {
	Action, Submit, CancelURL string
	// CSRF is filled in when the form goes onto a page. Sign-in fills it
	// itself: that page has no Page around it.
	CSRF, Error string
	Title       string
	Fields      []FieldView
	Buttons     []Button
}

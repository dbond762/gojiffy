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

// Notice — a highlighted block above the form. Code is shown in a monospace
// font: it is the part meant to be copied.
type Notice struct{ Title, Text, Code string }

type FormView struct {
	Action, Submit, CancelURL string
	CSRF, Error               string
	Title                     string
	Notice                    *Notice
	Fields                    []FieldView
	Buttons                   []Button
}

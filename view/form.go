package view

// Данные для partials/form.html и partials/field.html — их готовит Form.View.

type Option struct {
	Value, Label string
	Selected     bool
}

// FieldView с непустым Options рендерится как <select>, иначе как <input Type>.
type FieldView struct {
	Name, Label, Type, Value, Help, Error string
	Autocomplete                          string // "new-password" гасит подстановку менеджера паролей
	Options                               []Option
	Required                              bool
	// Readonly, а не disabled: значение всё равно уходит на сервер и не ломает
	// required, а принять его или нет — решает Parse.
	Readonly bool
}

// Button — действие рядом с формой, но своим запросом: вложенные формы в HTML
// запрещены, поэтому рисуется отдельной формой после основной.
type Button struct {
	Title, Action, Class string
	Confirm              string // текст подтверждения; пусто — без вопроса
}

// Notice — выделенный блок над формой. Code показывается моноширинным: это то,
// что нужно скопировать.
type Notice struct{ Title, Text, Code string }

type FormView struct {
	Action, Submit, CancelURL string
	CSRF, Error               string
	Title                     string
	Notice                    *Notice
	Fields                    []FieldView
	Buttons                   []Button
}

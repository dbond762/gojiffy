package gojiffy

// Search — filters from the request: field name to search string. What to do
// with them is up to the store: there is no SQL here, and the library knows
// nothing of any database dialect.
type Search map[string]string

// Order — which field to sort by. An empty field means the default order.
type Order struct {
	Field string
	Desc  bool
}

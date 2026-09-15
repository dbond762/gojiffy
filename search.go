package gojiffy

// Search — filters from the request: field name to search string. What to do
// with them is up to the store: there is no SQL here, and the library knows
// nothing of any database dialect.
type Search map[string]string

// SearchEmpty — the search value asking for records where the field has no
// value at all: a client with no manager, say. A choice filter offers it under
// the field's FilterEmpty (see view.Field); the store turns it into its own
// "is empty". No real value of a choice looks like it.
const SearchEmpty = "_empty"

// Order — which field to sort by. An empty field means the default order.
type Order struct {
	Field string
	Desc  bool
}

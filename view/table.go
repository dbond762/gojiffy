package view

// Data for partials/table.html — the template knows nothing of any entity.

// A Column with Search set draws a search[Search] filter field under its
// heading: a text input, a select with a fixed list when SearchOptions is set as
// well (its first option is the empty value, clearing the filter), or a text box
// suggesting as you type when Lookup is.
type Column struct {
	Title, Class  string
	Search        string // name of the search parameter, or "" for no filter
	Query         string // what has been entered already
	SearchOptions []Option
	// Lookup — where the filter asks for suggestions, see admin.Resource.WriteChoices;
	// QueryText is how its Query reads.
	Lookup, QueryText string
	Empty             Option // a lookup's choice of records with nothing in the field, see Field.FilterEmpty; zero offers none
	Actions           bool   // the search and reset buttons stand here in the filter row
	SortHref          string // link to sort by this column; "" means sorting is not allowed
	SortDir           string // current direction: "asc", "desc", or "" when not sorted by it
}

// RowAction — an action on a row. Post draws a button that submits the shared
// form from the layout: the table itself is already wrapped in the search form
// and nested forms are not allowed in HTML, so the button is tied to the outer
// one through form/formaction.
type RowAction struct {
	Title, Href string
	Post        bool
	Confirm     string // the confirmation text; "" asks nothing
}

// A Cell with Href renders as a link, one with Actions as the actions column:
// a single action as a button, several as a dropdown. Class is an optional css
// class for the cell, needed at the moment only for the tick and cross (see
// Field.Bool).
type Cell struct {
	Text, Href, Class string
	Actions           []RowAction
}

// Table — a list as drawn: its columns with their filters and sorting, and
// the cells row by row.
type Table struct {
	Columns []Column
	Rows    [][]Cell
	Empty   string // the text to show when there are no rows
	Action  string // where the search form goes; "" draws no form
}

// ListView — what goes into the list.html template.
type ListView struct {
	Title  string
	Header []Link
	Table  Table
	Footer ListFooter
}

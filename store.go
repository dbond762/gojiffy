package gojiffy

import "context"

// Lister — one page of records, filtered and sorted the way the request asked,
// and the total to page by.
type Lister[M any] interface {
	List(ctx context.Context, s Search, o Order, p Paging) ([]M, int, error)
}

// Getter — one record by its id.
type Getter[M any] interface {
	Get(ctx context.Context, id int) (M, error)
}

// Creator — a new record; the id it was given comes back.
type Creator[M any] interface {
	Create(ctx context.Context, m M) (int, error)
}

// Updater — a record saved over the one with the same id.
type Updater[M any] interface {
	Update(ctx context.Context, m M) error
}

// Choice — one value a field may take and the words it is shown with.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ChoiceQuery — which of a field's values the store is asked for. The zero
// query is all of them: a short list drawn whole into a select. A long one is
// asked by what was typed, a few at a time, and by the one value a form holds
// or sent.
type ChoiceQuery struct {
	Search string   // a part of the label, as typed; "" is any
	Values []string // only these values; nil is any
	Limit  int      // at most this many; 0 is no limit
}

// Chooser — a store that knows the values a field may take when they live in
// the data rather than in code: roles, authors, the articles a user may see.
// field is the Name of the field; a name the store does not know is an error,
// not an empty list — an empty list is a real answer a form would silently
// accept.
type Chooser interface {
	Choices(ctx context.Context, field string, q ChoiceQuery) ([]Choice, error)
}

// Deleter — the record gone from every list, whether the store erases it or
// only marks it deleted.
type Deleter interface {
	Delete(ctx context.Context, id int) error
}

// Store — everything a section of the admin panel does with its records.
// Each part is its own interface, so a function asks only for what it uses:
// ListBlock takes a Lister.
type Store[M any] interface {
	Lister[M]
	Getter[M]
	Creator[M]
	Updater[M]
	Deleter
}

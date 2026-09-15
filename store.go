package gojiffy

import "context"

// Lister — one page of records, filtered and sorted the way the request asked,
// and the total to page by.
type Lister[M any] interface {
	List(ctx context.Context, s Search, o Order, p Paging) ([]M, int, error)
}

type Getter[M any] interface {
	Get(ctx context.Context, id int) (M, error)
}

type Creator[M any] interface {
	Create(ctx context.Context, m M) (int, error)
}

type Updater[M any] interface {
	Update(ctx context.Context, m M) error
}

// Choice — one value a field may take and the words it is shown with.
type Choice struct{ Value, Label string }

// Chooser — a store that knows the values a field may take when they live in
// the data rather than in code: roles, managers, the clients a user may see.
// field is the Name of the field; a name the store does not know is an error,
// not an empty list — an empty list is a real answer a form would silently
// accept.
type Chooser interface {
	Choices(ctx context.Context, field string) ([]Choice, error)
}

type Store[M any] interface {
	Lister[M]
	Getter[M]
	Creator[M]
	Updater[M]
}

package gojiffy

import "context"

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

type Store[M any] interface {
	Lister[M]
	Getter[M]
	Creator[M]
	Updater[M]
}

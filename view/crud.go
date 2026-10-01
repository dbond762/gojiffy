package view

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strconv"

	"github.com/dbond762/gojiffy"
)

// CRUD — a whole section of the panel out of a Resource and a Store: the list,
// the forms to create and to edit, deleting, and the suggestions of a field
// that looks up as you type. What sets one section apart from another is said
// in the hooks; a section that needs none is a Resource, a Store and a Frame.
//
//	view.CRUD[Article]{Resource: Articles, Frame: app.Frame, Store: db.Articles()}.
//		Mount(mux, auth.Can, "articles.list")
//
// Editing parses the form over the record as it is stored, not over an empty
// one: a field the form does not send — hidden by a permission, read-only —
// keeps its value, and a Parse that looks at what the record held sees it.
//
// A section that does less — no deleting, no creating, only a list — says so
// with NoCreate, NoEdit and NoDelete, and its store needs only what is left.
// One that is more — buttons of its own, a one-time notice, a path that
// depends on a parent — keeps its own handlers and uses the Resource directly;
// the handlers here are methods too, for mounting one by one.
type CRUD[T any] struct {
	Resource Resource[T]
	Frame    Frame

	// Store — the records of the section, the same for everyone who may see it.
	// A list is all it has to give; what else it must do follows from what the
	// section does — Get for editing and deleting, Create, Update, Delete — and
	// Mount checks it. If it is also a gojiffy.Chooser, it answers the
	// LookupChoices fields.
	Store gojiffy.Lister[T]
	// StoreFor — the store for this request instead, when what one sees depends
	// on who asks: a store bounded by the user hides the rest from the list, the
	// forms and the address of a record alike. Set, it is used and Store is not.
	//
	//	StoreFor: func(r *http.Request) gojiffy.Lister[Article] {
	//		return db.Articles(auth.UserFrom(r.Context()).ID)
	//	},
	//
	// There is no store to check before a request, so one that falls short of
	// what the section does is a server error, with what it lacks in the log.
	StoreFor func(*http.Request) gojiffy.Lister[T]

	// NoCreate, NoEdit and NoDelete leave that part of the section out: Mount
	// does not put its addresses up, and the store need not do it. The zero
	// value is the whole section. The buttons that lead there — "New" in
	// Resource.Header, a delete Action — are the Resource's to leave out too.
	NoCreate, NoEdit, NoDelete bool

	// Perms — the permissions of who asks, for Resource.For; nil uses the
	// Resource as it is. With gojiffy/auth:
	//
	//	Perms: func(r *http.Request) gojiffy.Perms { return auth.UserFrom(r.Context()).Perms },
	Perms func(*http.Request) gojiffy.Perms

	// New — the record a create form starts from: defaults, a status. nil is
	// the zero record.
	New func(*http.Request) T

	// BeforeSave runs once the form has parsed cleanly, before the store is
	// written: it sets what does not come from the form (an owner from the
	// session, a parent from the path) and makes the checks that need more
	// than one field or a look into the store. The errors it returns are shown
	// under the fields, like those of Parse.
	BeforeSave func(r *http.Request, item *T, creating bool) (map[string]string, error)

	// StoreError turns an error of Create or Update into errors under fields —
	// a name already taken, say. nil from it, or no StoreError, is a server
	// error.
	StoreError func(error) map[string]string

	// BeforeDelete may refuse: an error made by Refuse is answered with a 409
	// and its text, any other is a server error.
	BeforeDelete func(*http.Request, T) error
	// AfterDelete — what follows a delete outside the store, a call to another
	// service, say. The record is gone by then, so there is nothing to answer
	// but the redirect; what must go with the delete or not at all belongs in
	// the store's Delete.
	AfterDelete func(*http.Request, T)

	// DeletePerm — the permission deleting asks for; "" is the one Mount was given.
	DeletePerm string
}

// Guard lets a request through to a handler only with a permission;
// auth.Auth.Can is one.
type Guard func(perm string, next http.HandlerFunc) http.HandlerFunc

// Mount puts the section at Resource.Path, every address under perm:
//
//	GET  /articles             the list
//	GET  /articles/choices     suggestions of a field, see WriteChoices
//	GET  /articles/new         create form
//	POST /articles             create
//	GET  /articles/{id}        edit form
//	POST /articles/{id}        save
//	POST /articles/{id}/delete delete, under DeletePerm when set
//
// less what NoCreate, NoEdit and NoDelete leave out. A delete Action of the
// Resource leads to Href(item) + "/delete".
//
// What cannot work panics here, at startup, rather than on the first request:
// neither Store nor StoreFor, or a Store that does not do what the section
// does — no Delete without NoDelete, say.
//
// The path wildcard {id} is the CRUD's own: Edit, Save and Delete read the
// record's id out of it, mounted by Mount or one by one, so a pattern of one's
// own for them has to call it {id} too. A Path with a wildcard of its own
// names it otherwise — /authors/{author_id}/articles, not /authors/{id}/articles:
// the same name twice in a pattern is a panic of the mux, and to the handlers
// it would be the parent's id.
func (c CRUD[T]) Mount(mux *http.ServeMux, can Guard, perm string) {
	p := c.Resource.Path
	switch {
	case c.Store == nil && c.StoreFor == nil:
		panic("view: CRUD for " + p + " has neither Store nor StoreFor")
	case c.StoreFor == nil:
		if lack := c.lacks(c.Store); lack != "" {
			panic(fmt.Sprintf("view: CRUD for %s: store %T has no %s", p, c.Store, lack))
		}
	}
	del := c.DeletePerm
	if del == "" {
		del = perm
	}
	mux.HandleFunc("GET "+p, can(perm, Handler(c.Frame, c.List)))
	mux.HandleFunc("GET "+p+"/choices", can(perm, c.Choices))
	if !c.NoCreate {
		mux.HandleFunc("GET "+p+"/new", can(perm, c.NewForm))
		mux.HandleFunc("POST "+p, can(perm, c.Create))
	}
	if !c.NoEdit {
		mux.HandleFunc("GET "+p+"/{id}", can(perm, c.Edit))
		mux.HandleFunc("POST "+p+"/{id}", can(perm, c.Save))
	}
	if !c.NoDelete {
		mux.HandleFunc("POST "+p+"/{id}/delete", can(del, c.Delete))
	}
}

// lacks — what the store does not do of what the section does, said the way
// to put it right; "" when it does all of it.
func (c CRUD[T]) lacks(store any) string {
	_, get := store.(gojiffy.Getter[T])
	_, create := store.(gojiffy.Creator[T])
	_, update := store.(gojiffy.Updater[T])
	_, del := store.(gojiffy.Deleter)
	switch {
	case !get && !(c.NoEdit && c.NoDelete):
		return "Get; implement it or set NoEdit and NoDelete"
	case !create && !c.NoCreate:
		return "Create; implement it or set NoCreate"
	case !update && !c.NoEdit:
		return "Update; implement it or set NoEdit"
	case !del && !c.NoDelete:
		return "Delete; implement it or set NoDelete"
	}
	return ""
}

// as — the store as the part a handler needs. One that is not, from a
// StoreFor or a handler mounted by hand, is a server error saying what it lacks.
func as[I any](w http.ResponseWriter, store any, method string) (I, bool) {
	i, ok := store.(I)
	if !ok {
		serverError(w, fmt.Errorf("view: CRUD store %T has no %s", store, method))
	}
	return i, ok
}

// List — the list as a block, to be drawn by Handler with other blocks around it.
func (c CRUD[T]) List(_ http.ResponseWriter, r *http.Request) (Block, error) {
	return c.resource(r).ListBlock(r, c.store(r))
}

// Choices answers a field that suggests as you type, see WriteChoices.
func (c CRUD[T]) Choices(w http.ResponseWriter, r *http.Request) {
	ch := asChooser(c.store(r))
	if ch == nil {
		http.NotFound(w, r)
		return
	}
	c.resource(r).WriteChoices(w, r, ch)
}

func (c CRUD[T]) NewForm(w http.ResponseWriter, r *http.Request) {
	c.resource(r).RenderForm(w, r, c.Frame, asChooser(c.store(r)), c.newItem(r), true, nil)
}

func (c CRUD[T]) Edit(w http.ResponseWriter, r *http.Request) {
	store := c.store(r)
	item, ok := c.get(w, r, store)
	if !ok {
		return
	}
	c.resource(r).RenderForm(w, r, c.Frame, asChooser(store), item, false, nil)
}

func (c CRUD[T]) Create(w http.ResponseWriter, r *http.Request) {
	store := c.store(r)
	cr, ok := as[gojiffy.Creator[T]](w, store, "Create")
	if !ok {
		return
	}
	c.save(w, r, store, c.newItem(r), true, func(item T) error {
		_, err := cr.Create(r.Context(), item)
		return err
	})
}

func (c CRUD[T]) Save(w http.ResponseWriter, r *http.Request) {
	store := c.store(r)
	up, ok := as[gojiffy.Updater[T]](w, store, "Update")
	if !ok {
		return
	}
	item, ok := c.get(w, r, store)
	if !ok {
		return
	}
	c.save(w, r, store, item, false, func(item T) error { return up.Update(r.Context(), item) })
}

// save parses the form over item, runs BeforeSave and writes the record with
// write: a form with errors comes back with a 422, a written record leads to
// the list.
func (c CRUD[T]) save(w http.ResponseWriter, r *http.Request, store gojiffy.Lister[T], item T, creating bool, write func(T) error) {
	rs, ch := c.resource(r), asChooser(store)
	item, errs, err := rs.Parse(r, ch, item, creating)
	if err != nil {
		serverError(w, err)
		return
	}
	if len(errs) == 0 && c.BeforeSave != nil {
		more, err := c.BeforeSave(r, &item, creating)
		if err != nil {
			serverError(w, err)
			return
		}
		maps.Copy(errs, more)
	}
	if len(errs) > 0 {
		rs.RenderForm(w, r, c.Frame, ch, item, creating, errs)
		return
	}

	if err := write(item); err != nil {
		if c.StoreError != nil {
			if errs := c.StoreError(err); len(errs) > 0 {
				rs.RenderForm(w, r, c.Frame, ch, item, creating, errs)
				return
			}
		}
		if errors.Is(err, gojiffy.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		serverError(w, err)
		return
	}
	http.Redirect(w, r, rs.Path, http.StatusSeeOther)
}

func (c CRUD[T]) Delete(w http.ResponseWriter, r *http.Request) {
	store := c.store(r)
	del, ok := as[gojiffy.Deleter](w, store, "Delete")
	if !ok {
		return
	}
	item, ok := c.get(w, r, store)
	if !ok {
		return
	}
	if c.BeforeDelete != nil {
		if err := c.BeforeDelete(r, item); err != nil {
			var refusal Refusal
			if errors.As(err, &refusal) {
				http.Error(w, string(refusal), http.StatusConflict)
				return
			}
			serverError(w, err)
			return
		}
	}
	id, _ := strconv.Atoi(r.PathValue("id")) // get has read it already
	switch err := del.Delete(r.Context(), id); {
	case errors.Is(err, gojiffy.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		serverError(w, err)
		return
	}
	if c.AfterDelete != nil {
		c.AfterDelete(r, item)
	}
	http.Redirect(w, r, c.Resource.Path, http.StatusSeeOther)
}

// get — the record the path names, as the store of this request sees it; a 404
// for an id that is not a number or not there.
func (c CRUD[T]) get(w http.ResponseWriter, r *http.Request, store gojiffy.Lister[T]) (T, bool) {
	var zero T
	getter, ok := as[gojiffy.Getter[T]](w, store, "Get")
	if !ok {
		return zero, false
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return zero, false
	}
	item, err := getter.Get(r.Context(), id)
	switch {
	case errors.Is(err, gojiffy.ErrNotFound):
		http.NotFound(w, r)
		return zero, false
	case err != nil:
		serverError(w, err)
		return zero, false
	}
	return item, true
}

func (c CRUD[T]) store(r *http.Request) gojiffy.Lister[T] {
	if c.StoreFor != nil {
		return c.StoreFor(r)
	}
	return c.Store
}

func (c CRUD[T]) resource(r *http.Request) Resource[T] {
	if c.Perms == nil {
		return c.Resource
	}
	return c.Resource.For(c.Perms(r))
}

func (c CRUD[T]) newItem(r *http.Request) T {
	if c.New == nil {
		var zero T
		return zero
	}
	return c.New(r)
}

// asChooser — the store as a Chooser, or nil when it is not one.
func asChooser(store any) gojiffy.Chooser {
	ch, _ := store.(gojiffy.Chooser)
	return ch
}

// Refusal — a delete that BeforeDelete will not allow, with the words why.
type Refusal string

func (r Refusal) Error() string { return string(r) }

// Refuse makes the error BeforeDelete returns to say no: the user gets a 409
// and msg.
func Refuse(msg string) error { return Refusal(msg) }

package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/dbond762/gojiffy"
	"github.com/dbond762/gojiffy/view"
)

// choices — what the field may take: its fixed Choices or, with LookupChoices,
// what the store answers to q. choice is false for a field that is not a choice
// at all.
func (f Field[T]) choices(ctx context.Context, store gojiffy.Chooser, item *T, q gojiffy.ChoiceQuery) (opts []view.Option, choice bool, err error) {
	switch {
	case f.Choices != nil:
		return f.Choices(item), true, nil
	case !f.LookupChoices:
		return nil, false, nil
	case store == nil:
		return nil, true, fmt.Errorf("admin: field %q has LookupChoices, but there is no store to ask", f.Name)
	}
	list, err := store.Choices(ctx, f.Name, q)
	if err != nil {
		return nil, true, err
	}
	return options(list), true, nil
}

func options(list []gojiffy.Choice) []view.Option {
	opts := make([]view.Option, len(list))
	for i, c := range list {
		opts[i] = view.Option{Value: c.Value, Label: c.Label}
	}
	return opts
}

// filterOptions — the options of a select filter: All, the records with nothing
// in the field when FilterEmpty offers them, then the choices themselves.
func (f Field[T]) filterOptions(opts []view.Option) []view.Option {
	if f.FilterEmpty != "" {
		opts = append([]view.Option{{Value: gojiffy.SearchEmpty, Label: f.FilterEmpty}}, opts...)
	}
	return withAll(opts)
}

// lookupURL — where a field that suggests as you type asks for suggestions, the
// form field and the list filter alike, see WriteChoices.
func (rs Resource[T]) lookupURL(f Field[T]) string {
	return rs.Path + "/choices?field=" + url.QueryEscape(f.Name)
}

// WriteChoices answers a field that suggests as you type — a form field with
// LookupLimit or a FilterLookup filter: the matches for ?q= among the choices
// of ?field=, at most LookupLimit of them, as JSON. A field the resource does
// not have, hides by For or does not look up this way is a 404 — the address
// gives out nothing its page would not.
//
//	mux.HandleFunc("GET /articles/choices", func(w http.ResponseWriter, r *http.Request) {
//		rs.For(perms).WriteChoices(w, r, store)
//	})
func (rs Resource[T]) WriteChoices(w http.ResponseWriter, r *http.Request, store gojiffy.Chooser) {
	name := r.URL.Query().Get("field")
	i := slices.IndexFunc(rs.Fields, func(f Field[T]) bool {
		return f.Name == name && f.LookupChoices &&
			(f.inForm() && f.LookupLimit > 0 || f.inList() && f.Search && f.Filter == FilterLookup)
	})
	if i < 0 {
		http.NotFound(w, r)
		return
	}
	f := rs.Fields[i]
	if store == nil {
		serverError(w, fmt.Errorf("admin: field %q has LookupChoices, but there is no store to ask", f.Name))
		return
	}
	list, err := store.Choices(r.Context(), f.Name, gojiffy.ChoiceQuery{
		Search: strings.TrimSpace(r.URL.Query().Get("q")),
		Limit:  f.LookupLimit,
	})
	if err != nil {
		serverError(w, err)
		return
	}
	if f.LookupLimit > 0 && len(list) > f.LookupLimit {
		list = list[:f.LookupLimit]
	}
	if list == nil {
		list = []gojiffy.Choice{} // [] rather than null: the script walks it as it is
	}
	w.Header().Set("Content-Type", "application/json")
	// names of the records are not kept by the browser, nor read as anything but JSON
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := json.NewEncoder(w).Encode(list); err != nil {
		log.Printf("admin: choices of %q: %v", f.Name, err)
	}
}

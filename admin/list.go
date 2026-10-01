package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/dbond762/gojiffy"
	"github.com/dbond762/gojiffy/view"
)

// Paging — which page the request asks for.
func (rs Resource[T]) Paging(r *http.Request) gojiffy.Paging {
	return ParsePaging(r.URL.Query(), rs.PerPage)
}

// ParseOrder takes the sorting out of the request if that field was declared
// sortable. Otherwise DefaultOrder: both when ?sort= is empty and when it names
// something else.
func (rs Resource[T]) ParseOrder(r *http.Request) gojiffy.Order {
	o := ParseOrder(r.URL.Query())
	for _, f := range rs.Fields {
		if f.Sort && f.inList() && f.Name == o.Field {
			return o
		}
	}
	return rs.DefaultOrder
}

// ParseSearch takes only fields marked Search out of the request: the
// declaration is the validation.
func (rs Resource[T]) ParseSearch(r *http.Request) gojiffy.Search {
	q := r.URL.Query()
	s := gojiffy.Search{}
	for _, f := range rs.Fields {
		if !f.Search || !f.inList() {
			continue
		}
		if v := strings.TrimSpace(q.Get("search[" + f.Name + "]")); v != "" {
			s[f.Name] = v
		}
	}
	return s
}

// List builds a page of the list: headings, filter fields, rows in the order
// the fields were declared, and a footer with the counter and page links.
func (rs Resource[T]) List(items []T, total int, p gojiffy.Paging, s gojiffy.Search, o gojiffy.Order) view.ListView {
	empty := rs.Empty
	if empty == "" {
		empty = view.T("Nothing found")
	}

	tbl := view.Table{Empty: empty}

	filtered := false
	for _, f := range rs.Fields {
		if !f.inList() {
			continue
		}
		col := view.Column{Title: f.Caption, Class: f.Class}
		if f.Search {
			col.Search, col.Query = f.Name, s[f.Name]
			filtered = true
			switch {
			case f.Filter == FilterSelect && f.Choices != nil:
				col.SearchOptions = selected(f.filterOptions(f.Choices(nil)), s[f.Name])
			case f.Filter == FilterLookup:
				// the label of the value comes from the store, see storeFilters;
				// until then the value itself is at least something to read
				col.Lookup, col.QueryText = rs.lookupURL(f), s[f.Name]
				if f.FilterEmpty != "" {
					col.Empty = view.Option{Value: gojiffy.SearchEmpty, Label: f.FilterEmpty}
					if s[f.Name] == gojiffy.SearchEmpty {
						col.QueryText = f.FilterEmpty // the store has no label for what is not there
					}
				}
			}
		}
		if f.Sort {
			next := gojiffy.Order{Field: f.Name}
			if o.Field == f.Name {
				col.SortDir = "asc"
				if o.Desc {
					col.SortDir = "desc"
				}
				next.Desc = !o.Desc // clicking again flips the order
			}
			// sorting resets the page: there is nothing to do on page seven of a new order
			col.SortHref = listURL(rs.Path, s, next, 1)
		}
		tbl.Columns = append(tbl.Columns, col)
	}

	// actions of their own go into their own columns, the rest follow in one bunch
	var own, menu []Action[T]
	for _, a := range rs.Actions {
		if a.Column {
			own = append(own, a)
		} else {
			menu = append(menu, a)
		}
	}
	for range own {
		tbl.Columns = append(tbl.Columns, view.Column{Class: "col-actions"})
	}
	if len(menu) > 0 {
		tbl.Columns = append(tbl.Columns, view.Column{Class: "col-actions"})
	}
	// with no filter at all the search row is pointless, and so is the form around the table
	if filtered {
		tbl.Action = rs.Path
		tbl.Columns[len(tbl.Columns)-1].Actions = true
	}

	for _, row := range items {
		cells := make([]view.Cell, 0, len(tbl.Columns))
		for _, f := range rs.Fields {
			if !f.inList() {
				continue
			}
			var cell view.Cell
			switch {
			case f.Bool != nil:
				cell = view.Cell{Text: "✓", Class: "bool-yes"}
				if !f.Bool(row) {
					cell = view.Cell{Text: "✗", Class: "bool-no"}
				}
			default:
				cell = view.Cell{Text: f.Text(row)}
			}
			if f.Href != nil {
				cell.Href = f.Href(row)
			}
			cells = append(cells, cell)
		}
		for _, a := range own {
			cells = append(cells, view.Cell{Actions: rowActions(row, a)})
		}
		if len(menu) > 0 {
			cells = append(cells, view.Cell{Actions: rowActions(row, menu...)})
		}
		tbl.Rows = append(tbl.Rows, cells)
	}

	header := make([]view.Link, 0, len(rs.Header))
	for _, h := range rs.Header {
		header = append(header, view.Link{Title: h.Title, Href: rs.Path + h.Href})
	}
	return view.ListView{
		Title:  rs.Title,
		Header: header,
		Table:  tbl,
		Footer: listFooter(rs.Path, s, o, p, total),
	}
}

func selected(opts []view.Option, value string) []view.Option {
	out := make([]view.Option, len(opts))
	for i, o := range opts {
		o.Selected = o.Value == value
		out[i] = o
	}
	return out
}

// withAll puts All first (an empty value, meaning the filter is off).
func withAll(opts []view.Option) []view.Option {
	return append([]view.Option{{Label: view.T("All")}}, opts...)
}

// ListBlock is the whole of a list page: it takes the filters, the sorting and
// the page out of the request, asks the store for that page of records and
// builds the block the theme draws.
func (rs Resource[T]) ListBlock(r *http.Request, store gojiffy.Lister[T]) (view.Block, error) {
	return MapList(rs, r, store, func(item T) T { return item })
}

// MapList is ListBlock where the page record is wider than the model — a
// password, a computed field: the store hands out models, f makes records of
// them.
//
//	admin.MapList(rs, r, db.Articles(), func(a Article) ArticleRow {
//		return ArticleRow{Article: a, Words: len(strings.Fields(a.Text))}
//	})
func MapList[M, T any](rs Resource[T], r *http.Request, store gojiffy.Lister[M], f func(M) T) (view.Block, error) {
	s, o, p := rs.ParseSearch(r), rs.ParseOrder(r), rs.Paging(r)

	models, total, err := store.List(r.Context(), s, o, p)
	if err != nil {
		return view.Block{}, err
	}
	items := make([]T, len(models))
	for i, m := range models {
		items[i] = f(m)
	}
	lv := rs.List(items, total, p, s, o)
	if err := storeFilters(r.Context(), rs, store, &lv.Table); err != nil {
		return view.Block{}, err
	}
	return view.Block{
		Name:   "list",
		Title:  rs.Title,
		Crumbs: slices.Concat(rs.Crumbs, []view.Link{{Title: rs.Title}}),
		Data:   lv,
	}, nil
}

// storeFilters fills in the filters whose choices live in the store: a select
// of them whole, or the label of the value a lookup filters by. The store asked
// is the one of the list — it knows the records, and what they refer to.
func storeFilters[T any](ctx context.Context, rs Resource[T], store any, tbl *view.Table) error {
	i := -1
	for _, f := range rs.Fields {
		if !f.inList() {
			continue
		}
		i++ // the columns of the fields come first and in the same order, see List
		if !f.Search || !f.LookupChoices || f.Filter == FilterText {
			continue
		}
		chooser, ok := store.(gojiffy.Chooser)
		if !ok {
			return fmt.Errorf("admin: field %q filters by LookupChoices, but the store of the list is no gojiffy.Chooser", f.Name)
		}
		col := &tbl.Columns[i]
		q := gojiffy.ChoiceQuery{}
		if f.Filter == FilterLookup {
			if col.Query == "" || col.Query == gojiffy.SearchEmpty {
				continue // nothing to read, or read already, see List
			}
			q.Values = []string{col.Query}
		}
		list, err := chooser.Choices(ctx, f.Name, q)
		if err != nil {
			return err
		}
		opts := options(list)
		if f.Filter == FilterSelect {
			col.SearchOptions = selected(f.filterOptions(opts), col.Query)
			continue
		}
		// a value the store no longer offers keeps showing itself: the list is
		// filtered by it all the same
		if j := slices.IndexFunc(opts, func(o view.Option) bool { return o.Value == col.Query }); j >= 0 {
			col.QueryText = opts[j].Label
		}
	}
	return nil
}

func listFooter(path string, s gojiffy.Search, o gojiffy.Order, p gojiffy.Paging, total int) view.ListFooter {
	perPage, offset := p.LimitOffset()
	pages, current := p.Pages(total), p.Current()

	f := view.ListFooter{Text: view.T("No records")}
	if total > 0 {
		f.Text = view.T(showingKey, offset+1, min(offset+perPage, total), total)
	}
	if pages < 2 {
		return f
	}

	href := func(page int) string { return listURL(path, s, o, page) }
	if current > 1 {
		f.Prev = href(current - 1)
	}
	if current < pages {
		f.Next = href(current + 1)
	}

	// the first, a window around the current one, the last; gaps as an ellipsis
	shown := map[int]bool{1: true, pages: true, current: true}
	for i := current - pageWindow; i <= current+pageWindow; i++ {
		if i > 1 && i < pages {
			shown[i] = true
		}
	}
	prev := 0
	for i := 1; i <= pages; i++ {
		if !shown[i] {
			continue
		}
		if prev != 0 && i-prev > 1 {
			f.Pages = append(f.Pages, view.PageLink{Title: view.T("…")})
		}
		link := view.PageLink{Title: strconv.Itoa(i), Current: i == current}
		if i != current {
			link.Href = href(i)
		}
		f.Pages = append(f.Pages, link)
		prev = i
	}
	return f
}

// listURL builds the address of a list: filters and sorting survive paging.
func listURL(path string, s gojiffy.Search, o gojiffy.Order, page int) string {
	q := url.Values{}
	for name, value := range s {
		q.Set("search["+name+"]", value)
	}
	if o.Field != "" {
		sort := o.Field
		if o.Desc {
			sort = "-" + sort // a minus instead of a separate direction parameter
		}
		q.Set("sort", sort)
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

// ParseOrder reads ?sort=login or ?sort=-login. A field that was not declared
// is cut off by the resource.
func ParseOrder(query url.Values) gojiffy.Order {
	sort := query.Get("sort")
	if desc := strings.HasPrefix(sort, "-"); desc {
		return gojiffy.Order{Field: sort[1:], Desc: true}
	}
	return gojiffy.Order{Field: sort}
}

// ParsePaging reads ?page= from the request. Rubbish and negative values give
// the first page.
func ParsePaging(query url.Values, perPage int) gojiffy.Paging {
	page, _ := strconv.Atoi(query.Get("page"))
	return gojiffy.Paging{Page: page, PerPage: perPage}
}

// pageWindow — how many neighbouring pages to show on either side of the
// current one.
const pageWindow = 2

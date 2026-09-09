package view

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dbond762/gojiffy"
)

// PageLink — a link to a page of a list. An empty Href is the ellipsis or the
// current page.
type PageLink struct {
	Title   string
	Href    string
	Current bool
}

// ListFooter — the footer of a list table: always there, the counter always
// shown, page links appearing once there is more than one page.
type ListFooter struct {
	Text       string // the finished counter line: "Showing 1-20 of 42" or "No records"
	Prev, Next string // "" means the button is inactive
	Pages      []PageLink
}

// pageWindow — how many neighbouring pages to show on either side of the
// current one.
const pageWindow = 2

func listFooter(path string, s gojiffy.Search, o gojiffy.Order, p gojiffy.Paging, total int) ListFooter {
	perPage, offset := p.LimitOffset()
	pages, current := p.Pages(total), p.Current()

	f := ListFooter{Text: t("No records")}
	if total > 0 {
		f.Text = t(showingKey, offset+1, min(offset+perPage, total), total)
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
			f.Pages = append(f.Pages, PageLink{Title: t("…")})
		}
		link := PageLink{Title: strconv.Itoa(i), Current: i == current}
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

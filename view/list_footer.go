package view

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dbond762/gojiffy"
)

// PageLink — ссылка на страницу списка. Пустой Href — многоточие или текущая страница.
type PageLink struct {
	Title   string
	Href    string
	Current bool
}

// ListFooter — подвал таблицы списка: живёт под ней всегда, счётчик показывается
// в любом случае, ссылки на страницы появляются, когда страниц больше одной.
type ListFooter struct {
	Text       string // готовая строка счётчика — «Показано 1–20 из 42» или «Записей нет»
	Prev, Next string // "" — кнопка неактивна
	Pages      []PageLink
}

// pageWindow — сколько соседних страниц показывать слева и справа от текущей.
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

	// первая, окно вокруг текущей, последняя; разрывы — многоточием
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

// listURL собирает адрес списка: фильтры и сортировка переживают переход по страницам.
func listURL(path string, s gojiffy.Search, o gojiffy.Order, page int) string {
	q := url.Values{}
	for name, value := range s {
		q.Set("search["+name+"]", value)
	}
	if o.Field != "" {
		sort := o.Field
		if o.Desc {
			sort = "-" + sort // минус вместо отдельного параметра направления
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

// ParseOrder читает ?sort=login или ?sort=-login. Незаявленное поле отсекает ресурс.
func ParseOrder(query url.Values) gojiffy.Order {
	sort := query.Get("sort")
	if desc := strings.HasPrefix(sort, "-"); desc {
		return gojiffy.Order{Field: sort[1:], Desc: true}
	}
	return gojiffy.Order{Field: sort}
}

// ParsePaging читает ?page= из запроса. Мусор и отрицательные значения — первая страница.
func ParsePaging(query url.Values, perPage int) gojiffy.Paging {
	page, _ := strconv.Atoi(query.Get("page"))
	return gojiffy.Paging{Page: page, PerPage: perPage}
}

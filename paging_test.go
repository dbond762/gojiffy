package gojiffy

import "testing"

func TestPaging(t *testing.T) {
	limit, offset := Paging{Page: 3, PerPage: 20}.LimitOffset()
	if limit != 20 || offset != 40 {
		t.Errorf("limit=%d offset=%d, expected 20/40", limit, offset)
	}

	// an empty or broken page is the first one, at the default size
	for _, p := range []Paging{{}, {Page: -5}, {Page: 0, PerPage: -1}} {
		limit, offset := p.LimitOffset()
		if limit != PerPageDefault || offset != 0 || p.Current() != 1 {
			t.Errorf("%+v gave limit=%d offset=%d", p, limit, offset)
		}
	}

	p := Paging{PerPage: 20}
	for total, want := range map[int]int{0: 1, 1: 1, 20: 1, 21: 2, 100: 5, 101: 6} {
		if got := p.Pages(total); got != want {
			t.Errorf("%d records gave %d pages, expected %d", total, got, want)
		}
	}
}

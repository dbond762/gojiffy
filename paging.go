package gojiffy

import "math"

// PerPageDefault — rows per page when no size is given.
const PerPageDefault = 20

// Paging — which page of a list is wanted. Numbering starts at one.
type Paging struct {
	Page    int
	PerPage int
}

func (p Paging) normalize() Paging {
	if p.PerPage <= 0 {
		p.PerPage = PerPageDefault
	}
	if p.Page < 1 {
		p.Page = 1
	}
	// ?page= is whatever was typed: a page too far to count an offset for would
	// wrap it round to a negative one, which a database refuses
	if last := math.MaxInt / p.PerPage; p.Page > last {
		p.Page = last
	}
	return p
}

// LimitOffset — bounds of the selection for SQL.
func (p Paging) LimitOffset() (limit, offset int) {
	p = p.normalize()
	return p.PerPage, (p.Page - 1) * p.PerPage
}

// Pages — how many pages there are at this size.
func (p Paging) Pages(total int) int {
	perPage := p.normalize().PerPage
	if total <= 0 {
		return 1
	}
	return (total + perPage - 1) / perPage
}

// Current — the current page number after normalization.
func (p Paging) Current() int { return p.normalize().Page }

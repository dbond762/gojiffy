package gojiffy

// PerPageDefault — сколько строк на страницу, если размер не задан.
const PerPageDefault = 20

// Paging — какая страница списка нужна. Нумерация с единицы.
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
	return p
}

// LimitOffset — границы выборки для SQL.
func (p Paging) LimitOffset() (limit, offset int) {
	p = p.normalize()
	return p.PerPage, (p.Page - 1) * p.PerPage
}

// Pages — сколько всего страниц при таком размере.
func (p Paging) Pages(total int) int {
	perPage := p.normalize().PerPage
	if total <= 0 {
		return 1
	}
	return (total + perPage - 1) / perPage
}

// Current — номер текущей страницы после нормализации.
func (p Paging) Current() int { return p.normalize().Page }

package gojiffy

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Columns — имена полей списка и SQL-выражения за ними:
//
//	var UserColumns = Columns{"login": Col("users.login")}
//
// Она же валидация запроса: чего в карте нет, то ни в WHERE, ни в ORDER BY не попадёт.
type Columns map[string]colDef

type colDef struct {
	Expr  string
	Exact bool // true — сравнение через "=", иначе ILIKE по подстроке
}

// col — обычное текстовое поле, поиск по подстроке (ILIKE). Большинство полей такие.
// Col — обычная колонка: поиск по подстрке через ILIKE.
func Col(expr string) colDef { return colDef{Expr: expr} }

// enumCol — точное совпадение: для перечислений (status и подобные) и булевых
// признаков (Expr вида "x IS NOT NULL"::text). ILIKE по подстроке здесь дал бы
// ложные срабатывания — "expired" ловит и "expired_soon".
// EnumCol — колонка с фиксированным набором значений: сравнение точное,
// иначе ILIKE поймал бы «expired» в «expired_soon».
func EnumCol(expr string) colDef { return colDef{Expr: expr, Exact: true} }

// Search — фильтры из запроса: имя поля → строка поиска.
type Search map[string]string

// where собирает условия по известным полям. Ключи обходятся отсортированными,
// чтобы одинаковый фильтр всегда давал одинаковый SQL.
func (s Search) Where(cols Columns) (string, []any) {
	var conds []string
	var args []any
	for _, name := range slices.Sorted(maps.Keys(cols)) {
		q := strings.TrimSpace(s[name])
		if q == "" {
			continue
		}
		c := cols[name]
		args = append(args, q)
		n := strconv.Itoa(len(args))
		if c.Exact {
			conds = append(conds, c.Expr+" = $"+n)
		} else {
			conds = append(conds, c.Expr+" ILIKE '%' || $"+n+" || '%'")
		}
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// Order — по какому полю сортировать. Пустое поле означает порядок по умолчанию.
type Order struct {
	Field string
	Desc  bool
}

// orderBy добавляет tiebreak вторым ключом: без него строки с одинаковым значением
// перескакивают между страницами, потому что порядок в постгресе не гарантирован.
func (o Order) OrderBy(cols Columns, tiebreak string) string {
	c, ok := cols[o.Field]
	if !ok || c.Expr == tiebreak {
		if o.Desc && ok {
			return " ORDER BY " + tiebreak + " DESC"
		}
		return " ORDER BY " + tiebreak
	}
	dir := ""
	if o.Desc {
		dir = " DESC"
	}
	return " ORDER BY " + c.Expr + dir + ", " + tiebreak
}

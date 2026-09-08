package gojiffy

import "testing"

var allowed = Columns{
	"login":  Col("users.login"),
	"name":   Col("users.name"),
	"role":   Col("users_roles.role"),
	"status": EnumCol("clients.status"),
}

func TestWhereUsesOnlyAllowedFields(t *testing.T) {
	where, args := Search{"login": "  petr  ", "чужое": "x"}.Where(allowed)

	if want := " WHERE users.login ILIKE '%' || $1 || '%'"; where != want {
		t.Errorf("where = %q, expected %q", where, want)
	}
	if len(args) != 1 || args[0] != "petr" {
		t.Errorf("args = %#v, expected [petr]", args)
	}

	if where, args := (Search{"чужое": "x"}).Where(allowed); where != "" || args != nil {
		t.Errorf("незаявленное поле дало %q %#v", where, args)
	}
	if where, _ := (Search{"login": "   "}).Where(allowed); where != "" {
		t.Errorf("пробелы дали условие %q", where)
	}
}

func TestWhereIsStableAndNumbersArgs(t *testing.T) {
	s := Search{"role": "manager", "login": "petr"}

	want := " WHERE users.login ILIKE '%' || $1 || '%' AND users_roles.role ILIKE '%' || $2 || '%'"
	for range 5 { // карта обходится отсортированной, поэтому SQL не пляшет от запуска к запуску
		where, args := s.Where(allowed)
		if where != want {
			t.Fatalf("where = %q, expected %q", where, want)
		}
		if len(args) != 2 || args[0] != "petr" || args[1] != "manager" {
			t.Fatalf("args = %#v", args)
		}
	}
}

// Перечисления сравниваются точно ("="), иначе ILIKE '%active%' поймал бы и "inactive".
func TestWhereExactMatch(t *testing.T) {
	where, args := Search{"status": "active"}.Where(allowed)

	if want := " WHERE clients.status = $1"; where != want {
		t.Errorf("where = %q, expected %q", where, want)
	}
	if len(args) != 1 || args[0] != "active" {
		t.Errorf("args = %#v", args)
	}
}

func TestPaging(t *testing.T) {
	limit, offset := Paging{Page: 3, PerPage: 20}.LimitOffset()
	if limit != 20 || offset != 40 {
		t.Errorf("limit=%d offset=%d, expected 20/40", limit, offset)
	}

	// пустая и битая страница — первая, размер по умолчанию
	for _, p := range []Paging{{}, {Page: -5}, {Page: 0, PerPage: -1}} {
		limit, offset := p.LimitOffset()
		if limit != PerPageDefault || offset != 0 || p.Current() != 1 {
			t.Errorf("%+v дало limit=%d offset=%d", p, limit, offset)
		}
	}

	p := Paging{PerPage: 20}
	for total, want := range map[int]int{0: 1, 1: 1, 20: 1, 21: 2, 100: 5, 101: 6} {
		if got := p.Pages(total); got != want {
			t.Errorf("при %d записях страниц %d, expected %d", total, got, want)
		}
	}
}

func TestOrderBy(t *testing.T) {
	cols := Columns{"id": Col("users.id"), "login": Col("users.login")}

	cases := map[Order]string{
		{}:                           " ORDER BY users.id",
		{Field: "чужое"}:             " ORDER BY users.id",
		{Field: "login"}:             " ORDER BY users.login, users.id",
		{Field: "login", Desc: true}: " ORDER BY users.login DESC, users.id",
		{Field: "id"}:                " ORDER BY users.id",
		{Field: "id", Desc: true}:    " ORDER BY users.id DESC",
	}
	for o, want := range cases {
		if got := o.OrderBy(cols, "users.id"); got != want {
			t.Errorf("%+v дало %q, expected %q", o, got, want)
		}
	}
}

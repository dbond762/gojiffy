package view

import (
	"strings"
	"testing"

	"github.com/dbond762/gojiffy"
)

func TestEveryLanguageHasAllKeys(t *testing.T) {
	defer SetLanguage("en")
	for _, lang := range []string{"ru", "uk"} {
		SetLanguage(lang)
		for _, s := range libStrings {
			if got := lib.T(s.key); got == s.key {
				t.Errorf("%s: нет перевода для %q", lang, s.key)
			}
		}
	}
}

// Счётчик в футере — сообщение с множественным числом: подстановки должны
// доехать, а форма — смениться вместе с числом.
func TestShowingPlural(t *testing.T) {
	defer SetLanguage("en")
	SetLanguage("en")

	one, many := lib.T(showingKey, 1, 1, 1), lib.T(showingKey, 1, 5, 5)
	if one == showingKey || many == showingKey {
		t.Fatal("сообщение showing не найдено")
	}
	if one == many {
		t.Errorf("форма не зависит от числа: %q", one)
	}
	for _, s := range []string{one, many} {
		if !strings.Contains(s, "Showing") {
			t.Errorf("подстановка не подставилась: %q", s)
		}
	}
}

// Неизвестный ключ возвращается как есть — страница остаётся рабочей.
func TestUnknownKeyReturnsID(t *testing.T) {
	if got := lib.T("no_such_key_here"); got != "no_such_key_here" {
		t.Errorf("получили %q", got)
	}
}

// Подписи из описания ресурса либа не переводит — они приходят готовыми.
// Раньше здесь стоял T(), и ключ приложения приходилось искать в общем
// каталоге; теперь строка проходит насквозь.
func TestResourceLabelsPassThrough(t *testing.T) {
	rs := Resource[string, string]{
		Title:    "Довільний заголовок",
		One:      "Запис",
		NewTitle: "Новий запис",
		Empty:    "Порожньо",
		Href:     func(string) string { return "/x" },
		Wrap:     func(s string) string { return s },
		Fields: []Field[string]{{
			Name: "v", Title: "Значення", Label: "Значення", Help: "Підказка",
			Text: func(s string) string { return s },
		}},
		Actions: []Action[string]{{Title: "Змінити", Href: func(string) string { return "/x" }}},
	}

	lv := rs.List([]string{"a"}, 1, gojiffy.Paging{Page: 1, PerPage: 20}, nil, gojiffy.Order{})
	if lv.Title != rs.Title || lv.Table.Columns[0].Title != "Значення" {
		t.Errorf("заголовки списка изменились: %q, %q", lv.Title, lv.Table.Columns[0].Title)
	}
	if got := lv.Table.Rows[0][1].Actions[0].Title; got != "Змінити" {
		t.Errorf("подпись действия изменилась: %q", got)
	}

	fv := rs.Form("a", false, nil, nil, "")
	if fv.Title != rs.One || fv.Fields[0].Label != "Значення" || fv.Fields[0].Help != "Підказка" {
		t.Errorf("подписи формы изменились: %+v", fv)
	}
}

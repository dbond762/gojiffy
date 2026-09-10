package view

import (
	"testing"

	"github.com/dbond762/gojiffy"
)

func rec(fields ...Field[string]) Resource[string] {
	return Resource[string]{
		Path: "/x", One: "One", NewTitle: "New",
		Href:   func(string) string { return "/x/1" },
		Fields: fields,
	}
}

func listed(rs Resource[string]) []string {
	var out []string
	for _, c := range rs.List([]string{"a"}, 1, gojiffy.Paging{Page: 1, PerPage: 20}, nil, gojiffy.Order{}).Table.Columns {
		out = append(out, c.Title)
	}
	return out
}

func labelled(rs Resource[string]) []string {
	var out []string
	for _, f := range rs.Form("a", false, nil, nil).Fields {
		out = append(out, f.Label)
	}
	return out
}

// Most fields belong in both places and are called the same in both, so that
// is what a field with nothing said about it does: one word, both places.
func TestFieldGoesBothWaysByDefault(t *testing.T) {
	rs := rec(Field[string]{Name: "v", Caption: "Value", Text: func(s string) string { return s }})

	if got := listed(rs); len(got) != 1 || got[0] != "Value" {
		t.Errorf("list columns: %q", got)
	}
	if got := labelled(rs); len(got) != 1 || got[0] != "Value" {
		t.Errorf("form labels: %q, Caption did not stand in for Label", got)
	}
}

// The few that belong in one place say so, and say it in the declaration
// rather than by leaving a caption out and hoping it reads as intent.
func TestListOnlyAndEditOnly(t *testing.T) {
	rs := rec(
		Field[string]{Name: "id", Caption: "ID", ListOnly: true, Text: func(s string) string { return s }},
		Field[string]{Name: "pw", Caption: "Password", EditOnly: true},
	)

	if got := listed(rs); len(got) != 1 || got[0] != "ID" {
		t.Errorf("list columns: %q — EditOnly should not be there", got)
	}
	if got := labelled(rs); len(got) != 1 || got[0] != "Password" {
		t.Errorf("form labels: %q — ListOnly should not be there", got)
	}
}

// Label is for when the column and the field really are called differently.
func TestLabelOverridesCaption(t *testing.T) {
	rs := rec(Field[string]{
		Name: "v", Caption: "Short", Label: "The long way of saying it",
		Text: func(s string) string { return s },
	})

	if got := listed(rs); got[0] != "Short" {
		t.Errorf("the list took the form's wording: %q", got)
	}
	if got := labelled(rs); got[0] != "The long way of saying it" {
		t.Errorf("the form ignored Label: %q", got)
	}
}

// An empty list says something in the language of the page without every
// resource having to repeat it.
func TestEmptyListHasWordsOfItsOwn(t *testing.T) {
	rs := rec(Field[string]{Name: "v", Caption: "Value", Text: func(s string) string { return s }})

	if got := rs.List(nil, 0, gojiffy.Paging{Page: 1, PerPage: 20}, nil, gojiffy.Order{}).Table.Empty; got == "" {
		t.Error("an empty list says nothing at all")
	}

	rs.Empty = "no clients of that name"
	if got := rs.List(nil, 0, gojiffy.Paging{Page: 1, PerPage: 20}, nil, gojiffy.Order{}).Table.Empty; got != rs.Empty {
		t.Errorf("the default won over what the resource said: %q", got)
	}
}

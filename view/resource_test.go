package view

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dbond762/gojiffy"
)

func rec(fields ...Field[string]) Resource[string] {
	return Resource[string]{
		Path: "/x", NewTitle: "New", EditTitle: func(string) string { return "One" },
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

type account struct{ Login string }

// probe — a record for the parsing tests: an embedded struct, names that come
// with underscores, and a field that is not a string.
type probe struct {
	account
	SystemType string
	Status     string
	Count      int
}

func probeRes(fields ...Field[probe]) Resource[probe] {
	return Resource[probe]{Path: "/x", Href: func(probe) string { return "/x/1" }, Fields: fields}
}

func post(values url.Values) *http.Request {
	r := httptest.NewRequest("POST", "/x", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

// What only stores what came in needs no Parse: the value lands in the field
// the name refers to, underscores and case aside, embedded structs included —
// trimmed, unless the field says otherwise.
func TestParseStoresWithoutParse(t *testing.T) {
	rs := probeRes(
		Field[probe]{Name: "login"},
		Field[probe]{Name: "system_type"},
		Field[probe]{Name: "status", NoTrim: true},
		Field[probe]{Name: "count"},
	)
	values := url.Values{"login": {"  olya "}, "system_type": {"Windows"}, "status": {" on "}, "count": {"7"}}
	p, errs := rs.Parse(post(values), probe{Count: 3}, true)
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if p.Login != "olya" {
		t.Errorf("Login = %q: not trimmed, or not stored through the embedded struct", p.Login)
	}
	if p.SystemType != "Windows" {
		t.Errorf("SystemType = %q: system_type did not find it", p.SystemType)
	}
	if p.Status != " on " {
		t.Errorf("Status = %q: NoTrim trimmed it anyway", p.Status)
	}
	if p.Count != 3 {
		t.Errorf("Count = %d: a number is Parse's to convert", p.Count)
	}
}

// Parse comes after, so it sees the value already stored and may overwrite it.
func TestParseOverridesWhatWasStored(t *testing.T) {
	rs := probeRes(Field[probe]{Name: "status", Parse: func(p *probe, v string, _ bool) error {
		if p.Status != v {
			return errors.New("Parse ran before the value was stored")
		}
		p.Status = strings.ToUpper(v)
		return nil
	}})
	p, errs := rs.Parse(post(url.Values{"status": {"on"}}), probe{}, true)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if p.Status != "ON" {
		t.Errorf("Status = %q: Parse did not get the last word", p.Status)
	}
}

// A rule about what may follow what needs the old value: NoSet keeps it there
// for Parse.
func TestNoSetLeavesTheRecordForParse(t *testing.T) {
	var seen string
	rs := probeRes(Field[probe]{Name: "status", NoSet: true, Parse: func(p *probe, v string, _ bool) error {
		seen = p.Status
		return nil
	}})
	p, _ := rs.Parse(post(url.Values{"status": {"active"}}), probe{Status: "pending"}, false)
	if seen != "pending" {
		t.Errorf("Parse saw %q: the record was overwritten first", seen)
	}
	if p.Status != "pending" {
		t.Errorf("Status = %q: stored despite NoSet", p.Status)
	}
}

// A readonly input only shows the value: a forged request does not change it.
func TestReadonlyIsNotTakenFromTheRequest(t *testing.T) {
	rs := probeRes(Field[probe]{Name: "status", Readonly: func(p probe) bool { return p.Status == "locked" }})
	p, _ := rs.Parse(post(url.Values{"status": {"open"}}), probe{Status: "locked"}, false)
	if p.Status != "locked" {
		t.Errorf("Status = %q: a readonly field was taken from the request", p.Status)
	}
}

// The declared checks: empty against Required, length in characters rather
// than bytes, and only on a value that is there.
func TestFieldChecks(t *testing.T) {
	rs := probeRes(Field[probe]{Name: "login", Required: true, Min: 3, Max: 5})
	for _, c := range []struct {
		in  string
		bad bool
	}{
		{"", true},
		{"   ", true},  // trimmed away to nothing
		{"аб", true},   // two characters in four bytes: short all the same
		{"абв", false}, // three characters in six bytes: bytes would call it long
		{"абвгд", false},
		{"абвгде", true},
	} {
		_, errs := rs.Parse(post(url.Values{"login": {c.in}}), probe{}, true)
		if got := errs["login"] != ""; got != c.bad {
			t.Errorf("%q: error %q, wanted one: %v", c.in, errs["login"], c.bad)
		}
	}

	// Min is not Required: an optional field may stay empty
	opt := probeRes(Field[probe]{Name: "login", Min: 3})
	if _, errs := opt.Parse(post(url.Values{"login": {""}}), probe{}, true); errs["login"] != "" {
		t.Errorf("an empty optional field failed Min: %q", errs["login"])
	}
}

// Words of the application's own win; without them the library says it, with
// the number in it.
func TestFieldErrorsOverrideTheDefaults(t *testing.T) {
	plain := probeRes(Field[probe]{Name: "login", Required: true, Min: 2, Max: 3})
	own := probeRes(Field[probe]{Name: "login", Required: true, Min: 2, Max: 3,
		Errors: FieldErrors{Required: "who are you", Min: "too short a name", Max: "too long a name"}})

	for in, want := range map[string]string{"": "who are you", "a": "too short a name", "abcd": "too long a name"} {
		_, def := plain.Parse(post(url.Values{"login": {in}}), probe{}, true)
		_, got := own.Parse(post(url.Values{"login": {in}}), probe{}, true)
		if def["login"] == "" || def["login"] == want {
			t.Errorf("%q: no message of the library's own: %q", in, def["login"])
		}
		if got["login"] != want {
			t.Errorf("%q: %q, want %q", in, got["login"], want)
		}
	}
	if _, def := plain.Parse(post(url.Values{"login": {"abcd"}}), probe{}, true); !strings.Contains(def["login"], "3") {
		t.Errorf("the limit is not in the message: %q", def["login"])
	}
}

// A failed check stops at one error: Parse does not pile its own on top, and
// the form still comes back with what was typed.
func TestFailedCheckSkipsParse(t *testing.T) {
	called := false
	rs := probeRes(Field[probe]{Name: "login", Max: 2, Parse: func(*probe, string, bool) error {
		called = true
		return nil
	}})
	p, errs := rs.Parse(post(url.Values{"login": {"abc"}}), probe{}, true)
	if called {
		t.Error("Parse ran after a failed check")
	}
	if errs["login"] == "" {
		t.Error("no error")
	}
	if p.Login != "abc" {
		t.Errorf("Login = %q: the typed value was lost", p.Login)
	}
}

// The "New" button, and anything else above the list, is a declared item —
// present because it was added, gone because permission was denied — never a
// side effect of some caption happening to be set.
func TestHeaderIsFilteredByPermission(t *testing.T) {
	rs := Resource[string]{Path: "/x", Header: []HeaderLink{
		{Title: "New", Href: "/new"},
		{Title: "Import", Href: "/import", Permission: "import"},
	}}

	open := rs.List(nil, 0, gojiffy.Paging{Page: 1, PerPage: 20}, nil, gojiffy.Order{}).Header
	if len(open) != 2 {
		t.Fatalf("with no permissions applied: %d buttons, want 2: %+v", len(open), open)
	}

	limited := rs.For(gojiffy.Perms{}).List(nil, 0, gojiffy.Paging{Page: 1, PerPage: 20}, nil, gojiffy.Order{}).Header
	if len(limited) != 1 || limited[0].Title != "New" {
		t.Fatalf("without the permission: %+v, want just New", limited)
	}

	allowed := rs.For(gojiffy.Perms{"import": true}).List(nil, 0, gojiffy.Paging{Page: 1, PerPage: 20}, nil, gojiffy.Order{}).Header
	if len(allowed) != 2 {
		t.Fatalf("with the permission: %+v, want both", allowed)
	}
}

// A resource may still get its Path filled in after it was declared — a
// client's tokens are given their own address only once the client is
// known — so Header's Href has to be resolved against Path as it stands when
// the list is actually drawn, not when Header was written.
func TestHeaderHrefFollowsPathSetLater(t *testing.T) {
	rs := Resource[string]{Header: []HeaderLink{{Title: "New", Href: "/new"}}}
	rs.Path = "/clients/7/tokens"

	header := rs.List(nil, 0, gojiffy.Paging{Page: 1, PerPage: 20}, nil, gojiffy.Order{}).Header
	if len(header) != 1 || header[0].Href != "/clients/7/tokens/new" {
		t.Fatalf("href = %+v, want /clients/7/tokens/new", header)
	}
}

// The edit form's heading comes from EditTitle, the record's own name and
// all — Resource carries no plain string that could go stale next to it.
func TestFormUsesEditTitle(t *testing.T) {
	rs := probeRes(Field[probe]{Name: "login"})
	rs.EditTitle = func(p probe) string {
		if p.Login == "" {
			return "Account"
		}
		return "Account: " + p.Login
	}

	fv := rs.Form(probe{account: account{Login: "olya"}}, false, nil, nil)
	if fv.Title != "Account: olya" {
		t.Errorf("title = %q", fv.Title)
	}
}

// Not every resource offers editing at all, so EditTitle left unset is a
// normal case and not a way to invite a nil-pointer panic — Title is a plain
// enough thing to show instead.
func TestFormFallsBackToTitleWithoutEditTitle(t *testing.T) {
	rs := probeRes(Field[probe]{Name: "login"})
	rs.Title = "Accounts"

	fv := rs.Form(probe{}, false, nil, nil)
	if fv.Title != "Accounts" {
		t.Errorf("title = %q, want the resource's own Title", fv.Title)
	}
}

package view

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
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
	for _, f := range form(rs, "a").Fields {
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

// form and parse — the calls with no store: a test resource has no LookupChoices
// field unless it says so, and then it calls Form or Parse itself.
func form[T any](rs Resource[T], item T) FormView {
	fv, err := rs.Form(context.Background(), nil, item, false, nil)
	if err != nil {
		panic(err)
	}
	return fv
}

func parse(rs Resource[probe], values url.Values, item probe, creating bool) (probe, map[string]string) {
	p, errs, err := rs.Parse(post(values), nil, item, creating)
	if err != nil {
		panic(err)
	}
	return p, errs
}

// chooser — a store answering LookupChoices fields out of a map.
type chooser map[string][]gojiffy.Choice

// Choices narrows by Search and Values like a real store, but leaves Limit to
// the library, so its own cut is what the tests see.
func (c chooser) Choices(_ context.Context, field string, q gojiffy.ChoiceQuery) ([]gojiffy.Choice, error) {
	list, ok := c[field]
	if !ok {
		return nil, errors.New("no choices for " + field)
	}
	var out []gojiffy.Choice
	for _, ch := range list {
		if strings.Contains(ch.Label, q.Search) && (q.Values == nil || slices.Contains(q.Values, ch.Value)) {
			out = append(out, ch)
		}
	}
	return out, nil
}

// A list too long for a select is asked by what was typed: the form shows how
// the value held reads, Parse asks only about the value sent, and WriteChoices
// hands out at most LookupLimit matches, and only for such a field.
func TestLookupLimitAsksByWhatWasTyped(t *testing.T) {
	rs := probeRes(Field[probe]{
		Name: "system_type", LookupChoices: true, LookupLimit: 2,
		Value: func(p probe) string { return p.SystemType },
	}, Field[probe]{Name: "login"})
	store := chooser{"system_type": {
		{Value: "1", Label: "Olya"}, {Value: "2", Label: "Oleg"}, {Value: "3", Label: "Olena"}, {Value: "4", Label: "Petro"},
	}}

	fv, err := rs.Form(context.Background(), store, probe{SystemType: "4"}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f := fv.Fields[0]; f.Lookup != "/x/choices?field=system_type" || f.Value != "4" || f.ValueText != "Petro" || f.Options != nil {
		t.Errorf("field: %+v", f)
	}
	gone, err := rs.Form(context.Background(), store, probe{SystemType: "9"}, false, nil)
	if f := gone.Fields[0]; err != nil || f.Value != "" || f.ValueText != "" {
		t.Errorf("a value the store no longer offers: %+v, err %v — want nothing chosen", f, err)
	}

	p, errs, err := rs.Parse(post(url.Values{"system_type": {"9"}}), store, probe{SystemType: "4"}, false)
	if err != nil || errs["system_type"] == "" || p.SystemType != "4" {
		t.Errorf("a value outside the store: stored %q, errs %v, err %v", p.SystemType, errs, err)
	}
	p, errs, err = rs.Parse(post(url.Values{"system_type": {"3"}}), store, probe{}, false)
	if err != nil || len(errs) > 0 || p.SystemType != "3" {
		t.Errorf("a value the store offers: stored %q, errs %v, err %v", p.SystemType, errs, err)
	}

	suggest := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		rs.WriteChoices(w, httptest.NewRequest("GET", "/x/choices?"+query, nil), store)
		return w
	}
	var got []gojiffy.Choice
	if w := suggest("field=system_type&q=Ol"); json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got) != 2 || got[0].Label != "Olya" {
		t.Errorf("suggestions: %s", w.Body)
	}
	if w := suggest("field=system_type&q=Zz"); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("nothing found: %q, want []", w.Body)
	}
	if w := suggest("field=login&q=Ol"); w.Code != http.StatusNotFound {
		t.Errorf("a field that is not looked up: code %d", w.Code)
	}
}

// LookupChoices asks the store by Name and then works as Choices do; a field
// that may stay empty gets a blank option to choose, and a store that cannot
// answer — or no store at all — is an error rather than an empty select.
func TestLookupChoicesAskTheStore(t *testing.T) {
	rs := probeRes(Field[probe]{Name: "system_type", LookupChoices: true, Value: func(p probe) string { return p.SystemType }})
	store := chooser{"system_type": {{Value: "win", Label: "Windows"}}}

	fv, err := rs.Form(context.Background(), store, probe{SystemType: "win"}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if o := fv.Fields[0].Options; len(o) != 2 || o[0].Value != "" || !o[1].Selected || o[1].Label != "Windows" {
		t.Errorf("options: %+v, want the blank one and Windows selected", o)
	}

	p, errs, err := rs.Parse(post(url.Values{"system_type": {"mac"}}), store, probe{SystemType: "win"}, false)
	if err != nil || errs["system_type"] == "" || p.SystemType != "win" {
		t.Errorf("a forged value: stored %q, errs %v, err %v", p.SystemType, errs, err)
	}
	p, errs, err = rs.Parse(post(url.Values{"system_type": {""}}), store, probe{SystemType: "win"}, false)
	if err != nil || len(errs) > 0 || p.SystemType != "" {
		t.Errorf("the blank option: stored %q, errs %v, err %v", p.SystemType, errs, err)
	}

	if _, err := rs.Form(context.Background(), nil, probe{}, false, nil); err == nil {
		t.Error("LookupChoices with no store drew a form")
	}
	if _, _, err := rs.Parse(post(url.Values{}), chooser{}, probe{}, false); err == nil {
		t.Error("a store that does not know the field gave no error")
	}
}

// The kind of filter is the resource's to say, not guessed from the field: the
// same choices may be a select, a text box that suggests, or plain text. What
// lives in the store comes from the store of the list — a select of it whole,
// the label of the value a lookup filters by — and the address of suggestions
// answers a filter that is not in the form at all.
func TestFilterIsChosen(t *testing.T) {
	fixed := func(*probe) []Option { return []Option{{Value: "on"}, {Value: "off"}} }
	store := chooser{
		"system_type": {{Value: "1", Label: "Olya"}, {Value: "2", Label: "Oleg"}},
		"login":       {{Value: "1", Label: "olya"}, {Value: "2", Label: "oleg"}},
	}
	rs := probeRes(
		Field[probe]{Name: "status", Search: true, Choices: fixed, Text: func(p probe) string { return p.Status }},
		Field[probe]{Name: "system_type", ListOnly: true, Search: true, Filter: FilterLookup, LookupChoices: true, LookupLimit: 1,
			Text: func(p probe) string { return p.SystemType }},
		Field[probe]{Name: "login", ListOnly: true, Search: true, Filter: FilterSelect, LookupChoices: true,
			Text: func(p probe) string { return p.Login }},
	)
	st := listStore{store, lister{}}

	get := func(query string) Table {
		b, err := rs.ListBlock(httptest.NewRequest("GET", "/x?"+query, nil), st)
		if err != nil {
			t.Fatal(err)
		}
		return b.Data.(ListView).Table
	}

	cols := get("search[system_type]=2&search[login]=1").Columns
	if cols[0].SearchOptions != nil || cols[0].Lookup != "" {
		t.Errorf("choices without a Filter: %+v, want a text box", cols[0])
	}
	if c := cols[1]; c.Lookup != "/x/choices?field=system_type" || c.Query != "2" || c.QueryText != "Oleg" {
		t.Errorf("lookup filter: %+v, want the label of the value picked", c)
	}
	if c := cols[2]; len(c.SearchOptions) != 3 || !c.SearchOptions[1].Selected {
		t.Errorf("select out of the store: %+v, want All and the store's two", c.SearchOptions)
	}

	w := httptest.NewRecorder()
	rs.WriteChoices(w, httptest.NewRequest("GET", "/x/choices?field=system_type&q=Ol", nil), store)
	var got []gojiffy.Choice
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got) != 1 {
		t.Errorf("suggestions for a filter-only field: %s", w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("suggestions headers: %v", w.Header())
	}

	if _, err := rs.ListBlock(httptest.NewRequest("GET", "/x", nil), lister{}); err == nil {
		t.Error("a list store that cannot answer the choices of its filters gave no error")
	}
}

// A choice filter may look for records with nothing in the field — a client with
// no manager: an option of its own right after All, in a select and a lookup
// alike, and read without asking the store, which knows only values that exist.
func TestFilterEmpty(t *testing.T) {
	fixed := func(*probe) []Option { return []Option{{Value: "on"}} }
	store := chooser{"system_type": {{Value: "1", Label: "Olya"}}}
	rs := probeRes(
		Field[probe]{Name: "status", Search: true, Filter: FilterSelect, FilterEmpty: "nobody",
			Choices: fixed, Text: func(p probe) string { return p.Status }},
		Field[probe]{Name: "system_type", Search: true, Filter: FilterLookup, FilterEmpty: "nobody",
			LookupChoices: true, LookupLimit: 5, Text: func(p probe) string { return p.SystemType }},
	)

	query := "search[status]=" + gojiffy.SearchEmpty + "&search[system_type]=" + gojiffy.SearchEmpty
	b, err := rs.ListBlock(httptest.NewRequest("GET", "/x?"+query, nil), listStore{store, lister{}})
	if err != nil {
		t.Fatal(err)
	}
	cols := b.Data.(ListView).Table.Columns
	if o := cols[0].SearchOptions; len(o) != 3 || o[1].Value != gojiffy.SearchEmpty || o[1].Label != "nobody" || !o[1].Selected {
		t.Errorf("select: %+v, want All, then nobody selected, then the choices", o)
	}
	if c := cols[1]; c.Empty.Value != gojiffy.SearchEmpty || c.Empty.Label != "nobody" || c.QueryText != "nobody" {
		t.Errorf("lookup: %+v, want nobody offered and read", c)
	}
}

// listStore — a store of the list that also answers choices, as a real one does.
type listStore struct {
	chooser
	lister
}

// A <select> knows no readonly: a readonly choice offers only the value it holds.
func TestReadonlyChoiceOffersOnlyItsValue(t *testing.T) {
	rs := probeRes(Field[probe]{
		Name: "status", Required: true, Value: func(p probe) string { return p.Status },
		Readonly: func(probe) bool { return true },
		Choices:  func(*probe) []Option { return []Option{{Value: "on"}, {Value: "off"}} },
	})
	if o := form(rs, probe{Status: "off"}).Fields[0].Options; len(o) != 1 || o[0].Value != "off" {
		t.Errorf("options: %+v, want just off", o)
	}
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
	p, errs := parse(rs, values, probe{Count: 3}, true)
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
	p, errs := parse(rs, url.Values{"status": {"on"}}, probe{}, true)
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
	p, _ := parse(rs, url.Values{"status": {"active"}}, probe{Status: "pending"}, false)
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
	p, _ := parse(rs, url.Values{"status": {"open"}}, probe{Status: "locked"}, false)
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
		_, errs := parse(rs, url.Values{"login": {c.in}}, probe{}, true)
		if got := errs["login"] != ""; got != c.bad {
			t.Errorf("%q: error %q, wanted one: %v", c.in, errs["login"], c.bad)
		}
	}

	// Min is not Required: an optional field may stay empty
	opt := probeRes(Field[probe]{Name: "login", Min: 3})
	if _, errs := parse(opt, url.Values{"login": {""}}, probe{}, true); errs["login"] != "" {
		t.Errorf("an empty optional field failed Min: %q", errs["login"])
	}
}

// Regex is checked on a value that is there: an optional field may stay empty,
// and the words of the application's own win over the library's.
func TestRegexCheck(t *testing.T) {
	letters := regexp.MustCompile(`^[a-z]+$`)
	rs := probeRes(Field[probe]{Name: "login", Regex: letters})
	for in, bad := range map[string]bool{"": false, "olya": false, "olya1": true, "../olya": true} {
		if _, errs := parse(rs, url.Values{"login": {in}}, probe{}, true); (errs["login"] != "") != bad {
			t.Errorf("%q: error %q, wanted one: %v", in, errs["login"], bad)
		}
	}

	own := probeRes(Field[probe]{Name: "login", Regex: letters, Errors: FieldErrors{Regex: "letters only"}})
	if _, errs := parse(own, url.Values{"login": {"olya1"}}, probe{}, true); errs["login"] != "letters only" {
		t.Errorf("error %q, want the application's own", errs["login"])
	}
}

// Words of the application's own win; without them the library says it, with
// the number in it.
func TestFieldErrorsOverrideTheDefaults(t *testing.T) {
	plain := probeRes(Field[probe]{Name: "login", Required: true, Min: 2, Max: 3})
	own := probeRes(Field[probe]{Name: "login", Required: true, Min: 2, Max: 3,
		Errors: FieldErrors{Required: "who are you", Min: "too short a name", Max: "too long a name"}})

	for in, want := range map[string]string{"": "who are you", "a": "too short a name", "abcd": "too long a name"} {
		_, def := parse(plain, url.Values{"login": {in}}, probe{}, true)
		_, got := parse(own, url.Values{"login": {in}}, probe{}, true)
		if def["login"] == "" || def["login"] == want {
			t.Errorf("%q: no message of the library's own: %q", in, def["login"])
		}
		if got["login"] != want {
			t.Errorf("%q: %q, want %q", in, got["login"], want)
		}
	}
	if _, def := parse(plain, url.Values{"login": {"abcd"}}, probe{}, true); !strings.Contains(def["login"], "3") {
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
	p, errs := parse(rs, url.Values{"login": {"abc"}}, probe{}, true)
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

// Choices is one list for three places: every value in the filter, what the
// record may take in the form, and nothing else accepted by Parse — asked with
// the record as it came in, and a forged value not stored.
func TestChoicesDriveFilterFormAndParse(t *testing.T) {
	next := map[string][]Option{
		"pending": {{Value: "pending"}},
		"active":  {{Value: "active"}, {Value: "off"}},
	}
	rs := probeRes(Field[probe]{
		Name: "status", Search: true, Filter: FilterSelect, Required: true, Text: func(p probe) string { return p.Status },
		Choices: func(p *probe) []Option {
			if p == nil {
				return []Option{{Value: "pending"}, {Value: "active"}, {Value: "off"}}
			}
			return next[p.Status]
		},
	})

	col := rs.List(nil, 0, gojiffy.Paging{Page: 1, PerPage: 20}, gojiffy.Search{"status": "off"}, gojiffy.Order{}).Table.Columns[0]
	if len(col.SearchOptions) != 4 || !col.SearchOptions[3].Selected {
		t.Errorf("filter: %+v, want All and every value with off selected", col.SearchOptions)
	}

	fv := form(rs, probe{Status: "active"}).Fields[0]
	if len(fv.Options) != 2 || !fv.Options[0].Selected || fv.Value != "" {
		t.Errorf("form: %+v, want the record's own two with active selected", fv)
	}

	p, errs := parse(rs, url.Values{"status": {"active"}}, probe{Status: "pending"}, false)
	if errs["status"] == "" {
		t.Error("a value outside the record's choices was accepted")
	}
	if p.Status != "pending" {
		t.Errorf("Status = %q: a forged value was stored", p.Status)
	}

	// an empty value fails Required and is not stored: that would leave the
	// form with the choices of no status at all
	if p, _ = parse(rs, url.Values{"status": {""}}, probe{Status: "active"}, false); p.Status != "active" {
		t.Errorf("Status = %q: an empty value was stored", p.Status)
	}

	p, errs = parse(rs, url.Values{"status": {"off"}}, probe{Status: "active"}, false)
	if len(errs) > 0 || p.Status != "off" {
		t.Errorf("Status = %q, errs %v: an offered value was refused", p.Status, errs)
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

	fv := form(rs, probe{account: account{Login: "olya"}})
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

	fv := form(rs, probe{})
	if fv.Title != "Accounts" {
		t.Errorf("title = %q, want the resource's own Title", fv.Title)
	}
}

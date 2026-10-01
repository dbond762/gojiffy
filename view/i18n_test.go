package view

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEveryLanguageHasAllKeys(t *testing.T) {
	defer SetLanguage("en")
	for _, lang := range []string{"ru", "uk"} {
		SetLanguage(lang)
		for _, s := range libStrings {
			if got := lib.T(s.key); got == s.key {
				t.Errorf("%s: no translation for %q", lang, s.key)
			}
		}
	}
}

// The footer counter is a plural message: the substitutions have to arrive and
// the form has to change along with the number.
func TestShowingPlural(t *testing.T) {
	defer SetLanguage("en")
	SetLanguage("en")

	one, many := lib.T(showingKey, 1, 1, 1), lib.T(showingKey, 1, 5, 5)
	if one == showingKey || many == showingKey {
		t.Fatal("the showing message was not found")
	}
	if one == many {
		t.Errorf("the form does not follow the number: %q", one)
	}
	for _, s := range []string{one, many} {
		if !strings.Contains(s, "Showing") {
			t.Errorf("the substitution did not happen: %q", s)
		}
	}
}

// An unknown key comes back as it stands, so the page keeps working.
func TestUnknownKeyReturnsID(t *testing.T) {
	if got := lib.T("no_such_key_here"); got != "no_such_key_here" {
		t.Errorf("got %q", got)
	}
}

// The language reaches the page as lang="": a screen reader picks the voice by
// it, and the browser its hyphenation. Unknown languages fall back to English,
// so the attribute never carries something a browser cannot read.
func TestLangAttribute(t *testing.T) {
	defer SetLanguage("en")

	for lang, want := range map[string]string{"uk": "uk", "ru": "ru", "klingon": "en"} {
		SetLanguage(lang)

		w := httptest.NewRecorder()
		Render(w, http.StatusOK, "page.html", Page{Title: "x"})
		if got := `<html lang="` + want + `">`; !strings.Contains(w.Body.String(), got) {
			t.Errorf("SetLanguage(%q): no %s on the page", lang, got)
		}
	}
}

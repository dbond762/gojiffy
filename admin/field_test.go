package admin

import (
	"fmt"
	"testing"

	"github.com/dbond762/gojiffy/view"
)

// The keys admin prints by live in view's catalogue, which knows their plural
// forms: the same words on both sides, or the message comes out untranslated.
func TestKeysAreTranslated(t *testing.T) {
	view.SetLanguage("uk")
	defer view.SetLanguage("en")
	for key, args := range map[string][]any{minKey: {5}, maxKey: {5}, showingKey: {1, 5, 5}} {
		if view.T(key, args...) == fmt.Sprintf(key, args...) {
			t.Errorf("%q has no translation in view's catalogue", key)
		}
	}
}

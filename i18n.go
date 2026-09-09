package gojiffy

import (
	"sync"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/message/catalog"
)

// Catalog — a set of translations and the language they print in. A message
// table of its own rather than message.DefaultCatalog: that one is a single
// table per process, and if both the library and the application wrote to it,
// whichever initialized second would overwrite the first. Each holds its own
// Catalog, and the two never meet.
type Catalog struct {
	b *catalog.Builder

	mu   sync.RWMutex
	p    *message.Printer
	lang string
}

// NewCatalog returns an empty catalogue; fill it with SetString/Set before the
// first render. The default language is English.
func NewCatalog() *Catalog {
	c := &Catalog{b: catalog.NewBuilder(catalog.Fallback(language.English))}
	c.SetLanguage("en")
	return c
}

// SetString registers the translation of key into the language lang. As a
// rule key is the text itself in the source language, as x/text/message
// intends.
func (c *Catalog) SetString(lang, key, translation string) {
	if err := c.b.SetString(language.MustParse(lang), key, translation); err != nil {
		panic(err) // the format is ours at the call site, so a mismatch shows at once
	}
}

// Set registers a message more involved than a single string — one choosing a
// plural form through plural.Selectf, say.
func (c *Catalog) Set(lang, key string, msg ...catalog.Message) {
	if err := c.b.Set(language.MustParse(lang), key, msg...); err != nil {
		panic(err)
	}
}

// SetLanguage picks the language to print in. Call it at startup, before the
// first render; an unknown language falls back to English.
func (c *Catalog) SetLanguage(lang string) {
	tag, err := language.Parse(lang)
	if err != nil {
		tag = language.English
	}
	p := message.NewPrinter(tag, message.Catalog(c.b))
	c.mu.Lock()
	c.p, c.lang = p, tag.String()
	c.mu.Unlock()
}

// Language — the tag printing actually settled on, ready for lang="" in HTML.
// Not what was passed to SetLanguage: an unknown language falls back to
// English, and the page should say so.
func (c *Catalog) Language() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lang
}

// T formats key through the catalogue — like fmt.Sprintf, but with the
// translation found for the current language. A key that is not in the
// catalogue prints as an ordinary format string: the page keeps working
// instead of failing.
func (c *Catalog) T(key string, args ...any) string {
	c.mu.RLock()
	p := c.p
	c.mu.RUnlock()
	return p.Sprintf(key, args...)
}

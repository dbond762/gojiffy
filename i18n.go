package gojiffy

import (
	"sync"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/message/catalog"
)

// Catalog — набор переводов и текущий язык печати. Своя таблица сообщений,
// а не message.DefaultCatalog: тот один на процесс, и если бы и либа, и
// приложение писали в него, вторая инициализация перебила бы первую.
// Каждый держит свой Catalog — они не пересекаются.
type Catalog struct {
	b *catalog.Builder

	mu sync.RWMutex
	p  *message.Printer
}

// NewCatalog возвращает пустой каталог; наполняют его SetString/Set, до
// первой отрисовки. Язык по умолчанию — английский.
func NewCatalog() *Catalog {
	c := &Catalog{b: catalog.NewBuilder(catalog.Fallback(language.English))}
	c.SetLanguage("en")
	return c
}

// SetString регистрирует перевод строки key на языке lang. key — как
// правило, сам текст на исходном языке: так принято в x/text/message.
func (c *Catalog) SetString(lang, key, translation string) {
	if err := c.b.SetString(language.MustParse(lang), key, translation); err != nil {
		panic(err) // формат задаём сами при вызове — несовпадение ловится сразу
	}
}

// Set регистрирует сообщение сложнее одной строки — например, с выбором
// формы множественного числа через plural.Selectf.
func (c *Catalog) Set(lang, key string, msg ...catalog.Message) {
	if err := c.b.Set(language.MustParse(lang), key, msg...); err != nil {
		panic(err)
	}
}

// SetLanguage выбирает язык печати. Вызывать при запуске, до первой
// отрисовки; неизвестный язык откатывается на английский.
func (c *Catalog) SetLanguage(lang string) {
	tag, err := language.Parse(lang)
	if err != nil {
		tag = language.English
	}
	p := message.NewPrinter(tag, message.Catalog(c.b))
	c.mu.Lock()
	c.p = p
	c.mu.Unlock()
}

// T форматирует key через каталог — как fmt.Sprintf, но со своим переводом
// для найденного языка. Ключа нет в каталоге — печатается как обычный
// формат: страница остаётся рабочей, а не пропадает с ошибкой.
func (c *Catalog) T(key string, args ...any) string {
	c.mu.RLock()
	p := c.p
	c.mu.RUnlock()
	return p.Sprintf(key, args...)
}

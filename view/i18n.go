package view

import (
	"golang.org/x/text/feature/plural"

	"github.com/dbond762/gojiffy"
)

// lib — translations of the admin panel itself: form buttons, All in a filter,
// the record counter. The catalogue is its own (see gojiffy.Catalog) and
// private — not message.DefaultCatalog, so that a catalogue an application
// builds the same way cannot overwrite it.
//
// The key is the source text in English, as x/text/message intends: an unknown
// key prints as it stands, so there is no need to invent a separate identifier
// for it. Labels out of a resource description never get here — they are
// finished text already, translated by the application at its end.
var lib = newCatalog()

// libStrings — plain messages with no form to choose: one value per language.
var libStrings = []struct{ key, ru, uk string }{
	{"Menu", "Меню", "Меню"},
	{"Log out", "Выйти", "Вийти"},
	{"Actions", "Действия", "Дії"},
	{"Search", "Найти", "Знайти"},
	{"Reset", "Сбросить", "Скинути"},
	{"No records", "Записей нет", "Записів немає"},
	{"Nothing found", "Ничего не найдено", "Нічого не знайдено"},
	{"Fill in this field", "Заполните поле", "Заповніть поле"},
	{"Previous", "Назад", "Назад"},
	{"Next", "Вперёд", "Вперед"},
	{"All", "Все", "Усі"},
	{"Create", "Создать", "Створити"},
	{"Save", "Сохранить", "Зберегти"},
	{"Cancel", "Отмена", "Скасувати"},
	{"Confirm", "Подтверждение", "Підтвердження"},
	{"Yes", "Да", "Так"},
	{"Sign in", "Вход", "Вхід"},
	{"Enter your login and password", "Введите логин и пароль", "Введіть логін і пароль"},
	{"Login", "Логин", "Логін"},
	{"Password", "Пароль", "Пароль"},
	{"Wrong login or password", "Неверный логин или пароль", "Невірний логін або пароль"},
	{"Access denied", "Нет доступа", "Немає доступу"},
	{"internal error", "внутренняя ошибка", "внутрішня помилка"},
}

// showingKey — the counter in a list footer; the format is the same in every
// language and only the plural forms differ, registered below under this key.
const showingKey = "Showing %[1]d–%[2]d of %[3]d records"

// minKey and maxKey — the length checks of a field: the number decides the
// form of the word "character".
const (
	minKey = "At least %d characters"
	maxKey = "At most %d characters"
)

func newCatalog() *gojiffy.Catalog {
	c := gojiffy.NewCatalog()
	for _, s := range libStrings {
		c.SetString("ru", s.key, s.ru)
		c.SetString("uk", s.key, s.uk)
	}

	// Showing X-Y of N: N decides the form of the word "record". Total is the
	// third positional argument, and %[3]d picks that same one for printing
	// inside each form.
	c.Set("en", showingKey, plural.Selectf(3, "%d",
		"one", "Showing %[1]d–%[2]d of %[3]d record",
		"other", showingKey))
	c.Set("ru", showingKey, plural.Selectf(3, "%d",
		"one", "Показано %[1]d–%[2]d из %[3]d записи",
		"few", "Показано %[1]d–%[2]d из %[3]d записей",
		"many", "Показано %[1]d–%[2]d из %[3]d записей",
		"other", "Показано %[1]d–%[2]d из %[3]d записи"))
	c.Set("uk", showingKey, plural.Selectf(3, "%d",
		"one", "Показано %[1]d–%[2]d з %[3]d запису",
		"few", "Показано %[1]d–%[2]d з %[3]d записів",
		"many", "Показано %[1]d–%[2]d з %[3]d записів",
		"other", "Показано %[1]d–%[2]d з %[3]d запису"))

	c.Set("en", minKey, plural.Selectf(1, "%d", "one", "At least %d character", "other", minKey))
	c.Set("en", maxKey, plural.Selectf(1, "%d", "one", "At most %d character", "other", maxKey))
	c.Set("ru", minKey, plural.Selectf(1, "%d",
		"one", "Не менее %d символа",
		"few", "Не менее %d символов",
		"many", "Не менее %d символов",
		"other", "Не менее %d символа"))
	c.Set("ru", maxKey, plural.Selectf(1, "%d",
		"one", "Не более %d символа",
		"few", "Не более %d символов",
		"many", "Не более %d символов",
		"other", "Не более %d символа"))
	c.Set("uk", minKey, plural.Selectf(1, "%d",
		"one", "Щонайменше %d символ",
		"few", "Щонайменше %d символи",
		"many", "Щонайменше %d символів",
		"other", "Щонайменше %d символу"))
	c.Set("uk", maxKey, plural.Selectf(1, "%d",
		"one", "Щонайбільше %d символ",
		"few", "Щонайбільше %d символи",
		"many", "Щонайбільше %d символів",
		"other", "Щонайбільше %d символу"))

	return c
}

// SetLanguage picks the language of the library strings. Call it at startup,
// before the first render; an unknown language falls back to English. The
// language is one per process: an admin panel is opened by a known circle of
// people, and there is nothing to gain from following each one's browser.
func SetLanguage(tag string) { lib.SetLanguage(tag) }

// T — a library string in the chosen language, the same as {{t}} in templates:
// arguments as for fmt.Sprintf, an unknown key printed as it stands. Packages
// of the library need it (auth); an application has nothing of its own to
// translate through this catalogue — it keeps its own.
func T(key string, args ...any) string { return lib.T(key, args...) }

func t(key string, args ...any) string { return T(key, args...) }

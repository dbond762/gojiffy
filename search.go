package gojiffy

// Search — фильтры из запроса: имя поля → строка поиска. Что с ними делать,
// решает хранилище: SQL здесь нет, диалекта базы либа не знает.
type Search map[string]string

// Order — по какому полю сортировать. Пустое поле означает порядок по умолчанию.
type Order struct {
	Field string
	Desc  bool
}

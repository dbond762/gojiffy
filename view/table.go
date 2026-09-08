package view

// Данные для partials/table.html — про конкретные сущности шаблон не знает.

// Column с непустым Search рисует под заголовком поле фильтра search[Search]:
// текстовый input, а с непустым SearchOptions — select с фиксированным списком
// (первый вариант — пустое значение, «снять фильтр»).
type Column struct {
	Title, Class  string
	Search        string // имя параметра поиска или "" — фильтра нет
	Query         string // что уже введено
	SearchOptions []Option
	Actions       bool   // в строке фильтров сюда встают кнопки «Найти»/«Сбросить»
	SortHref      string // ссылка «отсортировать по этому столбцу»; "" — сортировка запрещена
	SortDir       string // текущее направление: "asc", "desc" или "" — сейчас не по нему
}

// RowAction — действие над строкой. Post рисует кнопку, отправляющую общую форму
// из layout: сама таблица уже завёрнута в форму поиска, а вложенные формы в HTML
// запрещены — кнопка привязывается к внешней через form/formaction.
type RowAction struct {
	Title, Href string
	Post        bool
	Confirm     string // текст подтверждения; "" — без вопроса
}

// Cell с непустым Href рендерится ссылкой, с непустым Actions — колонкой действий:
// одно действие кнопкой, несколько — выпадающим списком. Class — необязательный
// css-класс ячейки, сейчас нужен только для галочки/крестика (см. Field.Bool).
type Cell struct {
	Text, Href, Class string
	Actions           []RowAction
}

type Table struct {
	Columns []Column
	Rows    [][]Cell
	Empty   string // текст, если строк нет
	Action  string // куда уходит форма поиска; "" — форма не рисуется
}

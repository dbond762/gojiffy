package view

import (
	"net/http"
	"strings"

	"github.com/dbond762/gojiffy"
)

// Req — когда поле обязательно.
type Req int

const (
	No       Req = iota // не обязательно
	Yes                 // всегда
	OnCreate            // только при создании (пароль: сменить можно, а можно и не трогать)
)

// Field — поле сущности целиком: и колонка списка, и поле формы.
// Пустой Title убирает его из списка, пустой Label — из формы.
type Field[T any] struct {
	Name string // имя параметра: search[Name] в списке, Name в форме

	// список
	Title  string
	Class  string // css-класс столбца, например для ширины
	Search bool
	// SearchChoices, если задан, рисует фильтр как <select> с фиксированным
	// списком (плюс «Все» первым) вместо текстового поля — для перечислений
	// и булевых признаков, где ILIKE по подстроке не подходит.
	SearchChoices []Option
	Sort          bool           // заголовок становится ссылкой сортировки
	Text          func(T) string // значение ячейки, уже отформатированное
	Href          func(T) string // если задан, ячейка становится ссылкой
	Bool          func(T) bool   // если задан, ячейка рисуется галочкой/крестиком вместо Text

	// форма
	Label        string
	Type         string
	Help         string // подсказка под полем
	HelpEdit     string // подсказка при редактировании, если отличается
	Required     Req
	Options      string                       // ключ набора вариантов → <select>
	Value        func(T) string               // значение поля; nil — берётся Text
	Parse        func(*T, string, bool) error // разбор и валидация; bool — идёт создание
	Readonly     func(T) bool                 // если вернул true, поле только показывается; Parse всё равно решает, что принять
	Autocomplete string

	Permission string // поле видно только с этим правом — и в списке, и в форме
}

// Action — действие над записью: адрес считается из неё самой.
// По умолчанию экшены живут общей пачкой в правом столбце списка (один рисуется
// кнопкой, несколько — выпадающим списком). Column выносит действие в свой
// отдельный столбец — для того, что должно быть на виду, а не под «Действиями».
type Action[T any] struct {
	Title      string
	TitleFor   func(T) string // подпись зависит от записи; nil — везде Title
	Href       func(T) string
	Permission string         // "" — доступно всем
	Column     bool           // свой столбец вместо общей пачки
	Post       bool           // меняет состояние: кнопка-форма, а не ссылка
	Confirm    func(T) string // текст подтверждения; nil или "" — без вопроса
}

// rowAction переводит объявление в то, что рисует шаблон.
func (a Action[T]) rowAction(item T) RowAction {
	r := RowAction{Title: a.Title, Href: a.Href(item), Post: a.Post}
	if a.TitleFor != nil {
		r.Title = a.TitleFor(item)
	}
	if a.Confirm != nil {
		r.Confirm = a.Confirm(item)
	}
	return r
}

func (f Field[T]) inList() bool { return f.Title != "" }
func (f Field[T]) inForm() bool { return f.Label != "" }

// Link — кнопка в шапке панели.
type Link struct{ Title, Href string }

// Resource — описание сущности: одни поля показываются в таблице, другие в форме,
// третьи и там и там. Механика ниже одна на все сущности, см. resources.go.
//
// M — модель, как её отдаёт база; T — запись формы (модель плюс то, чего в ней нет,
// например пароль). Wrap превращает первое во второе для списка.
type Resource[M, T any] struct {
	Path         string         // /users — адрес списка, он же action формы поиска
	Title        string         // Пользователи — заголовок списка
	One          string         // Пользователь — заголовок формы редактирования
	NewTitle     string         // Новый пользователь — заголовок формы создания
	Empty        string         // текст пустого списка
	PerPage      int            // строк на страницу; 0 — gojiffy.PerPageDefault
	DefaultOrder gojiffy.Order  // сортировка, когда в запросе её нет или поле незнакомо
	Href         func(T) string // адрес записи: action формы редактирования
	Wrap         func(M) T
	Fields       []Field[T]
	Actions      []Action[T]
}

// For отдаёт копию описания без того, на что у пользователя нет прав.
// Хендлер работает уже с ней, поэтому скрытое поле не покажется в списке,
// не появится в форме и не будет принято из запроса.
func (rs Resource[M, T]) For(perms gojiffy.Perms) Resource[M, T] {
	fields := make([]Field[T], 0, len(rs.Fields))
	for _, f := range rs.Fields {
		if f.Permission == "" || perms.Can(f.Permission) {
			fields = append(fields, f)
		}
	}
	actions := make([]Action[T], 0, len(rs.Actions))
	for _, a := range rs.Actions {
		if a.Permission == "" || perms.Can(a.Permission) {
			actions = append(actions, a)
		}
	}
	rs.Fields, rs.Actions = fields, actions
	return rs
}

// Paging — какую страницу просит запрос.
func (rs Resource[M, T]) Paging(r *http.Request) gojiffy.Paging {
	return ParsePaging(r.URL.Query(), rs.PerPage)
}

// ParseOrder берёт сортировку из запроса, если такое поле объявлено сортируемым.
// Иначе — DefaultOrder: и когда в ?sort= пусто, и когда там чужое имя.
func (rs Resource[M, T]) ParseOrder(r *http.Request) gojiffy.Order {
	o := ParseOrder(r.URL.Query())
	for _, f := range rs.Fields {
		if f.Sort && f.inList() && f.Name == o.Field {
			return o
		}
	}
	return rs.DefaultOrder
}

// ——— список ———

// ParseSearch берёт из запроса только поля с Search: объявление и есть валидация.
func (rs Resource[M, T]) ParseSearch(r *http.Request) gojiffy.Search {
	q := r.URL.Query()
	s := gojiffy.Search{}
	for _, f := range rs.Fields {
		if !f.Search || !f.inList() {
			continue
		}
		if v := strings.TrimSpace(q.Get("search[" + f.Name + "]")); v != "" {
			s[f.Name] = v
		}
	}
	return s
}

// List собирает страницу списка: шапку, поля фильтров, строки — в порядке объявления
// полей — и футер со счётчиком и ссылками на страницы.
func (rs Resource[M, T]) List(items []M, total int, p gojiffy.Paging, s gojiffy.Search, o gojiffy.Order) ListView {
	t := Table{Empty: rs.Empty}

	filtered := false
	for _, f := range rs.Fields {
		if !f.inList() {
			continue
		}
		col := Column{Title: f.Title, Class: f.Class}
		if f.Search {
			col.Search, col.Query = f.Name, s[f.Name]
			filtered = true
			if len(f.SearchChoices) > 0 {
				col.SearchOptions = selected(withAll(f.SearchChoices), s[f.Name])
			}
		}
		if f.Sort {
			next := gojiffy.Order{Field: f.Name}
			if o.Field == f.Name {
				col.SortDir = "asc"
				if o.Desc {
					col.SortDir = "desc"
				}
				next.Desc = !o.Desc // повторный клик переворачивает порядок
			}
			// сортировка сбрасывает страницу: на седьмой странице нового порядка делать нечего
			col.SortHref = listURL(rs.Path, s, next, 1)
		}
		t.Columns = append(t.Columns, col)
	}

	// вынесенные действия идут своими столбцами, остальные — одной пачкой следом
	var own, menu []Action[T]
	for _, a := range rs.Actions {
		if a.Column {
			own = append(own, a)
		} else {
			menu = append(menu, a)
		}
	}
	for range own {
		t.Columns = append(t.Columns, Column{Class: "col-actions"})
	}
	if len(menu) > 0 {
		t.Columns = append(t.Columns, Column{Class: "col-actions"})
	}
	// без единого фильтра строка поиска не нужна, а с ней и форма вокруг таблицы
	if filtered {
		t.Action = rs.Path
		t.Columns[len(t.Columns)-1].Actions = true
	}

	for _, item := range items {
		row := rs.Wrap(item)
		cells := make([]Cell, 0, len(t.Columns))
		for _, f := range rs.Fields {
			if !f.inList() {
				continue
			}
			var cell Cell
			switch {
			case f.Bool != nil:
				cell = Cell{Text: "✓", Class: "bool-yes"}
				if !f.Bool(row) {
					cell = Cell{Text: "✗", Class: "bool-no"}
				}
			default:
				cell = Cell{Text: f.Text(row)}
			}
			if f.Href != nil {
				cell.Href = f.Href(row)
			}
			cells = append(cells, cell)
		}
		for _, a := range own {
			cells = append(cells, Cell{Actions: []RowAction{a.rowAction(row)}})
		}
		if len(menu) > 0 {
			actions := make([]RowAction, 0, len(menu))
			for _, a := range menu {
				actions = append(actions, a.rowAction(row))
			}
			cells = append(cells, Cell{Actions: actions})
		}
		t.Rows = append(t.Rows, cells)
	}

	var new_ *Link
	if rs.NewTitle != "" {
		new_ = &Link{Title: rs.NewTitle, Href: rs.Path + "/new"}
	}
	return ListView{
		Title:  rs.Title,
		New:    new_,
		Table:  t,
		Footer: listFooter(rs.Path, s, o, p, total),
	}
}

// ——— форма ———

// Parse разбирает запрос по полям формы. creating меняет строгость: при создании
// обязательное поле не может быть пустым.
func (rs Resource[M, T]) Parse(r *http.Request, item T, creating bool) (T, map[string]string) {
	errs := map[string]string{}
	for _, f := range rs.Fields {
		if !f.inForm() || f.Parse == nil {
			continue
		}
		if err := f.Parse(&item, r.PostFormValue(f.Name), creating); err != nil {
			errs[f.Name] = err.Error()
		}
	}
	return item, errs
}

// Form собирает форму для шаблона: значения из записи, ошибки под полями,
// варианты для select из opts.
func (rs Resource[M, T]) Form(item T, creating bool, opts Options, errs map[string]string, csrf string) FormView {
	v := FormView{CancelURL: rs.Path, CSRF: csrf}
	if creating {
		v.Title, v.Submit, v.Action = rs.NewTitle, t("Create"), rs.Path
	} else {
		v.Title, v.Submit, v.Action = rs.One, t("Save"), rs.Href(item)
	}

	for _, f := range rs.Fields {
		if !f.inForm() {
			continue
		}
		fv := FieldView{
			Name:         f.Name,
			Label:        f.Label,
			Type:         f.Type,
			Help:         f.Help,
			Autocomplete: f.Autocomplete,
			Required:     f.Required == Yes || (creating && f.Required == OnCreate),
			Error:        errs[f.Name],
		}
		if !creating && f.HelpEdit != "" {
			fv.Help = f.HelpEdit
		}
		if f.Readonly != nil {
			fv.Readonly = f.Readonly(item)
		}
		switch {
		case f.Value != nil:
			fv.Value = f.Value(item)
		case f.Text != nil:
			fv.Value = f.Text(item)
		}
		if f.Options != "" {
			fv.Options = selected(opts[f.Options], fv.Value)
			fv.Value = ""
		}
		v.Fields = append(v.Fields, fv)
	}
	return v
}

// Options — наборы вариантов для <select>, известные только в рантайме
// (роли из базы и подобное): ключ совпадает с Field.Options.
type Options map[string][]Option

func selected(opts []Option, value string) []Option {
	out := make([]Option, len(opts))
	for i, o := range opts {
		o.Selected = o.Value == value
		out[i] = o
	}
	return out
}

// withAll добавляет первым пунктом «Все» (пустое значение — фильтр снят).
func withAll(opts []Option) []Option {
	return append([]Option{{Label: t("All")}}, opts...)
}

// ListView — то, что уходит в шаблон list.html.
type ListView struct {
	Title  string
	New    *Link
	Notice *Notice // выделенный блок над панелью, см. form.html
	Table  Table
	Footer ListFooter
}

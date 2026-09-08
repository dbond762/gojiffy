package view

// Page — общая обёртка layout.html: шапка, сайдбар, хлебные крошки.
// MenuItem — пункт бокового меню.
type MenuItem struct {
	Link
	Active bool
}

type Page struct {
	Title string
	// UserName — подпись в шапке. Не модель пользователя: приложению виднее,
	// что показывать, а либе тут хватает строки.
	UserName string
	CSRF     string     // для форм в layout (выход)
	Crumbs   []Link     // путь в шапке; последняя крошка — текущая страница, без ссылки
	Menu     []MenuItem // разделы, доступные пользователю
	Data     any
}

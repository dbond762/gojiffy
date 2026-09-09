package view

// Page — the common wrapper of layout.html: header, sidebar, breadcrumbs.
// MenuItem — one entry of the side menu.
type MenuItem struct {
	Link
	Active bool
}

type Page struct {
	Title string
	// UserName — the caption in the header. Not a user model: the application
	// knows better what to show, and a string is all the library needs here.
	UserName string
	CSRF     string     // for the forms in the layout (signing out)
	Crumbs   []Link     // the trail in the header; the last crumb is this page, unlinked
	Menu     []MenuItem // the sections this user may see
	Data     any
}

package view

// MenuItem — one entry of the side menu.
type MenuItem struct {
	Link
	Active bool
}

// Page — the common wrapper of layout.html: header, sidebar, breadcrumbs.
type Page struct {
	Title string
	// UserName — the caption in the header. Not a user model: the application
	// knows better what to show, and a string is all the library needs here.
	UserName string
	CSRF     string     // for the forms in the layout (signing out)
	Crumbs   []Link     // the trail in the header; the last crumb is this page, unlinked
	Menu     []MenuItem // the sections this user may see
	Blocks   []Block    // what the page is made of, drawn in this order
}

// Block — one piece of a page: the partial that draws it and the data for that
// partial. An empty Name is a block that did not happen — a notice with nothing
// to say — and Handler skips it.
//
// Names of partials do not belong in application code: take a block from
// admin.Resource.ListBlock, admin.Resource.FormBlock or NoticeBlock.
type Block struct {
	Name   string
	Title  string // the page title, if this block is what sets it
	Crumbs []Link // the trail in the header, if this block is what sets it
	Data   any
}

// Link — a button in the panel header.
type Link struct{ Title, Href string }

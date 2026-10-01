package view

// PageLink — a link to a page of a list. An empty Href is the ellipsis or the
// current page.
type PageLink struct {
	Title   string
	Href    string
	Current bool
}

// ListFooter — the footer of a list table: always there, the counter always
// shown, page links appearing once there is more than one page.
type ListFooter struct {
	Text       string // the finished counter line: "Showing 1-20 of 42" or "No records"
	Prev, Next string // "" means the button is inactive
	Pages      []PageLink
}

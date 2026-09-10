package view

import "html/template"

// NoticeKind — what a notice is about, and so what colour its frame comes in.
// The zero value is Info.
type NoticeKind string

const (
	Info    NoticeKind = ""
	Success NoticeKind = "success"
	Warning NoticeKind = "warning"
	Error   NoticeKind = "error"
)

// Notice — a highlighted block above the rest of a page. The theme draws the
// frame; what goes inside is the application's own: Text for a plain sentence,
// HTML where it takes markup — a field with a copy button, a link. Both may be
// set, Text goes first. Put it on a page with NoticeBlock.
type Notice struct {
	Kind  NoticeKind
	Title string
	Text  string
	HTML  template.HTML
}

package view

// styles and scripts — the files of the application itself, on every page:
// stylesheets in the head after the theme's own, scripts at the end of the body
// after the theme's. The theme knows about neither — its css is built by
// scanning its own templates and never sees anyone else's classes, its script
// drives its own widgets.
var styles, scripts []string

// SetStyle adds a stylesheet of the application, linked after the theme's.
// Call it at startup, once per file, in the order they should be linked:
//
//	view.SetStyle("/assets/dashboard.css")
func SetStyle(href string) { styles = append(styles, href) }

// SetScript adds a script of the application. It goes at the end of the body,
// so the markup it works on is already parsed by the time it runs.
//
//	view.SetScript("/assets/dashboard.js")
func SetScript(src string) { scripts = append(scripts, src) }

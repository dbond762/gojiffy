package view

import "slices"

// staticPrefix — where the application mounted Static. Templates ask for the
// theme's files through {{asset}} and never name an address themselves: which
// routes a mux gives away is the application's business. A library that took
// /static/ for itself would take it from everyone using it.
var staticPrefix string

// styles — stylesheets of the application, linked after the theme's own.
// Markup an application brings with it — the HTML of a Notice, a partial of
// its own — is styled from here: the theme's css is built by scanning the
// theme's own templates and knows nothing of anyone else's classes.
var styles []string

// SetStyles sets those stylesheets. Call it at startup, as with SetAppName.
//
//	view.SetStyles("/assets/dashboard.css")
func SetStyles(hrefs ...string) { styles = slices.Clone(hrefs) }

// Package gojiffy — an admin panel for Go: an entity is described once, as an
// admin.Resource, and that one description gives the list with its filters,
// the form and the parsing of the request.
//
// This package holds what does not depend on drawing: the filters, sorting and
// page asked for (Search, Order, Paging), the permissions of the current user
// (Perms), the contract of an entity's store (Store and its parts, Chooser)
// and a translation catalogue (Catalog). Sections are in admin, drawing in
// view, signing in in auth, the default look in themes/TailAdmin.
package gojiffy

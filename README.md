# gojiffy

An admin panel for Go: an entity is described by one struct, and that gives
both the list with its filters and the form; the markup and the parsing of the
request come with them.

## What's inside

```
search.go paging.go perms.go   filters, sorting, pages, permissions
store.go                       the contract of an entity's store
i18n.go                        translation catalogue
auth/                          sign-in, session, CSRF, permissions on a route
view/                          Resource/Table/Form/Field + rendering
themes/TailAdmin/              the default look: templates and CSS
```

## Resource

A section is described once: by default a field is both a list column and a
form field, and it is named the same in both places — `Caption`. A field that
belongs in only one of the two says so outright: `ListOnly` (an id, a date) or
`EditOnly` (a password, a pick from a lookup). `Label` is needed only when the
form label has to differ from the column heading. All captions are finished
text: if the application translates them, the description becomes a function,
see "Translations".

```go
type Article struct {
	ID    int
	Title string
}

var Articles = view.Resource[Article]{
	Path:      "/articles",
	Title:     "Articles",
	EditTitle: func(a Article) string {
		if a.Title == "" {
			return "Article"
		}
		return "Article: " + a.Title
	},
	Href:      func(a Article) string { return "/articles/" + strconv.Itoa(a.ID) },
	Fields: []view.Field[Article]{{
		Name:     "title",
		Caption:  "Title",
		Search:   true,
		Sort:     true,
		Required: true,
		Max:      200,
		Text:     func(a Article) string { return a.Title },
	}},
}
```

The same description gives the filters (`ParseSearch`), the sorting
(`ParseOrder`), the page (`Paging`) and the parsing of the form (`Parse`) —
declaring a field is the validation: what is not in the description, the
request does not bring in.

A field is parsed in order. The value is trimmed of spaces (`NoTrim` when
spaces matter, as in a password) and put into the record's string field of the
same name — `title` into `Title`, `system_type` into `SystemType`; then
`Required`, `Min` and `Max` are checked (length in characters, and only for a
non-empty value), and only then is `Parse` called, if there is one. So a field
that just keeps what was typed needs no `Parse` at all; it is for dates,
numbers and rules of your own, and may overwrite the value already put in.
`NoSet` leaves the record untouched for `Parse` — when a rule needs to see what
the record held before parsing. The texts of the check errors are `Errors`;
unset, the library's own message in the page's language is used. The field's
`autocomplete` attribute is called `Autofill`: it is a hint to the browser, not
suggestions while typing.

`For(perms)` returns a copy of the description without what the user has no
permission for, so a hidden field is not shown in the list, does not appear in
the form and is not accepted from the request. Permissions are `Perms`, a set
of strings; where they come from is up to the application.

A button above the list ("New…" and the like) is not a property of the heading
but an action declared on its own, `Header []HeaderLink`, with the same
`Permission` as `Field` and `Action` have: `For(perms)` removes the button
itself, not just the text on it. `NewTitle` stays all the same — it is the
title of the create form once it is reached, and has nothing to do with the
button.

```go
Header: []view.HeaderLink{{Title: "New article", Href: "/new"}},
```

`Href` is resolved against `Path` when the list is drawn, not when the resource
is declared — `Path` is sometimes filled in later, as for a section nested
under another record, whose address is known only once that record is found.

A resource has one type — the record the page sees, usually the model itself.
If the form needs more than the model has (a password, a computed field), the
record is made wider, and the store is brought up to it on the way into the
list:

```go
type UserForm struct {
	models.User
	Password string
}

view.MapList(rs, r, db.Users(), func(u models.User) UserForm {
	return UserForm{User: u}
})
```

## Page

A page is a set of blocks: a notice, a list, a form. A block knows which
partial draws it and carries the data for it; `view.Handler` puts the page
together.

```go
// what only the application knows: who signed in, their menu, csrf
func (a *App) Frame(r *http.Request) (view.Page, error)

// the whole list: filters, sorting and page from the request, records from the
// store, a finished block out
func (a *App) Clients(_ http.ResponseWriter, r *http.Request) (view.Block, error) {
	u := auth.UserFrom(r.Context())
	return resources.ClientsFor(u.Perms).ListBlock(r, a.db.Clients(u.ID))
}

mux.HandleFunc("GET /clients", view.Handler(a.Frame, a.Secret, a.Clients))
```

`Component` is `func(http.ResponseWriter, *http.Request) (Block, error)`.
`ResponseWriter` is there because a block may have a side effect: a one-time
notice clears the cookie it came from. Components are drawn in the order given;
a block without a name is skipped, so "nothing to show" is a plain
`view.Block{}` rather than a special case. An error from any of them is a 500
and not a line of the page: it is assembled in a buffer.

Ready-made blocks: `Resource.ListBlock` (a list), `view.FormBlock` (a form with
its panel and title), `view.NoticeBlock` (a notice; `nil` gives an empty
block). Names of partials never appear in application code.

A notice is only a frame: the colour by `Kind` (`view.Success`,
`view.Warning`, `view.Error`, `view.Info` — the zero one) and a title. The
content is the application's: `Text` for a plain string, `HTML` for markup of
its own — a field with a button, links. A script for such markup is the
application's too; the theme knows nothing of it.

```go
view.NoticeBlock(&view.Notice{
	Kind:  view.Success,
	Title: "Token created",
	Text:  "Copy it and pass it on — it will not be shown again.",
	HTML:  secretField(secret), // markup of your own on the theme's classes
})
```

The page title is taken from the first block that names one. Where a status
code of its own is needed (422 for a form), crumbs of its own, or a 404 before
anything is drawn, `Handler` does not fit — `RenderPage` draws the same blocks:

```go
view.RenderPage(w, r, a.Frame, http.StatusUnprocessableEntity, title, crumbs,
	view.FormBlock(form))
```

A form does not ask for its CSRF token: the page has it (`Page.CSRF`, put there
by `Frame`), and `Handler` and `RenderPage` set it on every form drawn on it.
`Resource.Form` does not see the request and cannot know the token, and a token
forgotten by hand is a refusal on submit. Only the sign-in form keeps its own:
there is no `Page` around it.

Pages go out with `Cache-Control: no-store`, `nosniff`,
`Referrer-Policy: same-origin` and the CSP `frame-ancestors 'none'; form-action
'self'; base-uri 'self'; object-src 'none'`: the admin panel cannot be embedded
in someone else's page, and its forms are sent only back to it. The CSP does
not restrict scripts — the theme's are inline.

## Store

`store.go` declares the contract for getting at records — the one point of
agreement with whoever provides the data.

```go
type Lister[M any] interface {
	List(ctx context.Context, s Search, o Order, p Paging) ([]M, int, error)
}
```

The interfaces are small and separate because entities are rarely alike: one
needs a value to be created that the record itself lacks, another has its list
limited to what a user may see. That limit does not go into the signature —
the store closes over it:

```go
func Articles(db *sql.DB, authorID int) ArticleStore // the constructor takes the limit
func (st ArticleStore) List(...) ([]Article, int, error)
```

So `List` stays the same everywhere, and someone else's record cannot be
reached even by its id. Conformance to the contract is worth pinning with a
compile-time check:

```go
var _ gojiffy.Store[Article] = ArticleStore{}
```

## Sign-in

`auth` provides ready-made sign-in and sign-out handlers, a session in a signed
cookie and a permission check on a route. All it needs from the application is
an `auth.Store`: check a login and password, and return a user by id.

```go
a := auth.New(db, sessionKey, "/clients") // where to go after signing in
a.Mount(mux)                              // GET/POST /login, POST /logout
mux.Handle("/", a.Require(private))       // everything else needs a session

private.HandleFunc("GET /users", a.Can("users.list", h.UsersList))
```

`Require` lets through only a valid session, checks CSRF on every POST
(double-submit: the hidden `_csrf` field against the cookie) and puts the user
into the context — get it with `auth.UserFrom(ctx)`, the token for a form with
`auth.CSRF(r)`. `Can` refuses without the permission; wrap with it what already
sits behind `Require`.

```go
type Store interface {
	Authenticate(ctx context.Context, login, password string) (int, error)
	User(ctx context.Context, id int) (User, error)
	EndSessions(ctx context.Context, id int) error
}
```

How a password is stored and where permissions come from, the library does not
know. `Authenticate` returns an id or an error (any error — "no such login" and
"wrong password" give the same message in the form; they should not differ in
timing either, so compare the password against some hash even when the login
does not exist). `User` is called on every request, so a revoked permission
takes effect at once. The key signs the cookie — change the key and every
session is gone.

The cookies are `Secure` whoever holds HTTPS — the process itself or a proxy in
front: a request does not tell one from the other. Browsers accept them on
`http://localhost`; an admin panel over plain HTTP across a network needs
`a.Insecure = true`.

The session is signed together with `User.Session` — a counter in the user's
record, say. `EndSessions` changes it, and every session of that user ends,
stolen copies of the cookie included. Signing out calls it itself; on a
password change the application calls it.

The library holds back password guessing itself: 5 attempts per login in 15
minutes, after which the form answers 429 until the window ends and
`Authenticate` is not called. A successful sign-in resets the login's counter.
A browser that has already signed in at a login remembers it for 90 days (the
`device` cookie) and is counted on its own, so someone guessing cannot lock the
owner out.

Lifetimes and limits are `Auth` fields with defaults from `New`; change them
before `Mount`:

```go
a.SessionTTL = 8 * time.Hour      // the session; 24 hours
a.DeviceTTL = 30 * 24 * time.Hour // own attempts for a browser that signed in before; 90 days
a.LoginAttempts = 10              // attempts per login in a window; 5
a.LoginWindow = time.Hour         // the window; 15 minutes
```

The `/login` and `/logout` paths are fixed: the theme's templates know them, so
the library holds both ends of the agreement.

## Styles

The default theme's markup is [TailAdmin](https://tailadmin.com) (MIT),
Tailwind CSS. The built `themes/TailAdmin/static/app.css` is in the repository;
rebuild it after changing the theme's templates:

```sh
tailwindcss -i themes/TailAdmin/styles/app.css -o themes/TailAdmin/static/app.css --minify
```

The library serves it itself, and the application decides at what address:

```go
mux.Handle("GET /theme/", view.Static("/theme/"))
```

The address is an argument of that same call rather than a separate setter: it
is written both in the route and in the markup, and if the two drift apart the
page has no styles at all. Templates get it through `{{asset "app.css"}}` and
name no address themselves — a name like `/static/` belongs to whoever writes
the application, not to the library they use. No `StripPrefix` is needed, the
library strips the prefix itself: it has been told it. Without a call to
`Static` it is `/static/`.

Markup of your own comes with styles and a script of your own. The application
serves the files itself, and the library links them: css in `<head>` after the
theme's, js at the end of `<body>` after the theme's own — by then the markup it
works on has been parsed.

```go
mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(mine)))
view.SetStyle("/assets/app.css")
view.SetScript("/assets/app.js")
```

Each call adds one file, in the order of the calls: there is no whole list to
keep, files are declared where they are known about.

## Theme

A look is a filesystem of a known shape: `templates/` and `static/` at its
root. `themes/TailAdmin` is used by default, but it can be replaced whole or
have single files swapped in it.

```go
//go:embed templates static     // a root with its own templates/ and static/
var mine embed.FS

view.SetTheme(mine)             // your own look instead of the default one
view.Override(mine)             // your own files over the default one
```

Both calls are made at startup, before the first render, and both return an
error if a template did not parse; the look then stays as it was and pages keep
working. `Override` can be called more than once, each layer covering the ones
before it; `view.SetTheme(tailadmin.FS)` puts everything back. For editing live,
`os.DirFS("theme")` will do.

The directory names inside matter — `templates` and `static`; if yours sit
deeper, pass `fs.Sub(mine, "your/dir")`.

### What a theme must contain

| file | entry point | data |
|---|---|---|
| `templates/layout.html` | `{{define "layout"}}` + `{{block "content" .}}` | `view.Page` |
| `templates/page.html` | `{{define "content"}}` | `view.Page`, draws `.Blocks` |
| `templates/login.html` | `{{define "login.html"}}` | `view.FormView`, no `Page` |
| `templates/partials/*.html` | `list`, `form-panel`, `notice`, `form`, `field`, `table`, `row-actions`, `list-footer` | see below |
| `static/app.css` | — | served where `view.Static` was mounted |

There is one page for every section, because what it shows is blocks, not a
view of its own of a record:

```html
{{define "content"}}{{range .Blocks}}{{render .}}{{end}}{{end}}
```

`login.html` gets the partials but not the layout: it is a document of its own,
and its entry point is named after the file.

What each one is given: `list` — `view.ListView`; `form-panel` and
`login.html` — `view.FormView`; `notice` — `*view.Notice`; `form` —
`view.FormView` (called from `form-panel`); `field` — `view.FieldView` (from
`form`); `table` — `view.Table`; `row-actions` — `[]view.RowAction` (from
`table`); `list-footer` — `view.ListFooter`.

Available in templates: `{{app}}` — the application's name
(`view.SetAppName`); `{{t "Save"}}` — a library string in the chosen language
(`view.SetLanguage`), with arguments as for `fmt.Sprintf`; `{{lang}}` — the
code of that language for `<html lang="…">` (an unknown language falls back to
`en`, and `{{lang}}` returns what is actually printed in); `{{asset "app.css"}}`
— the address of a theme file where it was mounted; `{{styles}}` and
`{{scripts}}` — the application's files (`view.SetStyle`, `view.SetScript`);
and `{{render}}` — draw a block with the partial of its name. That last one is
what keeps the set of blocks open: a block of your own works as soon as the
theme has a partial of that name — there is nothing to change in Go.

Three links not to lose when replacing `layout.html`: `<form
id="post-action">` with `_csrf` — the action buttons in table rows hang on it
through `form`/`formaction` (nested forms are not allowed in HTML);
`<dialog id="confirm">` with the `data-confirm` and `data-copy` handlers;
`POST /logout` with `_csrf`. Changing the layout — either keep them or change
`partials/row_actions.html` along with it.

A file is overridden at the same path. A new file in `partials/` with a
`{{define}}` already taken also joins the set and silently wins by alphabet.

### Tailwind classes in your own templates

The theme's `static/app.css` is built by scanning its own `templates/`
(`@source "../templates"`). A class that was not there is not in the built
CSS — your template will not get it. The options: stick to the theme's classes
(`btn`, `btn-primary`, `field-input`, `field-select`, `table`, `menu-item`…),
or write css of your own and link it through `view.SetStyle` — it comes after
the theme, so the theme's variables (`--color-gray-800`, `--radius-lg`) are
available in it, and its rules override the theme's utilities.

## Translations

The library translates only its own strings — the form buttons, "All" in a
filter, the record counter. They are registered in code (`view/i18n.go`),
currently English, Russian and Ukrainian; the language is chosen at startup:

```go
view.SetLanguage("ru")
```

The key is the English text itself, as is the custom in
[x/text/message](https://pkg.go.dev/golang.org/x/text/message): no translation
found — the key itself is printed, and the page keeps working.

Captions from a resource's description — section titles, field names, hints,
action texts — arrive here **as finished text**. The library has no dictionary
for them and cannot reach into someone else's — these strings are not
translated at all, only printed as they are:

```go
func Articles() view.Resource[Article] {
	return view.Resource[Article]{
		Title:  "Articles",
		Fields: []view.Field[Article]{{Caption: "Title"}},
	}
}
```

If the application needs translation itself, it has a catalogue of its own —
there is no shared table, so an update of the library cannot override its
translation or the other way round:

```go
type Catalog struct{ ... } // gojiffy.Catalog

cat := gojiffy.NewCatalog()
cat.SetString("de", "Draft", "Entwurf")
cat.SetLanguage("de")
cat.T("Draft") // "Entwurf"
```

`SetString` is one message per language; `Set` is the same with an arbitrary
`catalog.Message`, for example with a plural form picked through
[plural.Selectf](https://pkg.go.dev/golang.org/x/text/feature/plural), as in the
footer's counter (`showingKey` in `view/i18n.go`).

## Application name

The caption at the top of the sidebar and the tail of the tab title are set
once at startup, before the first render:

```go
view.SetAppName("Name")
```

Templates take it through the `{{app}}` function rather than from the page's
data: there is one name per process but many pages, and sign-in is drawn
without the common frame at all. Unset, it is `Admin`.

## License

Copyright 2026 Dmytro Bondarenko. Licensed under the
[Apache License, Version 2.0](LICENSE); see [NOTICE](NOTICE). The TailAdmin
markup in `themes/TailAdmin` is under its own MIT license.

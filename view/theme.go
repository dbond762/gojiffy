package view

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"slices"
	"strings"

	tailadmin "github.com/dbond762/gojiffy/themes/TailAdmin"
)

// Оформление — это файловая система, в корне которой лежат templates/ и
// static/. Тип для неё не заведён намеренно: fs.FS уже описывает всё нужное, а
// своя обёртка заставила бы каждую тему знать про либу.

// layers — тема и то, что приложение положило поверх неё. Ищем сверху вниз,
// поэтому подменяется по одному файлу, а не всё оформление разом.
var layers []fs.FS

// SetTheme меняет оформление целиком. fsys — файловая система с templates/ и
// static/ в корне, см. themes/TailAdmin. Вызывать при запуске, до первой
// отрисовки, как SetAppName.
//
// Ошибка означает, что тема неполная или шаблон в ней не собрался; оформление
// при этом остаётся прежним, страницы не ломаются. Он же возвращает всё на
// место после Override: SetTheme(tailadmin.FS).
func SetTheme(fsys fs.FS) error { return apply([]fs.FS{fsys}) }

// Override кладёт файлы поверх текущего оформления: что есть в fsys — берётся
// оттуда, остальное остаётся от темы. Форма та же (templates/..., static/...),
// и подменять файл нужно по тому же пути.
//
// Вызовов может быть несколько, каждый следующий слой перекрывает предыдущие.
func Override(fsys fs.FS) error { return apply(slices.Concat(layers, []fs.FS{fsys})) }

// Static отдаёт static/ оформления. Монтировать по /static/:
//
//	mux.Handle("GET /static/", view.Static())
//
// Этот адрес зашит в шаблонах (<link rel="stylesheet" href="/static/app.css">),
// поэтому оба конца договорённости держит либа — и смена темы роутинга не
// касается. Слои читаются на каждый запрос, так что порядок вызова со SetTheme
// неважен.
func Static() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.FileServerFS(overlay(layers)).ServeHTTP(w, r)
	})
}

func init() {
	// Тема по умолчанию вшита в бинарь: сломанный шаблон здесь — ошибка сборки
	// либы, а не приложения, поэтому падаем сразу, как раньше template.Must.
	if err := SetTheme(tailadmin.FS); err != nil {
		panic(err)
	}
}

// apply собирает шаблоны заново и подменяет их разом. Именно заново: набор
// html/template после разбора не доопределить, а старый живёт, пока на него
// ссылается уже начатый Render.
func apply(l []fs.FS) error {
	fsys := overlay(l)

	built := make(map[string]*template.Template, len(pages)+1)
	for _, name := range pages {
		tpl, err := template.New(name).Funcs(funcs).ParseFS(fsys,
			"templates/layout.html", "templates/partials/*.html", "templates/"+name)
		if err != nil {
			return fmt.Errorf("шаблон %s: %w", name, err)
		}
		built[name] = tpl
	}
	// Вход собирается без layout: это отдельный документ, и точка входа в нём
	// названа именем файла, см. Render.
	login, err := template.New("login.html").Funcs(funcs).ParseFS(fsys,
		"templates/partials/*.html", "templates/login.html")
	if err != nil {
		return fmt.Errorf("шаблон login.html: %w", err)
	}
	built["login.html"] = login

	// Присваивание последней строкой: неудачная тема не должна оставлять
	// приложение без оформления — только поэтому возвращаемая ошибка и нужна.
	layers, templates = l, built
	return nil
}

// overlay — стопка файловых систем: тема снизу, слои приложения сверху.
// Реализуем fs.FS, а не свой разбор шаблонов: тогда ParseFS, fs.Glob и
// http.FileServerFS работают со слоями сами, а вызовы ParseFS в apply остаются
// теми же, что были до появления тем.
type overlay []fs.FS

func (o overlay) Open(name string) (fs.File, error) {
	for i := len(o) - 1; i >= 0; i-- {
		if f, err := o[i].Open(name); err == nil {
			return f, nil
		}
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// ReadDir нужен ради partials/*.html: без него fs.Glob прочитал бы каталог
// одного слоя и потерял бы partials остальных. Одноимённые файлы схлопываются
// в верхний — он же откроется на чтение.
func (o overlay) ReadDir(name string) ([]fs.DirEntry, error) {
	var all []fs.DirEntry
	seen := map[string]bool{}
	for i := len(o) - 1; i >= 0; i-- {
		entries, err := fs.ReadDir(o[i], name)
		if err != nil {
			continue // в этом слое каталога нет — значит есть в другом
		}
		for _, e := range entries {
			if !seen[e.Name()] {
				seen[e.Name()] = true
				all = append(all, e)
			}
		}
	}
	if all == nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	// fs.ReadDir обязана отдавать отсортированное, и здесь это не формальность:
	// от порядка разбора зависит, чей {{define}} окажется последним.
	slices.SortFunc(all, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return all, nil
}

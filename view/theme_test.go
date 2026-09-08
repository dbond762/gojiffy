package view

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	tailadmin "github.com/dbond762/gojiffy/themes/TailAdmin"
)

// Слой подменяет ровно один файл, остальное берётся из темы. Иначе оформление
// было бы «всё или ничего»: ради одной правки пришлось бы копировать к себе все
// десять шаблонов и потом тащить за ними каждое изменение в либе.
func TestOverrideReplacesOneFile(t *testing.T) {
	t.Cleanup(func() { SetTheme(tailadmin.FS) })

	err := Override(fstest.MapFS{
		"templates/list.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}<p>своя вёрстка</p>{{end}}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "list.html", Page{Title: "x"})
	body := w.Body.String()

	if !strings.Contains(body, "своя вёрстка") {
		t.Error("страница не взяла переопределённый шаблон")
	}
	if !strings.Contains(body, "<!doctype html>") {
		t.Error("слой снёс layout темы, а должен был подменить только list.html")
	}
	// partials темы попадают в набор через glob по каталогу: если бы overlay не
	// объединял слои, а отдавал каталог верхнего, их бы тут не было.
	if !strings.Contains(body, "/static/app.css") {
		t.Error("layout и partials темы не доехали до набора")
	}
}

// Свой partial не должен утаскивать за собой остальные: они лежат в одном
// каталоге и попадают в набор через glob, а тот читает каталог — значит слои
// нужно объединять, иначе от темы останется ровно один подменённый файл.
func TestOverrideKeepsSiblingPartials(t *testing.T) {
	t.Cleanup(func() { SetTheme(tailadmin.FS) })

	err := Override(fstest.MapFS{
		"templates/partials/list_footer.html": &fstest.MapFile{
			Data: []byte(`{{define "list-footer"}}<p>свой футер</p>{{end}}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "list.html", Page{Title: "x", Data: ListView{
		Table: Table{
			Columns: []Column{{Title: "Имя"}},
			Rows:    [][]Cell{{{Text: "ячейка из темы"}}},
		},
	}})
	body := w.Body.String()

	if !strings.Contains(body, "свой футер") {
		t.Error("свой partial не подхватился")
	}
	if !strings.Contains(body, "ячейка из темы") {
		t.Error("соседние partials темы потерялись — слои каталогов не объединены")
	}
}

// Неполная тема — ошибка приложению, а не паника и не пустые страницы:
// прежнее оформление остаётся рабочим.
func TestSetThemeIncomplete(t *testing.T) {
	if err := SetTheme(fstest.MapFS{}); err == nil {
		t.Fatal("пустая тема принята")
	}

	w := httptest.NewRecorder()
	Render(w, http.StatusOK, "list.html", Page{Title: "x", Data: ListView{}})
	if w.Code != http.StatusOK {
		t.Fatalf("после неудачной темы страница сломалась: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<!doctype html>") {
		t.Error("прежнее оформление не уцелело")
	}
}

// Статику отдаёт либа, а не приложение: адрес /static/app.css зашит в шаблонах.
func TestStaticServesTheme(t *testing.T) {
	w := httptest.NewRecorder()
	Static().ServeHTTP(w, httptest.NewRequest("GET", "/static/app.css", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("код %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Error("пустой css")
	}
}

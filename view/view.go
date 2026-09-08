package view

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
)

// pages — страницы с общим каркасом layout.html. Список фиксирован: набор
// страниц задаёт либа своими вызовами Render, а не оформление.
var pages = []string{"list.html", "form.html"}

// templates — разобранное оформление, по набору на страницу. Заполняет apply,
// см. theme.go: list.html и form.html оба определяют "content", поэтому в один
// набор их не собрать.
var templates = map[string]*template.Template{}

// appName — как приложение называется: подпись в шапке сайдбара и хвост
// заголовка вкладки. Шаблоны берут его функцией {{app}}, а не из данных
// страницы: название одно на процесс, а страниц много, и вход рисуется вообще
// без обвязки.
var appName = "Admin"

// SetAppName задаёт название приложения. Вызывать до первой отрисовки —
// один раз при запуске.
func SetAppName(name string) { appName = name }

var funcs = template.FuncMap{
	"app": func() string { return appName },
	"t":   t,
}

// Render складывает страницу в буфер, чтобы ошибка шаблона не уехала в уже отданный 200.
func Render(w http.ResponseWriter, status int, name string, data any) {
	tpl, ok := templates[name]
	if !ok {
		log.Printf("нет шаблона %s", name)
		http.Error(w, t("internal error"), http.StatusInternalServerError)
		return
	}

	entry := "layout"
	if name == "login.html" {
		entry = name
	}

	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, entry, data); err != nil {
		log.Printf("шаблон %s: %v", name, err)
		http.Error(w, t("internal error"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Страницы админки в истории браузера не хранятся: no-store выключает и
	// bfcache, иначе кнопка «Назад» показывала бы страницу заново — со всем,
	// что на ней было. Показанное один раз (секрет, подтверждение) кэш иначе
	// возвращал бы сколько угодно.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

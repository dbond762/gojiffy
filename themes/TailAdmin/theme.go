// Package tailadmin — оформление админки по умолчанию: шаблоны и собранный CSS.
// Вёрстка взята из TailAdmin (MIT), см. LICENSE.txt рядом.
//
// Про view пакет не знает намеренно: тема — это просто файловая система нужной
// формы, и чужая тема тоже не обязана тащить за собой либу.
package tailadmin

import "embed"

// FS — тема целиком: templates/ и static/ лежат в корне, как ждёт view.SetTheme.
//
// styles/ в бинарник не едет — исходник CSS нужен при сборке, а не при
// отрисовке. LICENSE.txt едет: собранный CSS уезжает в чужой бинарник, и
// лицензия должна ехать вместе с ним.
//
//go:embed templates static LICENSE.txt
var FS embed.FS

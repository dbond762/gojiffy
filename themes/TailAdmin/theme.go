// Package tailadmin — the default look of the admin panel: templates and the
// built CSS. The markup comes from TailAdmin (MIT), see LICENSE.txt next to it.
//
// It deliberately knows nothing of view: a theme is just a filesystem of the
// right shape, and a theme written elsewhere should not have to drag the
// library along either.
package tailadmin

import "embed"

// FS — the whole theme: templates/ and static/ sit at the root, as
// view.SetTheme expects.
//
// styles/ does not travel into the binary — the CSS source is needed at build
// time, not at render time. LICENSE.txt does travel: the built CSS ends up in
// a binary that is not ours, and the licence has to go with it.
//
//go:embed templates static LICENSE.txt
var FS embed.FS

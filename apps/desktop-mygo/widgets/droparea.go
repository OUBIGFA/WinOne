package widgets

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
)

// DropArea makes target take files and folders dragged from File Explorer
// and checks them with DropPaths; its border shows when they are over it.
// A drop while disabled is taken and ignored, so it never reaches another
// area, as the React window's drop zones consumed theirs.
func DropArea(c *ui.Context, env *core.Env, target ui.Element, kind DropKind, multiple, disabled bool, done func(paths []string, err error)) {
	if target.FileDragOver() && !disabled {
		target.Border(1, c.Theme().Accent)
	}
	if dropped := target.DroppedFiles(); len(dropped) > 0 && !disabled {
		DropPaths(env, append([]string(nil), dropped...), kind, multiple, done)
	}
}

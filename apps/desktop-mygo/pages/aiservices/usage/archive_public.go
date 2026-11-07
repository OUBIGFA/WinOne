package usage

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
)

// Archive shares the bounded preview and full export used by Usage with the
// routing detail. The selected record owns its state, including save receipts.
type Archive struct {
	view  archiveView
	owner *Usage
}

func NewArchive(env *core.Env, id string) *Archive {
	a := &Archive{view: archiveView{id: id}, owner: &Usage{env: env}}
	a.view.read(a.owner)
	return a
}
func (a *Archive) View(c *ui.Context) { a.view.view(c, a.owner) }

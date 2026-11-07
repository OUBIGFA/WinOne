package usage

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
)

// Allowances lets Routing reuse the account windows, balances, trends and
// refresh behavior already used by Usage, without a second quota reader.
type Allowances struct {
	state    *allowances
	revision int
}

func NewAllowances(env *core.Env) *Allowances { return &Allowances{state: newAllowances(env)} }
func (a *Allowances) Close()                  { a.state.deactivate() }
func (a *Allowances) Refresh()                { a.revision++ }
func (a *Allowances) View(c *ui.Context, providers []Provider, prefs *Preferences, change func(Preferences)) {
	a.state.view(c, providers, prefs, change, a.revision)
}

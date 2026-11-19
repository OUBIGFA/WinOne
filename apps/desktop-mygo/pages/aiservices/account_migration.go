package aiservices

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type accountWithdrawal struct{ ID, Name, Instead, Package string }

func (a *accountPanel) migrationView(c *ui.Context, pr *provider, disabled bool) {
	if a.withdrawn == nil {
		return
	}
	kit.Notice(c, kit.Caution, i18n.T("aiSync.accountWithdrawn"), i18n.T("aiSync.antigravityWithdrawal"), func() {
		kit.Help(c, i18n.T("aiSync.antigravityMoveHint"))
		if kit.Button(c, i18n.T("aiSync.moveToAgy"), icons.ArrowRight, a.importBusy).Disabled(disabled || a.importBusy || !a.p.connectionSaved(pr) || len(a.accounts) == 0).Clicked() {
			a.moveToAgy(pr)
		}
		for _, result := range a.moveResults {
			kit.Help(c, result.User+" · "+nativeT("import."+result.Status))
		}
	})
}

func (a *accountPanel) moveToAgy(pr *provider) {
	if a.importBusy {
		return
	}
	a.importBusy, a.err = true, ""
	owner := a.owner
	request := map[string]string{"provider": pr.ID, "agent": "antigravity"}
	core.Go(a.p.env, func() ([]accountImportResult, error) {
		var out struct {
			Results []accountImportResult `json:"results"`
		}
		err := a.p.env.Host.CallCompletingInto("capture.ai.accounts.move-agy", request, &out)
		return out.Results, err
	}, func(results []accountImportResult, err error) {
		if owner != a.owner {
			return
		}
		a.importBusy = false
		if err != nil {
			a.err = err.Error()
			return
		}
		a.moveResults = results
		all := len(results) > 0
		for _, result := range results {
			all = all && result.Error == "" && (result.Status == "added" || result.Status == "exists" || result.Status == "updated")
		}
		if !all {
			a.err = i18n.T("aiSync.accountMovePartial")
			return
		}
		// Both adapters used this same private profile. Change only its backend
		// draft after every account succeeds; the original auth stays on disk.
		a.p.change(pr.ID, func(p *provider) { p.Agent, p.AgentBackend = "agy-cli", "plugin" })
		a.p.env.Toast(i18n.T("aiSync.accountMoveSave"))
	})
}

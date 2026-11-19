package aiservices

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type warmupPanel struct {
	user    string
	busy    bool
	err     string
	results []warmupResult
}
type warmupResult struct {
	User  string `json:"user"`
	Skip  string `json:"skip,omitempty"`
	Error string `json:"error,omitempty"`
}

func (a *accountPanel) warmupView(c *ui.Context, r accountRequest, disabled bool) {
	if r.Agent != "codex" && r.Agent != "claude" || len(a.accounts) == 0 {
		return
	}
	w := &a.warmup
	kit.Group(c, icons.Activity, i18n.T("aiSync.warmup"), i18n.T("aiSync.warmupHint"), func() {
		kit.Stack(c, "", "", func() {
			choices := options("", i18n.T("aiSync.warmAllAccounts"))
			for _, account := range a.accounts {
				choices = append(choices, kit.SelectOption{Value: account.User, Label: account.User})
			}
			nativeSelect(c, i18n.T("aiOptions.account"), &w.user, choices, disabled || w.busy || r.Draft != nil)
			if kit.Button(c, i18n.T("aiSync.warmNow"), icons.Play, w.busy).Disabled(disabled || w.busy || r.Draft != nil).Clicked() {
				owner, user := a.owner, w.user
				method := "capture.ai.accounts.warm-now"
				if user == "" {
					method = "capture.ai.accounts.warm-all"
				}
				w.busy, w.err, w.results = true, "", nil
				core.Go(a.p.env, func() ([]warmupResult, error) {
					var reply struct {
						Results []warmupResult `json:"results"`
					}
					err := a.p.env.Host.CallCompletingInto(method, map[string]string{"provider": r.Provider, "agent": r.Agent, "user": user}, &reply)
					return reply.Results, err
				}, func(results []warmupResult, err error) {
					if owner != a.owner {
						return
					}
					w.busy = false
					if err != nil {
						w.err = err.Error()
						return
					}
					w.results = results
				})
			}
			if r.Draft != nil {
				kit.Help(c, i18n.T("aiSync.warmSaveFirst"))
			}
			kit.InlineError(c, w.err)
			for _, result := range w.results {
				outcome := i18n.T("aiSync.warmed")
				switch {
				case result.Error != "":
					outcome = result.Error
				case result.Skip == "off":
					outcome = i18n.T("aiSync.warmOff")
				case result.Skip == "running":
					outcome = i18n.T("aiSync.warmRunning")
				case result.Skip != "":
					outcome = result.Skip
				}
				kit.Help(c, result.User+" · "+outcome)
			}
		})
	})
}

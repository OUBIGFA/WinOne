package aiservices

import (
	"github.com/egoist/mygo/ui"
	"slices"
	"strings"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/ai"
)

func (e *editor) callerAccountPreferences(c *ui.Context, pr *provider, locked bool) {
	callers := []string{"magpie", "claude", "codex", "opencode", "gemini", "copilot"}
	for _, name := range sortedKeys(pr.AgentAccounts) {
		if !slices.Contains(callers, name) {
			callers = append(callers, name)
		}
	}
	choices := options("", i18n.T("ai.automatic"))
	for _, id := range e.preferenceAccounts(pr) {
		label := id
		if name := pr.AccountNames[id]; name != "" {
			label = name
		}
		for _, key := range pr.slots() {
			if key.ID == id && key.Name != "" {
				label = key.Name
			}
		}
		choices = append(choices, kit.SelectOption{Value: id, Label: label})
	}
	kit.Group(c, nil, i18n.T("aiUpdate.agentAccounts"), i18n.T("aiUpdate.agentAccountsHint"), func() {
		for _, name := range callers {
			current := pr.AgentAccounts[name]
			label := name
			if name == "magpie" {
				label = "WinOne"
			}
			kit.Stack(c.Key(name), "", "", func() { nativeSelect(c, label, &current, choices, locked) })
			if current != pr.AgentAccounts[name] {
				if pr.AgentAccounts == nil {
					pr.AgentAccounts = map[string]string{}
				}
				if current == "" {
					delete(pr.AgentAccounts, name)
				} else {
					pr.AgentAccounts[name] = current
				}
			}
		}
		kit.Stack(c, "", "", func() {
			f := e.fields.get("newCaller")
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				kit.TextInput(c, &f.text).Grow(1).MinWidth(0).Label(i18n.T("aiUpdate.addCaller")).Placeholder(i18n.T("aiUpdate.addCaller")).Disabled(locked)
				if kit.IconButton(c, icons.Plus, i18n.T("aiUpdate.addCaller")).Disabled(locked || strings.TrimSpace(f.text) == "").Clicked() {
					if pr.AgentAccounts == nil {
						pr.AgentAccounts = map[string]string{}
					}
					pr.AgentAccounts[strings.ToLower(strings.TrimSpace(f.text))] = ""
					f.text, f.shown = "", ""
				}
			})
		})
	})
}

func (e *editor) keyBalancePreferences(c *ui.Context, pr *provider, id string, locked bool) {
	auth := pr.KeyBalanceAuth[id]
	kit.Group(c, nil, i18n.T("aiUpdate.balanceAuth"), i18n.T("aiUpdate.balanceAuthHint"), func() {
		kit.Stack(c, i18n.T("aiUpdate.balanceToken"), "", func() {
			f := e.fields.get("balanceToken:" + id)
			current := ""
			if auth.Token != nil {
				current = *auth.Token
			}
			if text, changed := f.sync(current); changed {
				auth.Token = new(text)
			}
			placeholder := "ai.keyPlaceholder"
			if auth.HasToken {
				placeholder = "ai.keySaved"
			}
			kit.TextInput(c, &f.text).FillWidth().Label(i18n.T("aiUpdate.balanceToken")).Password().Placeholder(i18n.T(placeholder)).Disabled(locked)
			nativeText(c, i18n.T("aiUpdate.balanceUser"), &auth.User, locked)
		})
		if kit.GhostButton(c, i18n.T("aiUpdate.clearBalance"), icons.RotateCcw, false).Disabled(locked).Clicked() {
			auth = ai.KeyBalanceAuth{Token: new("")}
			f := e.fields.get("balanceToken:" + id)
			f.text, f.shown = "", ""
		}
	})
	if auth.Token != nil || auth.HasToken || auth.User != "" {
		if pr.KeyBalanceAuth == nil {
			pr.KeyBalanceAuth = map[string]ai.KeyBalanceAuth{}
		}
		pr.KeyBalanceAuth[id] = auth
	}
}

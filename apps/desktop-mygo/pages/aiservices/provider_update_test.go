package aiservices

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"testing"
	"winone/desktopmygo/i18n"
	"winone/internal/ai"
)

func TestUpdatedProviderControlsSaveAndReopen(t *testing.T) {
	f, h, p, _ := setup(t)
	pr, e := p.current(), p.editor
	pr.Preset = "remote-magpie"
	pr.Addresses = []string{"https://backup.invalid"}
	pr.AgentAccounts = map[string]string{"codex": "primary"}
	pr.Routing = "pace-drain"
	pr.KeyBalanceAuth = map[string]ai.KeyBalanceAuth{"primary": {Token: new("fixture-balance"), User: "42"}}
	e.preferenceAccount = "primary"
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).FillWidth().Children(func() { e.accountPreferences(c, pr, false) }) }, 900, 2400)
	settle(h, tt)
	shot(t, tt, "ai-updated-provider-options")
	for _, label := range []string{i18n.T("aiUpdate.agentAccounts"), i18n.T("aiUpdate.balanceAuth"), i18n.T("aiUpdate.balanceToken")} {
		if _, ok := tt.Find(label); !ok {
			t.Fatal("missing retained control", label)
		}
	}
	p.save()
	settle(h, tt)
	saved := f.lastSave()
	if saved == nil {
		t.Fatal("save missing")
	}
	raw, _ := json.Marshal(saved["configuration"])
	var reopened configuration
	if err := json.Unmarshal(raw, &reopened); err != nil {
		t.Fatal(err)
	}
	got := reopened.Providers[0].Value
	if got.Routing != "pace-drain" || len(got.Addresses) != 1 || got.AgentAccounts["codex"] != "primary" || got.KeyBalanceAuth["primary"].User != "42" {
		t.Fatal("saved provider choices lost")
	}
}

func TestPrivacyToggleChangesSavedRuntimeChoice(t *testing.T) {
	f, h, p, _ := setup(t)
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).FillWidth().Children(func() { p.homeView(c) }) }, 900, 1600)
	settle(h, tt)
	shot(t, tt, "ai-updated-common-options")
	if err := tt.Click(i18n.T("aiUpdate.hideText")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	p.save()
	settle(h, tt)
	saved := f.lastSave()
	if saved == nil {
		t.Fatal("save missing")
	}
	raw, _ := json.Marshal(saved["configuration"])
	var reopened configuration
	if err := json.Unmarshal(raw, &reopened); err != nil {
		t.Fatal(err)
	}
	if !reopened.settings().HideRequestText {
		t.Fatal("privacy choice lost")
	}
}

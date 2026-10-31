package aiservices

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
)

// Exercise the real parent page, not only usage's isolated view: a package
// can pass every unit test while a build tag still hides it from the product.
func TestUsageIsMountedAndAliasesSaveThroughThePage(t *testing.T) {
	f, h, p, tt := setup(t)
	if err := tt.Click(i18n.T("usage.title")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.tab != tabUsage || p.usage == nil || !tt.HasText(i18n.T("usage.overview")) {
		t.Fatal("usage was not mounted")
	}
	if len(h.Called("capture.ai.usage.get")) == 0 {
		t.Fatal("page displayed without reading the ledger")
	}
	if err := tt.Click(i18n.T("usage.aliases")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	fill := func(label, text string) {
		t.Helper()
		// ui.Field focuses its control when the visible label is clicked.
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type(text)
	}
	fill(i18n.T("usage.aliasFrom"), "model-a")
	fill(i18n.T("usage.aliasTo"), "display-a")
	if err := tt.Click(i18n.T("usage.aliasApply")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.draft.Value.magpie().UsageAliases["model-a"] != "display-a" || !p.dirty() || len(f.saves) != 0 {
		t.Fatal("alias did not stay in the page's unsaved draft")
	}
	if h.Env.Rail.Save.Disabled {
		t.Fatal("page Save is disabled for edited usage")
	}
	h.Env.Rail.Save.Run()
	settle(h, tt)
	saved := f.lastSave()["configuration"].(map[string]any)["magpieSettings"].(map[string]any)
	if saved["usageAliases"].(map[string]any)["model-a"] != "display-a" || saved["futureMagpie"] != 7.0 || p.dirty() {
		t.Fatal("usage save lost aliases or unrelated fields", saved)
	}
	p.draft.Value.settings().HideAccounts = !p.draft.Value.magpie().HideAccounts
	settle(h, tt)
	if h.Env.Rail.Undo.Disabled {
		t.Fatal("usage Undo is disabled")
	}
	h.Env.Rail.Undo.Run()
	settle(h, tt)
	if p.dirty() || p.tab != tabUsage {
		t.Fatal("undo lost the selected usage tab or retained its changes")
	}
}

func TestTopLevelTabAppliesAfterTheUnsavedGuardAnswers(t *testing.T) {
	_, h, p, tt := setup(t)
	p.change("one", func(pr *provider) { pr.Name = "Draft name" })
	var apply func()
	h.Env.RequestChange = func(fn func()) { apply = fn }
	if err := tt.Click(i18n.T("gateway.title")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if apply == nil || p.tab != tabCustom || p.draft.Value.find("one").Name != "Draft name" {
		t.Fatal("guard did not preserve the page and draft")
	}
	// The shell discards drafts before applying a confirmed transition.
	p.load()
	settle(h, tt)
	apply()
	settle(h, tt)
	if p.tab != tabGateway || p.dirty() {
		t.Fatal("confirmed tab change was discarded after the frame")
	}
}

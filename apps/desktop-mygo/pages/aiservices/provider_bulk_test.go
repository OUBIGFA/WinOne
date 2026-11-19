package aiservices

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/widgets"
)

func TestBulkProviderRemovalKeepsOthersAndSavesOnce(t *testing.T) {
	f, h, p, tt := setup(t)
	first, second := p.add(nil), p.add(nil)
	p.change(first, func(pr *provider) { pr.Name = "Remove one" })
	p.change(second, func(pr *provider) { pr.Name = "Remove two" })
	p.draft.Value.Tasks["default"] = widgets.AISelection{Provider: first, Model: "temporary-model"}
	p.keys[first] = map[string]string{"primary": "fixture-removed"}
	p.keys["one"] = map[string]string{"primary": "fixture-kept"}
	p.preferenceFields = map[string]*preferenceField{first + ":extraBody": {invalid: true}}
	beforeDocument := clone(*p.draft)
	beforeKeep := beforeDocument.Value.find("one")
	settle(h, tt)
	if err := tt.Click(i18n.T("aiBulkProviders.open")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	for _, name := range []string{"Remove one", "Remove two"} {
		if err := tt.Click(i18n.T("aiBulkProviders.pick", "name", name)); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
	}
	if len(f.saves) != 0 {
		t.Fatal("selecting providers saved configuration")
	}
	if err := tt.Click(i18n.T("aiBulkProviders.remove")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(p.draft.Value.Providers) != 1 || !equal(beforeKeep, p.draft.Value.find("one")) {
		t.Fatal("bulk removal changed an unselected provider")
	}
	if p.draft.Value.Tasks["default"].Provider != "" || p.draft.Value.Tasks["ocr"].Provider != "one" {
		t.Fatalf("bulk removal cleared the wrong tasks: %+v", p.draft.Value.Tasks)
	}
	if _, ok := p.keys[first]; ok || p.keys["one"]["primary"] != "fixture-kept" || p.preferencesInvalid() {
		t.Fatal("removed-provider key or unfinished options survived, or another provider's draft was lost")
	}
	if len(f.saves) != 0 {
		t.Fatal("bulk removal bypassed the existing save flow")
	}
	p.save()
	settle(h, tt)
	if len(f.saves) != 1 || p.err != "" {
		t.Fatalf("bulk removal did not save in one transaction: saves=%d err=%q", len(f.saves), p.err)
	}
	p.load()
	settle(h, tt)
	if len(p.draft.Value.Providers) != 1 || p.draft.Value.find("one") == nil || p.draft.Value.Tasks["default"].Provider != "" {
		t.Fatal("bulk removal did not survive rereading the saved configuration")
	}
}

func TestBulkProviderCancelKeepsDraftKeysAndTasks(t *testing.T) {
	f, h, p, tt := setup(t)
	added := p.add(nil)
	p.change(added, func(pr *provider) { pr.Name = "Pending provider" })
	p.keys[added] = map[string]string{"primary": "fixture-pending"}
	before := clone(*p.draft)
	settle(h, tt)
	for _, label := range []string{i18n.T("aiBulkProviders.open"), i18n.T("aiBulkProviders.all"), i18n.T("common.cancel")} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
	}
	if !equal(before, *p.draft) || p.keys[added]["primary"] != "fixture-pending" || len(f.saves) != 0 {
		t.Fatal("cancelling bulk selection changed the draft, keys or save count")
	}
}

func TestBulkProviderSearchAndSelectionStayWithinTheirTab(t *testing.T) {
	_, h, p, tt := setup(t)
	custom := p.add(nil)
	p.change(custom, func(pr *provider) { pr.Name = "Custom second" })
	account := p.add(&agent{ID: "codex", Name: "Account one", Icon: "openai"})
	p.switchTab(tabCustom)
	settle(h, tt)
	for _, label := range []string{i18n.T("aiBulkProviders.open"), i18n.T("aiBulkProviders.search")} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
	}
	tt.Type("Custom second")
	settle(h, tt)
	if _, visible := tt.AccessByRole(ui.RoleCheckBox, i18n.T("aiBulkProviders.pick", "name", "Relay")); visible {
		t.Fatal("provider search did not hide unrelated rows")
	}
	if err := tt.Click(i18n.T("aiBulkProviders.all")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if err := tt.Click(i18n.T("common.cancel")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	p.switchTab(tabAccounts)
	settle(h, tt)
	if err := tt.Click(i18n.T("aiBulkProviders.open")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	check, ok := tt.AccessByRole(ui.RoleCheckBox, i18n.T("aiBulkProviders.pick", "name", "Account one"))
	if !ok || check.Checked {
		t.Fatalf("custom selection leaked into the account tab: %+v", check)
	}
	if _, visible := tt.AccessByRole(ui.RoleCheckBox, i18n.T("aiBulkProviders.pick", "name", "Relay")); visible {
		t.Fatal("account bulk list included a custom provider")
	}
	if err := tt.Click(i18n.T("common.cancel")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	p.switchTab(tabCustom)
	settle(h, tt)
	if err := tt.Click(i18n.T("aiBulkProviders.open")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if err := tt.Click(i18n.T("aiBulkProviders.search")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	settle(h, tt)
	for _, name := range []string{"Relay", "Custom second"} {
		check, ok := tt.AccessByRole(ui.RoleCheckBox, i18n.T("aiBulkProviders.pick", "name", name))
		if !ok || !check.Checked {
			t.Fatalf("select-all omitted a search-hidden provider %q: %+v", name, check)
		}
	}
	if err := tt.Click(i18n.T("aiBulkProviders.none")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	for _, name := range []string{"Relay", "Custom second"} {
		check, _ := tt.AccessByRole(ui.RoleCheckBox, i18n.T("aiBulkProviders.pick", "name", name))
		if check.Checked {
			t.Fatalf("select-none did not clear %q", name)
		}
	}
	if p.draft.Value.find(account) == nil {
		t.Fatal("bulk selection modified the account provider")
	}
}

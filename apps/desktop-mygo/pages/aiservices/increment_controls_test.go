package aiservices

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
)

func TestCodexClientNativeChoiceSavesAndReopens(t *testing.T) {
	f, h, p, _ := setup(t)
	pr := p.current()
	pr.AuthMode, pr.Agent = "account", "codex"
	e := p.editor
	e.optionsOpen, e.preferenceTab = true, 1
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).FillWidth().Children(func() { e.View(c, pr, pr) })
	}, 900, 1600)
	settle(h, tester)
	for _, label := range []string{"Codex CLI", i18n.T("aiSync.codexDesktop")} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		settle(h, tester)
	}
	p.save()
	settle(h, tester)
	saved := f.lastSave()
	if saved == nil {
		t.Fatal("Codex client choice did not save")
	}
	settings := saved["configuration"].(map[string]any)["magpieSettings"].(map[string]any)
	if settings["codexClient"] != "desktop" {
		t.Fatalf("the selected client was not persisted: %+v", settings)
	}
	raw, _ := json.Marshal(p.draft.Value.settings())
	var reopened map[string]any
	_ = json.Unmarshal(raw, &reopened)
	if reopened["codexClient"] != "desktop" {
		t.Fatal("the selected Codex client was lost when the saved document was reread")
	}
}

func TestGatewayBareModelModesSaveAndReopen(t *testing.T) {
	f, h, p, _ := setup(t)
	g := p.gateway
	g.networkOpen = true
	tester := ui.NewTester(func(c *ui.Context) { g.View(c, nil, p) }, 900, 2000)
	settle(h, tester)
	for _, mode := range []struct {
		label    string
		on, only bool
	}{{"aiSync.bareAlongside", true, false}, {"aiSync.bareOnly", true, true}, {"aiSync.bareOff", false, false}} {
		if err := tester.Click(i18n.T(mode.label)); err != nil {
			t.Fatal(err)
		}
		settle(h, tester)
		g.save(false)
		settle(h, tester)
		if len(f.gatewaySaves) == 0 {
			t.Fatal("gateway model ID mode did not save")
		}
		saved := f.gatewaySaves[len(f.gatewaySaves)-1]["settings"].(map[string]any)
		if saved["bareModelIDs"] != mode.on || saved["bareModelIDsOnly"] != mode.only {
			t.Fatalf("%s saved a different model listing mode: %+v", mode.label, saved)
		}
		g.load()
		settle(h, tester)
		raw, _ := json.Marshal(g.saved)
		var reopened map[string]any
		_ = json.Unmarshal(raw, &reopened)
		if reopened["bareModelIDs"] != mode.on || reopened["bareModelIDsOnly"] != mode.only {
			t.Fatalf("%s did not survive reopening", mode.label)
		}
	}
}

func TestAccountWarmupNativeActionKeepsPerAccountResults(t *testing.T) {
	f, h, p, _ := setup(t)
	pr, old := p.current(), p.saved.Value.find("one")
	pr.AuthMode, pr.Agent = "account", "codex"
	old.AuthMode, old.Agent = "account", "codex"
	f.accounts = []map[string]any{{"agent": "codex", "user": "ready@example.invalid"}}
	a := newAccountPanel(p, pr)
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).FillWidth().Children(func() { a.maintenanceView(c, pr, false) })
	}, 900, 1800)
	settle(h, tester)
	if err := tester.Click(i18n.T("aiSync.warmNow")); err != nil {
		t.Fatal(err)
	}
	settle(h, tester)
	if len(f.warmups) != 1 || f.warmups[0]["provider"] != "one" || f.warmups[0]["agent"] != "codex" {
		t.Fatalf("warm-up lost its saved account provider: %+v", f.warmups)
	}
	for _, result := range []string{i18n.T("aiSync.warmed"), i18n.T("aiSync.warmRunning"), i18n.T("aiSync.warmOff"), "Warm-up could not be completed"} {
		if !strings.Contains(strings.Join(tester.Texts(), "\n"), result) {
			t.Fatalf("warm-up omitted a per-account outcome %q: %v", result, tester.Texts())
		}
	}
	if len(f.saves) != 0 {
		t.Fatal("manual warm-up changed provider preferences")
	}
}

func TestInlineProxyNativeTestUsesDraftAndShowsExit(t *testing.T) {
	f, h, p, _ := setup(t)
	e := p.editor
	proxy := "http://user:fixture-password@127.0.0.1:7890"
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).FillWidth().Children(func() { e.proxyPreference(c, "provider", &proxy, false) })
	}, 900, 700)
	settle(h, tester)
	if err := tester.Click(i18n.T("aiSync.proxyTest")); err != nil {
		t.Fatal(err)
	}
	settle(h, tester)
	if len(f.proxyChecks) != 1 || f.proxyChecks[0]["proxy"] != proxy {
		t.Fatalf("proxy test did not use the unsaved address: %+v", f.proxyChecks)
	}
	text := strings.Join(tester.Texts(), "\n")
	for _, value := range []string{"203.0.113.9", "US", "210"} {
		if !strings.Contains(text, value) {
			t.Fatalf("proxy result omitted %q: %s", value, text)
		}
	}
	if len(f.saves) != 0 || proxy != "http://user:fixture-password@127.0.0.1:7890" {
		t.Fatal("proxy probe changed the form or saved preferences")
	}
}

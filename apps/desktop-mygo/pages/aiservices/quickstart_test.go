package aiservices

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	"winone/desktopmygo/widgets"
)

// emptyAI is the Host of a first run: no providers and no task choices.
func emptyAI(t *testing.T) (*fakeAI, *testhost.Harness, *page, *ui.Tester) {
	f := newFakeAI(t)
	f.configuration["providers"] = []any{}
	f.configuration["tasks"] = map[string]any{}
	h := testhost.New(t, f.handle)
	p := newPage(h.Env)
	t.Cleanup(p.Close)
	tt := ui.NewTester(p.View, 1000, 1400)
	settle(h, tt)
	return f, h, p, tt
}

// A first run starts on Home with three steps; each step's action opens
// the existing place that does it, and the guide leaves once all are done.
func TestQuickStartGuidesAFirstSetup(t *testing.T) {
	_, h, p, tt := emptyAI(t)
	if p.tab != tabHome || !tt.HasText(homeT("startTitle")) || !tt.HasText(homeT("stepProvider")) {
		t.Fatalf("Home shows no first-run guide: %q", tt.Texts())
	}
	if err := tt.Click(i18n.T("ai.addPreset")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.tab != tabCustom || !p.choosingPreset {
		t.Fatalf("the first step did not open the presets on the custom tab: tab=%d choosing=%v", p.tab, p.choosingPreset)
	}
	p.choosingPreset = false
	id := p.add(nil)
	p.change(id, func(pr *provider) {
		pr.Name, pr.Models, pr.AvailableModels = "Relay", []string{"gpt-a"}, &[]string{"gpt-a"}
	})
	p.switchTab(tabHome)
	settle(h, tt)
	if !tt.HasText(homeT("startTitle")) || !tt.HasText(homeT("stepDefault")) {
		t.Fatal("the guide left before the everyday model was chosen")
	}
	p.draft.Value.Tasks["default"] = widgets.AISelection{Provider: id, Model: "gpt-a"}
	settle(h, tt)
	if tt.HasText(homeT("startTitle")) {
		t.Fatal("the guide stays after every step is done")
	}
}

// An empty provider tab is an empty state whose actions add the first
// provider, instead of a toolbar over a sentence.
func TestEmptyProviderTabsTeachTheFirstStep(t *testing.T) {
	_, h, p, tt := emptyAI(t)
	p.switchTab(tabCustom)
	settle(h, tt)
	if !tt.HasText(homeT("emptyCustom")) || !tt.HasText(homeT("addManually")) {
		t.Fatalf("custom tab has no empty state: %q", tt.Texts())
	}
	if err := tt.Click(homeT("addManually")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(p.sectionProviders()) != 1 || tt.HasText(homeT("emptyCustom")) {
		t.Fatal("adding manually did not open a new provider")
	}
	p.switchTab(tabAccounts)
	settle(h, tt)
	if !tt.HasText(homeT("emptyAccounts")) {
		t.Fatalf("accounts tab has no empty state: %q", tt.Texts())
	}
}

func TestFirstRunShots(t *testing.T) {
	_, h, p, _ := emptyAI(t)
	for i, tab := range []int{tabHome, tabCustom, tabAccounts} {
		p.switchTab(tab)
		h.Shots(t, fmt.Sprintf("aiservices-first-%d", i), p.View, 1000, nil)
	}
}

func TestQuickStartDoesNotTreatAnUnavailableDefaultAsComplete(t *testing.T) {
	_, h, p, tt := setup(t)
	p.switchTab(tabHome)
	p.draft.Value.Tasks["default"] = widgets.AISelection{Provider: "one", Model: "removed-model"}
	settle(h, tt)
	if !tt.HasText(homeT("startTitle")) {
		t.Fatal("the guide disappeared with a missing default model")
	}
	p.draft.Value.Tasks["default"] = widgets.AISelection{Provider: "one", Model: "gpt-a"}
	p.draft.Value.find("one").Off = true
	settle(h, tt)
	if !tt.HasText(homeT("stepDefault")) {
		t.Fatal("a disabled default provider was treated as ready")
	}
}

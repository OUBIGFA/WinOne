package aiservices

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

func TestModelSettingsHoverShowsOneSpecificTooltip(t *testing.T) {
	_, h, p, _ := setup(t)
	pr := p.draft.Value.find("one")
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).FillWidth().Padding(60, 16).Children(func() {
			p.editor.models.list(c, pr, pr.available(), false, false, true)
		})
	}, 720, 620)
	settle(h, tt)
	label := i18n.T("ai.editModelSettings", "name", "gpt-a")
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing model settings action %q", label)
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(time.Second)
	count := 0
	for _, text := range tt.Texts() {
		if text == label || text == i18n.T("ai.modelSettings") {
			count++
		}
	}
	// Texts also includes the button's accessible label: one label and one tip.
	if count != 2 {
		t.Fatalf("expected one specific tooltip, got %d: %q", count, tt.Texts())
	}
}

func TestRoutingListKeepsAllMembersWithoutOversizedRows(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	r.state = &routeGroups{}
	for i := range 4 {
		r.state.Groups = append(r.state.Groups, core.Preserved[routeGroup]{Value: routeGroup{ID: fmt.Sprint(i), Name: fmt.Sprintf("Group %d", i), Ready: true, Members: []string{"primary/model-a", "backup/model-b"}}})
	}
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).FillWidth().Gap(24).Children(func() { r.groupsView(c) }) }, 1100, 1800)
	settle(h, tt)
	a, _ := tt.Find("Group 0")
	b, _ := tt.Find("Group 1")
	if b.Y-a.Y > 112 {
		t.Fatalf("two-member route rows waste vertical space: %.1f DIP", b.Y-a.Y)
	}
	for _, member := range []string{"model-a", "model-b"} {
		if !tt.HasText(member) {
			t.Fatalf("missing member %s", member)
		}
	}
}

func TestModelPriceEditorAddsAndRemovesEveryTier(t *testing.T) {
	_, h, p, tt := setup(t)
	m := p.editor.models
	m.editing = "gpt-a"
	m.settingsTab = 3
	settle(h, tt)
	for range 2 {
		if err := tt.Click(i18n.T("aiComplete.addPriceTier")); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
	}
	price := p.draft.Value.find("one").ModelMetadata["gpt-a"].Value.Price
	if price == nil || len(price.Value.Tiers) != 2 {
		t.Fatal("price tiers were not added")
	}
	if err := tt.Click(i18n.T("aiComplete.removePriceTier", "number", 1)); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	price = p.draft.Value.find("one").ModelMetadata["gpt-a"].Value.Price
	if len(price.Value.Tiers) != 1 {
		t.Fatal("removed tier did not leave the other tier intact")
	}
}

func TestClaudeRunModeIsAvailableInItsProviderAndStaysUnsaved(t *testing.T) {
	f, h, p, _ := setup(t)
	pr := p.draft.Value.find("one")
	pr.AuthMode = "account"
	pr.Agent = "claude"
	e := p.editor
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).FillWidth().Children(func() { e.View(c, pr, pr) }) }, 900, 1800)
	settle(h, tt)
	if err := tt.Click("claude -p"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !p.draft.Value.settings().ClaudePrintMode || len(f.saves) > 0 {
		t.Fatal("mode did not stay in the shared unsaved settings")
	}
}

func TestModelListEmptySpaceDoesNotClaimToResize(t *testing.T) {
	_, h, p, _ := setup(t)
	pr := p.draft.Value.find("one")
	tt := ui.NewTester(func(c *ui.Context) { p.editor.models.list(c, pr, pr.available(), false, false, true) }, 720, 620)
	settle(h, tt)
	tt.Move(300, 300)
	tt.Advance(time.Second)
	count := 0
	for _, text := range tt.Texts() {
		if text == i18n.T("ai.resizeModelList") {
			count++
		}
	}
	if count != 1 {
		t.Fatal("list body shows the resize handle's tooltip")
	}
}

func TestRoutingDefaultControlsUpdateTheSharedDraft(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	r.state = &routeGroups{Groups: []core.Preserved[routeGroup]{{Value: routeGroup{ID: "fixture", Name: "Fixture", Ready: true}}}}
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).FillWidth().Gap(16).Children(func() { r.groupsView(c) }) }, 1100, 2000)
	settle(h, tt)
	if err := tt.Click(nativeT("groupDefaults")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	settle(h, tt)
	if err := tt.Click(nativeT("sink")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !p.draft.Value.settings().GroupSink {
		t.Fatal("routing default edited a copy instead of the saved draft")
	}
}

func TestCompleteModelSettingsShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("explicit visual inspection")
	}
	_, h, p, _ := setup(t)
	pr := p.draft.Value.find("one")
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for tab := 0; tab < 4; tab++ {
			p.editor.models.settingsTab = tab
			p.editor.models.editing = "gpt-a"
			h.Shots(t, fmt.Sprintf("model-%s-%d", locale, tab), func(c *ui.Context) { p.editor.models.settings(c, pr, false) }, 900, nil)
		}
	}
	i18n.SetLocale(i18n.Chinese)
}

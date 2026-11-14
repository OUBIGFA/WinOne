package aiservices

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func TestProviderOverviewKeepsCountsAndQuickNavigation(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	f := newFakeAI(t)
	f.configuration["providers"] = []any{
		map[string]any{"id": "openai", "name": "OpenAI", "icon": "openai", "authMode": "api-key", "models": []any{"gpt-4.1", "gpt-4.1-mini"}},
		map[string]any{"id": "anthropic", "name": "Anthropic", "icon": "anthropic", "authMode": "api-key", "models": []any{"claude-sonnet-4-5"}},
		map[string]any{"id": "gemini", "name": "Gemini", "icon": "gemini", "authMode": "account", "models": []any{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite"}},
		map[string]any{"id": "disabled", "name": "Disabled service", "authMode": "api-key", "off": true, "models": []any{"disabled-model"}},
		map[string]any{"id": "empty", "name": "Empty service", "authMode": "api-key", "models": []any{}},
	}
	h := testhost.New(t, f.handle)
	p := newPage(h.Env)
	t.Cleanup(p.Close)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, width := range []int{420, 560, 880} {
			for _, dark := range []bool{false, true} {
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(kit.Theme(c.Preferences(), dark))
					p.overview(c)
				}, width, 240)
				settle(h, tt)
				if !tt.HasText(i18n.T("ai.providerTotal", "count", 3, "models", 6)) || tt.HasText("Disabled service") || tt.HasText("Empty service") {
					t.Fatalf("preview counts or visibility differ: %q", tt.Texts())
				}
				for _, entry := range []struct {
					id, name   string
					count, tab int
				}{{"openai", "OpenAI", 2, tabCustom}, {"anthropic", "Anthropic", 1, tabCustom}, {"gemini", "Gemini", 3, tabAccounts}} {
					label := i18n.T("ai.providerCount", "name", entry.name, "count", entry.count)
					button, ok := tt.Access(label)
					if !ok || !button.Focusable || button.Bounds.X < 0 || button.Bounds.X+button.Bounds.W > float32(width) {
						t.Fatalf("preview entry is inaccessible at %d: %+v", width, button)
					}
					if err := tt.Click(label); err != nil {
						t.Fatal(err)
					}
					if p.selected != entry.id || p.tab != entry.tab || !p.focusModels {
						t.Fatalf("%s did not locate its models: selected=%s tab=%d", entry.name, p.selected, p.tab)
					}
					p.selected, p.focusModels = "", false
					tt.Key(0, ui.KeyEnter)
					if p.selected != entry.id || p.tab != entry.tab || !p.focusModels {
						t.Fatalf("keyboard activation did not locate %s", entry.name)
					}
				}
				bounds, _ := tt.Find(i18n.T("ai.providerOverview"))
				t.Logf("preview %s dark=%t width=%d height=%.1f", locale, dark, width, bounds.H)
				tt.Move(float32(width-1), 239)
				shot(t, tt, fmt.Sprintf("provider-preview-%s-%t-%d", locale, dark, width))
			}
		}
	}
	// A removed provider remains visible but cannot navigate before saving.
	p.remove("openai")
	tt := ui.NewTester(p.overview, 880, 240)
	settle(h, tt)
	removed, ok := tt.Access(i18n.T("ai.providerCount", "name", "OpenAI", "count", 2))
	if !ok || !removed.Disabled {
		t.Fatalf("removed provider is no longer a disabled preview: %+v", removed)
	}
	longName := "Anthropic · Work and research with a long provider name"
	p.draft.Value.find("anthropic").Name = longName
	tt = ui.NewTester(p.overview, 420, 400)
	tt.SetPreferences(ui.Preferences{TextScale: 1.5})
	entry, ok := tt.Access(i18n.T("ai.providerCount", "name", longName, "count", 1))
	if !ok || entry.Bounds.X < 0 || entry.Bounds.X+entry.Bounds.W > 420 || entry.Description != longName {
		t.Fatalf("long provider name lost its bounds or full tooltip: %+v", entry)
	}
	shot(t, tt, "provider-preview-long-name-scaled")
}

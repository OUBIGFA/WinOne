package aiservices

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// All five retained tasks stay on the first level.
func TestAINavigationKeepsFiveTasks(t *testing.T) {
	_, h, p, _ := setup(t)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		p.switchTab(tabHome)
		tt := ui.NewTester(p.upstreamNavigation, 880, 420)
		settle(h, tt)
		for _, label := range []string{homeT("home"), i18n.T("ai.customProviders"), i18n.T("ai.addProvidersTab"), homeT("preferences"), i18n.T("gateway.title")} {
			if !tt.HasText(label) {
				t.Errorf("primary navigation is missing %s", label)
			}
		}
		for _, label := range []string{"Sessions", "Library", "Routing", "Routing groups", "Usage", "Plugins", "Masking"} {
			if tt.HasText(upstreamT(label)) {
				t.Errorf("secondary or removed entry on primary navigation: %s", label)
			}
		}
		if err := tt.Click(i18n.T("gateway.title")); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
		if p.tab != tabGateway {
			t.Fatal("Gateway did not open")
		}
	}
	i18n.SetLocale(i18n.Chinese)
}

func TestAIPageUsesSharedSettingsWidth(t *testing.T) {
	_, h, p, _ := setup(t)
	p.switchTab(tabHome)
	for _, width := range []int{560, 1400} {
		tt := ui.NewTester(p.View, width, 1200)
		settle(h, tt)
		first, ok := tt.Find(homeT("home"))
		if !ok {
			t.Fatal("missing navigation")
		}
		want := float32(0)
		if width > kit.PageWidth {
			want = float32(width-kit.PageWidth) / 2
		}
		if first.X < want || first.X > want+20 {
			t.Fatalf("width %d: navigation starts at %v, shared column at %v", width, first.X, want)
		}
	}
}

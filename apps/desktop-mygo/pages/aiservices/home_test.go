package aiservices

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"os"
	"testing"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
)

func TestAIHomeKeepsCommonTasksAndAllSecondaryPagesReachable(t *testing.T) {
	f := newFakeAI(t)
	h := testhost.New(t, f.handle)
	p := newPage(h.Env)
	t.Cleanup(p.Close)
	tt := ui.NewTester(p.View, 560, 1000)
	settle(h, tt)
	if p.tab != tabHome || !tt.HasText(homeT("connect")) || tt.HasText(i18n.T("gateway.title")) {
		t.Fatal("home exposes specialist configuration or omits connection task")
	}
	for _, entry := range moreDestinations() {
		p.switchTab(tabMore)
		settle(h, tt)
		if err := tt.Click(entry.label); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
		if p.tab != entry.tab {
			t.Fatalf("could not open %s", entry.label)
		}
	}
}
func TestAIHomeAndMoreScreens(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("explicit visual inspection")
	}
	_, h, p, _ := setup(t)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, tab := range []int{tabHome, tabMore, tabCustom, tabGateway} {
			p.switchTab(tab)
			h.Shots(t, fmt.Sprintf("ai-area-%s-%d", locale, tab), p.View, 1100, nil)
		}
	}
}

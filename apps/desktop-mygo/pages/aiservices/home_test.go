package aiservices

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"os"
	"testing"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
)

func TestAIHomeKeepsOnlyRetainedTasksReachable(t *testing.T) {
	f := newFakeAI(t)
	h := testhost.New(t, f.handle)
	p := newPage(h.Env)
	t.Cleanup(p.Close)
	tt := ui.NewTester(p.View, 560, 1000)
	settle(h, tt)
	if p.tab != tabHome || !tt.HasText(homeT("default")) {
		t.Fatal("home omits the everyday model")
	}
	for _, removed := range []string{"连接 AI 服务", "添加或管理服务", "登录或管理账号", "Connect an AI service", "Add or manage services", "Sign in or manage accounts"} {
		if tt.HasText(removed) {
			t.Fatalf("Home still shows the removed connection shortcut %q", removed)
		}
	}
	// Result preferences belong to Home and must still commit through the
	// shared draft after the separate preferences tab has been removed.
	for _, key := range []string{"ai.autoRetranslate", "ai.showReasoning"} {
		if err := tt.Click(i18n.T(key)); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
	}
	p.save()
	settle(h, tt)
	configuration := f.lastSave()["configuration"].(map[string]any)
	if configuration["autoRetranslate"] != false || configuration["showReasoning"] != false {
		t.Fatalf("Home did not save result preferences: %+v", configuration)
	}
	p.load()
	settle(h, tt)
	if p.draft.Value.AutoRetranslate == nil || *p.draft.Value.AutoRetranslate || p.draft.Value.ShowReasoning == nil || *p.draft.Value.ShowReasoning || p.dirty() {
		t.Fatal("reopening Home lost the saved result preferences")
	}
	for _, entry := range []struct {
		label string
		tab   int
	}{{i18n.T("ai.customProviders"), tabCustom}, {i18n.T("ai.addProvidersTab"), tabAccounts}, {i18n.T("gateway.title"), tabGateway}} {
		p.switchTab(tabHome)
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
func TestAIRetainedScreens(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("explicit visual inspection")
	}
	_, h, p, _ := setup(t)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for _, tab := range []int{tabHome, tabCustom, tabAccounts, tabGateway} {
			p.switchTab(tab)
			h.Shots(t, fmt.Sprintf("ai-area-%s-%d", locale, tab), p.View, 1100, nil)
		}
	}
}

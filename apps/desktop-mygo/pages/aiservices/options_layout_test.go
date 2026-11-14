package aiservices

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/internal/ai"
)

func TestProviderOptionsUseNamedTaskSections(t *testing.T) {
	_, h, p, _ := setup(t)
	pr := p.current()
	e := newEditor(p, pr, pr.ID)
	tt := ui.NewTester(func(c *ui.Context) { e.preferences(c, pr, false) }, 800, 1200)
	settle(h, tt)
	for _, key := range []string{"aiOptions.requests", "aiOptions.accounts", "aiOptions.billing", "aiOptions.compatibility"} {
		if _, ok := tt.Find(i18n.T(key)); !ok {
			t.Errorf("missing task section %s", key)
		}
	}
}

// Exercise every rebuilt section at actual desktop and narrow content widths.
// The same production views supply screenshots and translated text bounds.
func TestRebuiltAISectionLayouts(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, dark := range []bool{false, true} {
			for _, width := range []float32{560, 1000} {
				_, h, p, _ := setup(t)
				i18n.SetLocale(locale)
				pr := p.current()
				pr.RateWindows = []ai.RateWindow{{Start: "09:00", End: "18:00", Rate: 0.8, Days: []int{1, 2, 3, 4, 5}}}
				pr.AccountNames = map[string]string{"primary": "Daily service"}
				pr.AccountWeights = map[string]int{"primary": 2}
				pr.Resolve = map[string][]string{"api.example.com": {"127.0.0.1", "dns:gateway.example.com"}}
				pr.AccountWindowAmounts = map[string]map[string]float64{"primary": {"5h": 3.5}}
				pr.KeyPriceRates = map[string]float64{"primary": 0.8}
				e := newEditor(p, pr, pr.ID)
				for _, area := range []struct {
					name  string
					count int
					view  func(*ui.Context, int)
				}{
					{"provider", 6, func(c *ui.Context, n int) { e.preferenceTab = n; e.preferences(c, pr, false) }},
				} {
					for n := 0; n < area.count; n++ {
						tt := ui.NewTester(func(c *ui.Context) {
							c.SetTheme(kit.Theme(c.Preferences(), dark))
							ui.Column(c).FillWidth().Gap(20).Padding(20).Children(func() { area.view(c, n) })
						}, int(width), 1600)
						settle(h, tt)
						for _, label := range tt.Texts() {
							if b, ok := tt.Find(label); ok && (b.X < -.5 || b.X+b.W > width+.5) {
								t.Errorf("%s/%d %s dark=%t width=%v: %q outside %+v", area.name, n, locale, dark, width, label, b)
							}
						}
						shot(t, tt, fmt.Sprintf("rebuilt-%s-%d-%s-%t-%.0f", area.name, n, locale, dark, width))
					}
				}
			}
		}
	}
}

func TestProviderSectionsKeepPoliciesOnSaveAndReopen(t *testing.T) {
	_, h, p, tt := setup(t)
	pr := p.current()
	e := newEditor(p, pr, pr.ID)
	pr.AccountWeights = map[string]int{"primary": 3, "offline": 7}
	pr.AccountWindowCaps = map[string]map[string]int{"primary": {"5h": 80}}
	pr.AccountWindowAmounts = map[string]map[string]float64{"offline": {"week": 6.5}}
	pr.KeyPriceRates = map[string]float64{"primary": 0.8}
	pr.Resolve = map[string][]string{"api.example.com": {"127.0.0.1"}}
	pr.ServerPin = "fixture-certificate-pin"
	pr.NoStandIn, pr.UpstreamProviders, pr.NoBalanceProbe = true, true, true
	pr.RateWindows = []ai.RateWindow{{Start: "22:00", End: "06:00", Rate: .5, Days: []int{1, 5}}}
	pr.Proxy = "direct"
	before, _ := json.Marshal(pr.ProviderPreferences)
	for n := 0; n < 6; n++ {
		e.preferenceTab = n
		view := ui.NewTester(func(c *ui.Context) { e.preferences(c, pr, false) }, 1000, 2400)
		settle(h, view)
	}
	after, _ := json.Marshal(pr.ProviderPreferences)
	if string(before) != string(after) {
		t.Fatalf("reading sections changed preferences: before=%s after=%s", before, after)
	}
	p.save()
	settle(h, tt)
	reopened, _ := json.Marshal(p.saved.Value.find(pr.ID).ProviderPreferences)
	if string(reopened) != string(before) {
		t.Fatalf("save lost provider policies: %s", reopened)
	}
}

func TestProviderProxyCanChangeFromDirectToCustom(t *testing.T) {
	_, h, p, _ := setup(t)
	pr := p.current()
	pr.Proxy = "direct"
	e := newEditor(p, pr, pr.ID)
	tt := ui.NewTester(func(c *ui.Context) { e.proxyPreference(c, "provider", &pr.Proxy, false) }, 800, 600)
	settle(h, tt)
	if err := tt.Click(i18n.T("ai.providerProxy")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(i18n.T("aiOptions.customProxy")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if err := tt.Click(i18n.T("aiOptions.proxyAddress")); err != nil {
		t.Fatal(err)
	}
	tt.Type("http://127.0.0.1:7890")
	settle(h, tt)
	if pr.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("custom proxy not retained: %q", pr.Proxy)
	}
}

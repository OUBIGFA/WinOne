package aiservices

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
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
				e := newEditor(p, pr, pr.ID)
				a := newAdvancedPanel(p)
				p.routing.state = &routeGroups{Groups: []core.Preserved[routeGroup]{{Value: routeGroup{ID: "daily", Name: "Daily assistant", Ref: "group/daily", Members: []string{"fixture/model"}, Ready: true, Rules: []core.Preserved[routeRule]{{Value: routeRule{Use: "fixture/model", Intent: "coding"}}}}}}}
				p.routing.begin(p.routing.state.Groups[0])
				var request traceRoute
				json.Unmarshal([]byte(`{"id":7,"time":"2026-11-07T12:00:00Z","model":"group/daily","agent":"Chat","session":"daily","sessionTitle":"Daily assistant","done":true,"status":200,"ms":6000,"ttft":1000,"tokens":2400,"out":400,"priced":true,"cost":0.003,"order":[{"id":"fixture/primary","name":"Fixture","who":"person@example.invalid","model":"fixture/model","known":true,"used":0.3}],"tries":[{"id":"fixture/primary","model":"fixture/model","start":"2026-11-07T12:00:00Z","done":true,"status":200,"ms":6000}],"usage":[{"in":1000,"cache_read":1000,"out":400}]}`), &request)
				p.routing.trace.routes = []traceRoute{request}
				p.routing.trace.loaded, p.routing.trace.paused, p.routing.trace.daysLoaded = true, true, true
				for _, area := range []struct {
					name  string
					count int
					view  func(*ui.Context, int)
				}{
					{"provider", 4, func(c *ui.Context, n int) { e.preferenceTab = n; e.preferences(c, pr, false) }},
					{"privacy", 4, func(c *ui.Context, n int) { a.privacyTab = n; a.privacy(c, &magpieCatalog{}, false) }},
					{"group", 5, func(c *ui.Context, n int) { p.routing.editorTab = n; p.routing.editor(c) }},
					{"trace", 3, func(c *ui.Context, n int) {
						tr := &p.routing.trace
						if n == 0 {
							tr.View(c, p)
						}
						if n == 1 {
							tr.filtersOpen = true
							tr.metricsOpen = true
							tr.View(c, p)
						}
						if n == 2 {
							tr.evidenceView(c, p, &request)
						}
					}},
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
				a.Close()
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
	pr.RateWindows = []ai.RateWindow{{Start: "22:00", End: "06:00", Rate: .5, Days: []int{1, 5}}}
	pr.Proxy = "direct"
	before, _ := json.Marshal(pr.ProviderPreferences)
	for n := 0; n < 4; n++ {
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

func TestPrivacyStartsWithProtectionAndKeepsRecordsSeparate(t *testing.T) {
	_, h, p, _ := setup(t)
	a := newAdvancedPanel(p)
	tt := ui.NewTester(func(c *ui.Context) { a.privacy(c, &magpieCatalog{}, false) }, 800, 1400)
	settle(h, tt)
	for _, key := range []string{"aiPrivacy.protection", "aiPrivacy.rules", "aiPrivacy.records", "aiPrivacy.access"} {
		if _, ok := tt.Find(i18n.T(key)); !ok {
			t.Errorf("missing privacy section %s", key)
		}
	}
	if a.records != nil {
		t.Error("opening protection unnecessarily loads history")
	}
	a.Close()
}

func TestRoutingGroupsStartsWithUpstreamToolbar(t *testing.T) {
	_, h, p, _ := setup(t)
	p.routing.state = &routeGroups{Groups: []core.Preserved[routeGroup]{{Value: routeGroup{ID: "one", Name: "Primary", Members: []string{"fixture/model"}, Ready: true}}}}
	tt := ui.NewTester(func(c *ui.Context) { p.routing.groupsView(c) }, 800, 900)
	settle(h, tt)
	if tt.HasText(nativeT("overview")) {
		t.Fatal("custom summary displaced the upstream group toolbar")
	}
	if !tt.HasText(nativeT("newGroup")) || !tt.HasText("Primary") {
		t.Fatal("group toolbar or existing group missing")
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

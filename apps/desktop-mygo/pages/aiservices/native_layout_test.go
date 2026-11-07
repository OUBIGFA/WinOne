package aiservices

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// The new native surfaces use real production controls at both supported
// widths, not HTML screenshots. Long IDs and errors may wrap, never overflow.
func TestNativeAISurfacesKeepTextInsideNarrowWindows(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, dark := range []bool{false, true} {
			_, h, p, _ := setup(t)
			i18n.SetLocale(locale)
			p.routing.state = &routeGroups{Groups: []core.Preserved[routeGroup]{{Value: routeGroup{ID: "long-group", Name: "A group with a descriptive title", Ref: "group/long-group", Members: []string{"provider-with-a-long-name/model-with-a-long-name"}}}}}
			p.plugins.state = &managedPlugins{Plugins: []managedPlugin{{Spec: "@example/native-plugin-with-a-long-name", OwnName: "Native plugin", Version: "1.0.0", Error: "The upstream returned a detailed error. Please verify the endpoint and retry."}}}
			views := []struct {
				name string
				view func(*ui.Context)
			}{
				{"groups", p.routing.groupsView},
				{"requests", func(c *ui.Context) {
					p.routing.trace.paused = true
					p.routing.trace.loaded = true
					p.routing.trace.View(c, p)
				}},
				{"plugins", func(c *ui.Context) { p.plugins.installedView(c, false) }},
				{"sources", func(c *ui.Context) { p.plugins.sourcesView(c, false) }},
				{"group-editor", func(c *ui.Context) {
					if p.routing.edit == nil {
						p.routing.begin(p.routing.state.Groups[0])
					}
					p.routing.editor(c)
				}},
			}
			for _, v := range views {
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(kit.Theme(c.Preferences(), dark))
					ui.Column(c).FillWidth().Gap(12).Children(func() { v.view(c) })
				}, 560, 6000)
				settle(h, tt)
				for _, text := range tt.Texts() {
					bounds, ok := tt.Find(text)
					if ok && (bounds.X < -.5 || bounds.X+bounds.W > 560.5) {
						t.Errorf("%s %s: %q exceeds width at %+v", locale, v.name, text, bounds)
					}
				}
				shot(t, tt, fmt.Sprintf("native-%s-%s-%t-560", v.name, locale, dark))
			}
		}
	}
}

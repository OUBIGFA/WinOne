package aiservices

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"os"
	"testing"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

func TestRefinementShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	_, h, p, _ := setup(t)
	r := p.routing
	const a = "11111111-2222-4333-8444-555555555555/gpt-6.1-sol"
	const b = "66666666-7777-4888-8999-000000000000/nofxone/gpt-6.1-sol"
	r.state = &routeGroups{Models: []routeModel{{ID: a, Name: "GPT 6.1 Sol", ProviderName: "Primary service", Efforts: []string{"low", "medium", "high"}}, {ID: b, Name: "GPT 6.1 Sol", ProviderName: "Backup service"}}, Groups: []core.Preserved[routeGroup]{{Value: routeGroup{ID: "writing", Name: "日常写作 · Writing", Ref: "group/writing", Ready: true, Members: []string{a, b}}}, {Value: routeGroup{ID: "review", Name: "代码审查 · Code review", Ref: "group/review", Routing: "order", Ready: true, Members: []string{b, a}}}}}
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for view := -1; view < 4; view++ {
			if view >= 0 {
				r.begin(r.state.Groups[0])
				r.editorTab = view
				r.edit.Value.Rules = []core.Preserved[routeRule]{{Value: routeRule{Use: a}}}
			}
			h.Shots(t, fmt.Sprintf("routing-%s-%d", locale, view), func(c *ui.Context) {
				ui.Column(c).FillWidth().Padding(24).Gap(16).Children(func() {
					if view < 0 {
						r.groupsView(c)
					} else {
						r.editor(c)
					}
				})
			}, 1100, nil)
		}
	}
	i18n.SetLocale(i18n.Chinese)
}

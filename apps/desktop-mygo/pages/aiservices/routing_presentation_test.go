package aiservices

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// Names are presentation only: the real references must survive editing.
func TestRoutingShowsModelNamesWithoutInternalReferences(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	const providerA = "11111111-2222-4333-8444-555555555555"
	const providerB = "66666666-7777-4888-8999-000000000000"
	a, b := providerA+"/gpt-6.1-sol", providerB+"/nofxone/gpt-6.1-sol"
	r.state = &routeGroups{Models: []routeModel{
		{ID: a, Name: "GPT 6.1 Sol", Provider: providerA, ProviderName: "Primary service"},
		{ID: b, Name: "GPT 6.1 Sol", Provider: providerB, ProviderName: "Backup service"},
	}, Groups: []core.Preserved[routeGroup]{{Value: routeGroup{ID: "writing", Name: "Writing", Ref: "group/writing", Ready: true, Members: []string{a, b}}}}}
	for _, view := range []string{"list", "basics", "members", "rules"} {
		t.Run(view, func(t *testing.T) {
			if view != "list" {
				r.begin(r.state.Groups[0])
				r.editorTab = map[string]int{"basics": 0, "members": 1, "rules": 2}[view]
				r.edit.Value.Rules = []core.Preserved[routeRule]{{Value: routeRule{Use: a}}}
			}
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).FillWidth().Children(func() {
					if view == "list" {
						r.groupsView(c)
					} else {
						r.editor(c)
					}
				})
			}, 800, 3000)
			settle(h, tt)
			joined := strings.Join(tt.Texts(), "\n")
			for _, internal := range []string{providerA, providerB, "group/writing"} {
				if strings.Contains(joined, internal) {
					t.Errorf("%s exposes internal reference %s", view, internal)
				}
			}
			if !strings.Contains(joined, "GPT 6.1 Sol") || !strings.Contains(joined, "Primary service") {
				t.Errorf("%s does not identify the model and provider: %s", view, joined)
			}
			if view == "members" {
				if err := tt.Click(nativeT("remove") + " GPT 6.1 Sol · Primary service"); err != nil {
					t.Fatal(err)
				}
				settle(h, tt)
				if len(r.edit.Value.Members) != 1 || r.edit.Value.Members[0] != b {
					t.Fatal("presentation changed the member references", r.edit.Value.Members)
				}
			}
		})
	}
	i18n.SetLocale(i18n.Chinese)
}

func TestRoutingGroupMenuKeepsCopyAndDeleteActions(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	r.state = &routeGroups{Groups: []core.Preserved[routeGroup]{{Value: routeGroup{ID: "writing", Name: "Writing", Members: []string{}}}}}
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).FillWidth().Children(func() { r.groupsView(c) }) }, 560, 900)
	for _, action := range []string{"copy", "delete"} {
		settle(h, tt)
		if err := tt.Click(nativeT("groupActions") + " Writing"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(nativeT(action)); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
		if r.dialog != action || r.text != "writing" {
			t.Fatalf("%s acted on the wrong group", action)
		}
		r.dialog = ""
	}
}

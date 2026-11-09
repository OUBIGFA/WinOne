package aiservices

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

func TestNewRouteNamesNeverReplaceExistingGroups(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	r.state = &routeGroups{Groups: []core.Preserved[routeGroup]{{Value: routeGroup{ID: "daily"}}, {Value: routeGroup{ID: "daily-2"}}, {Value: routeGroup{ID: "group"}}}}
	var saved routeGroup
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		json.NewDecoder(req.Body).Decode(&saved)
		w.Write([]byte(`{"groups":[]}`))
	}))
	defer server.Close()
	p.extensions.state.URL = server.URL + "/session/test/plugins.html"
	for _, tc := range []struct{ name, want string }{{"Daily", "daily-3"}, {"中文", "group-2"}, {"GPT 5...1", "gpt-5.1"}} {
		r.beginNew(core.Preserved[routeGroup]{Value: routeGroup{Name: tc.name, Members: []string{"p/m"}}})
		if got := r.groupID(); got != tc.want {
			t.Fatalf("%q: %s", tc.name, got)
		}
	}
	r.beginNew(core.Preserved[routeGroup]{Value: routeGroup{Name: "Daily", Members: []string{"p/m"}}})
	r.saveGroup()
	tt := ui.NewTester(func(*ui.Context) {}, 800, 600)
	settle(h, tt)
	if saved.ID != "daily-3" {
		t.Fatalf("saved colliding ID: %+v", saved)
	}
}

func TestRouteMemberKeyboardReorderKeepsFallbackOrder(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	r.state = &routeGroups{}
	r.begin(core.Preserved[routeGroup]{Value: routeGroup{ID: "g", Members: []string{"a", "b", "c"}, Off: []string{"b"}, Pick: "a"}})
	tt := ui.NewTester(func(c *ui.Context) { r.membersView(c, &r.edit.Value, false) }, 800, 2200)
	settle(h, tt)
	if err := tt.Click(upstreamT("Move {name}", "name", r.memberLabel("b"))); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Alt, ui.KeyUp)
	settle(h, tt)
	if !slices.Equal(r.edit.Value.Members, []string{"b", "a", "c"}) || r.edit.Value.Pick != "a" || !slices.Equal(r.edit.Value.Off, []string{"b"}) {
		t.Fatalf("reorder lost links: %+v", r.edit.Value)
	}
	from, ok := tt.Find(upstreamT("Move {name}", "name", r.memberLabel("c")))
	if !ok {
		t.Fatal("missing drag handle")
	}
	to, _ := tt.Find(upstreamT("Move {name}", "name", r.memberLabel("b")))
	x := from.X + from.W/2
	tt.Press(x, from.Y+from.H/2)
	for y := from.Y + from.H/2; y > to.Y+to.H/2; y -= 8 {
		tt.Move(x, y)
	}
	tt.Move(x, to.Y+to.H/2)
	tt.Release(x, to.Y+to.H/2)
	settle(h, tt)
	if !slices.Equal(r.edit.Value.Members, []string{"c", "b", "a"}) || !slices.Equal(r.edit.Value.Placed, r.edit.Value.Members) {
		t.Fatalf("drag did not preserve fallback order: %+v", r.edit.Value)
	}
}

func TestRoutingCancelAndPauseConversionKeepAValidDraft(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	r.state = &routeGroups{}
	r.beginNew(core.Preserved[routeGroup]{Value: routeGroup{Name: "New", Members: []string{"a", "b"}}})
	tt := ui.NewTester(r.editor, 800, 1600)
	settle(h, tt)
	if err := tt.Click(i18n.T("common.cancel")); err != nil {
		t.Fatal(err)
	}
	if r.edit != nil {
		t.Fatal("Cancel retained the new draft")
	}
	r.begin(core.Preserved[routeGroup]{Value: routeGroup{ID: "g", Members: []string{"a", "b"}, Rules: []core.Preserved[routeRule]{{Value: routeRule{Use: "a", More: []string{"b"}, UseEffort: "high", Tokens: 1, Images: true, Compact: true, Intent: "coding", Agents: []string{"codex"}}}}}})
	r.editorTab = 2
	tt = ui.NewTester(r.editor, 800, 1800)
	settle(h, tt)
	if err := tt.Click(nativeT("pauseMember")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	rule := r.edit.Value.Rules[0].Value
	if !rule.Pause || len(rule.More) != 0 || rule.UseEffort != "" || rule.Tokens != 0 || rule.Images || rule.Compact || rule.Intent != "" || !slices.Equal(rule.Agents, []string{"codex"}) {
		t.Fatalf("pause retained unsupported hidden conditions: %+v", rule)
	}
}

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

func TestRoutingKeepsGroupsRequestsAndPoolsReachable(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	r.state = &routeGroups{Pools: []routePool{{Provider: "one", Name: "Pool fixture", Who: []string{"first", "second"}}}}
	r.source = p.extensions.state.URL
	r.capabilities = &routeCapabilities{}
	p.switchTab(tabRouting)
	tt := ui.NewTester(p.View, 1000, 1600)
	settle(h, tt)
	if err := tt.Click(nativeT("accountPools")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !tt.HasText("Pool fixture") {
		t.Fatal("account pool settings are unreachable")
	}
}

func TestRoutingWeightSortRetainsMembersAndLinks(t *testing.T) {
	_, h, p, _ := setup(t)
	r := p.routing
	r.state = &routeGroups{}
	r.begin(core.Preserved[routeGroup]{Value: routeGroup{ID: "weight", Routing: "weight", Members: []string{"a/a", "b/b", "c/c"}, Weights: map[string]int{"a/a": 1, "b/b": 5, "c/c": 5}, Pick: "a/a", Off: []string{"c/c"}}})
	tt := ui.NewTester(func(c *ui.Context) { r.membersView(c, &r.edit.Value, false) }, 800, 2200)
	settle(h, tt)
	if err := tt.Click(upstreamT("Sort by weight")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	g := r.edit.Value
	if strings.Join(g.Members, ",") != "b/b,c/c,a/a" || g.Pick != "a/a" || strings.Join(g.Off, ",") != "c/c" {
		t.Fatalf("weight sort changed membership: %+v", g)
	}
}

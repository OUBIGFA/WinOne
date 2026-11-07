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
	"winone/internal/ai"
)

func TestGroupAccountChoiceUsesTheExactUpstreamID(t *testing.T) {
	_, h, p, _ := setup(t)
	var state routeGroups
	if err := json.Unmarshal([]byte(`{"groups":[],"accounts":{"fixture":[{"id":"fixture/key-123","name":"Fixture key"}]}}`), &state); err != nil {
		t.Fatal(err)
	}
	p.routing.state = &state
	p.routing.begin(core.Preserved[routeGroup]{Value: routeGroup{ID: "g", Members: []string{"fixture/model"}}})
	tt := ui.NewTester(p.routing.editor, 900, 8000)
	if err := tt.Click("Fixture key"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if got := p.routing.edit.Value.Accounts; !slices.Equal(got, []string{"fixture/key-123"}) {
		t.Fatalf("account ID was rewritten: %q", got)
	}
}

func TestRemovingAGroupMemberKeepsRulesValidAndOtherMemberPreferences(t *testing.T) {
	g := routeGroup{
		Members: []string{"a/m", "b/m"}, Off: []string{"a/m"}, Fast: []string{"a/m", "b/m"}, Placed: []string{"a/m", "b/m"}, Pick: "a/m", Weights: map[string]int{"a/m": 2, "b/m": 3},
		Rules: []core.Preserved[routeRule]{{Value: routeRule{Use: "a/m", More: []string{"b/m"}}}, {Value: routeRule{Use: "a/m", Pause: true}}},
	}
	removeRouteMember(&g, "a/m")
	if !slices.Equal(g.Members, []string{"b/m"}) || g.Pick != "" || len(g.Off) != 0 || len(g.Rules) != 1 || g.Rules[0].Value.Use != "b/m" || g.Weights["b/m"] != 3 {
		t.Fatalf("member removal broke linked choices: %+v", g)
	}
	if _, exists := g.Weights["a/m"]; exists {
		t.Fatal("removed member retained its weight")
	}
}

func TestAnotherPluginMutationAsksBeforeDiscardingOptionDraft(t *testing.T) {
	_, h, p, tt := setup(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.Write([]byte(`{"plugins":[]}`)) }))
	defer server.Close()
	p.extensions.state.URL = server.URL + "/session/key/plugins.html"
	p.plugins.optionsSpec, p.plugins.optionsSaved, p.plugins.optionsText = "fixture", `{}`, `{"changed":true}`
	var apply func()
	h.Env.RequestChange = func(fn func()) { apply = fn }
	p.plugins.act("off", map[string]any{"spec": "fixture", "off": true})
	settle(h, tt)
	if apply == nil || requests != 0 || !p.plugins.dirty() {
		t.Fatal("mutation bypassed the unsaved option draft guard")
	}
	apply()
	settle(h, tt)
	if requests != 1 || p.plugins.dirty() {
		t.Fatalf("approved mutation did not finish: requests=%d dirty=%v error=%s", requests, p.plugins.dirty(), p.plugins.api.err)
	}
}

func TestNativeLatestSettingsSaveOriginalKeysAndRemainDraftsUntilSave(t *testing.T) {
	f, h, p, tt := setup(t)
	p.switchTab(tabAdvanced)
	p.advancedTab = advancedModels
	settle(h, tt)
	s := p.draft.Value.settings()
	s.RouterAllTools = true
	s.RouterNote = "User router note"
	s.AgentCompacts = map[string]ai.AgentCompact{"claude": {At: 200000}}
	s.SearcherThen = []string{"fixture/model"}
	s.RefusalRetries = ptr(0.0)
	if len(f.saves) != 0 {
		t.Fatal("editing latest settings saved them implicitly")
	}
	p.save()
	settle(h, tt)
	data, _ := json.Marshal(p.saved)
	for _, key := range []string{`"routerAllTools":true`, `"routerNote":"User router note"`, `"agentCompacts"`, `"searcherThen"`, `"refusalRetries":0`} {
		if !strings.Contains(string(data), key) {
			t.Fatalf("saved document lost %s: %s", key, data)
		}
	}
}

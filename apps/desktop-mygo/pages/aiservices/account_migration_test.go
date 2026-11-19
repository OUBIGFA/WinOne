package aiservices

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"slices"
	"testing"
	"winone/desktopmygo/testhost"
)

// A regional plan replaces the default plan; neither its chat nor media
// candidates should silently come from another subscription's region.
func TestPresetStartsWithOnlyTheSelectedPlansChatAndMedia(t *testing.T) {
	_, _, p, _ := setup(t)
	p.addPreset(servicePreset{ID: "regional-plan", Name: "Plan", Models: []string{"default-text"}, Drawers: []string{"default-image"}, Regions: []presetRegion{{ID: "selected", Name: "Selected", Models: []string{"regional-text"}, Drawers: []string{"regional-image"}, Speakers: []string{"regional-tts"}, endpoints: endpoints{Chat: "https://regional.example/v1"}}}})
	got := p.current()
	if !slices.Equal(got.available(), []string{"regional-image", "regional-text", "regional-tts"}) || len(got.Models) != 0 {
		t.Fatalf("preset mixed plan candidates or enabled them without a choice: %+v", got)
	}
}

// A backend changes only after every private account import succeeds, and
// only in the draft. Partial imports keep the working original connection.
func TestAntigravityMigrationKeepsSavedBackendUntilCompleteAndSaved(t *testing.T) {
	for _, tc := range []struct{ name, reply, want string }{
		{"complete", `{"results":[{"user":"one@example.invalid","status":"added"}]}`, "agy-cli"},
		{"partial", `{"results":[{"user":"one@example.invalid","status":"added"},{"user":"two@example.invalid","status":"failed","error":"failed"}]}`, "antigravity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAI(t)
			f.configuration["providers"] = []any{map[string]any{"id": "old-private", "name": "My account", "authMode": "account", "agent": "antigravity", "agentBackend": "plugin", "models": []any{"gemini-model"}, "proxy": "direct"}}
			f.configuration["tasks"] = map[string]any{}
			h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
				if method == "capture.ai.accounts.move-agy" {
					return json.RawMessage(tc.reply), nil
				}
				return f.handle(method, params)
			})
			p := newPage(h.Env)
			defer p.Close()
			tt := ui.NewTester(p.View, 1100, 1600)
			settle(h, tt)
			pr := p.draft.Value.find("old-private")
			a := newAccountPanel(p, pr)
			defer a.Close()
			a.moveToAgy(pr)
			settle(h, tt)
			if got := p.draft.Value.find("old-private"); got.Agent != tc.want || got.Proxy != "direct" || got.Name != "My account" || len(got.Models) != 1 {
				t.Fatalf("migration changed unrelated settings or lost its result: %+v", got)
			}
			if p.saved.Value.find("old-private").Agent != "antigravity" || len(f.saves) != 0 {
				t.Fatal("migration silently saved the backend change")
			}
		})
	}
}

package automations

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEditingKeepsLegacyTimingPermissionAndUnknownFields(t *testing.T) {
	const raw = `{"id":"old","name":"Before","prompt":"Check","agentKind":"pi","kind":"cron","cronExpr":"0 9 * * *","timezone":"Asia/Shanghai","recurring":false,"intervalMs":5400123,"onceAt":1600000000000,"permissionMode":"ask","futureField":{"keep":true},"preRunPlugin":{"pluginId":"plugin-one","onFailure":"skip"}}`
	var source Schedule
	if err := json.Unmarshal([]byte(raw), &source); err != nil {
		t.Fatal(err)
	}
	d := newDraft(&source)
	if d.changed() {
		t.Fatal("opening changed the definition")
	}
	d.Value.Name = "After"
	patch := d.patch()
	if !reflect.DeepEqual(patch, map[string]any{"name": "After"}) {
		t.Fatalf("name edit changed other fields: %#v", patch)
	}
	var persisted map[string]any
	_ = json.Unmarshal([]byte(raw), &persisted)
	for k, v := range patch {
		persisted[k] = v
	}
	if persisted["permissionMode"] != "ask" || persisted["onceAt"] != float64(1600000000000) || persisted["intervalMs"] != float64(5400123) || persisted["futureField"].(map[string]any)["keep"] != true {
		t.Fatalf("legacy data changed: %#v", persisted)
	}
	d.Value.PreRunPlugin = nil
	if value, ok := d.patch()["preRunPlugin"]; !ok || value != nil {
		t.Fatal("removing plugin must send explicit null")
	}
}
func TestScheduleSortFilterAndHandBoundDeletion(t *testing.T) {
	rows := []Schedule{{ID: "paused", Status: "paused", LastFiredAt: 100}, {ID: "new", Status: "active", UpdatedAt: 50}, {ID: "once", Status: "expired", LastFiredAt: 10}, {ID: "old", Status: "active", LastFiredAt: 5}}
	actual := visible(rows, "all", "", "")
	ids := []string{}
	for _, r := range actual {
		ids = append(ids, r.ID)
	}
	if !reflect.DeepEqual(ids, []string{"once", "old", "new", "paused"}) {
		t.Fatalf("Cindy ordering changed: %v", ids)
	}
	if len(visible(rows, "active", "", "")) != 3 {
		t.Fatal("active view must include expired one-shot schedules")
	}
	source := Schedule{Definition: Definition{TargetSessionID: "owner"}}
	got := generatedSessionIDs(source, []Run{{SessionID: "owner"}, {SessionID: "made"}, {SessionID: "made"}, {SessionID: ""}})
	if !reflect.DeepEqual(got, []string{"made"}) {
		t.Fatalf("deletion includes the hand-bound owner: %v", got)
	}
}
func TestProjectPreviewPushDoesNotCauseAReadPublishLoop(t *testing.T) {
	p := &Page{}
	preview := json.RawMessage(`{"type":"import-required","pendingImport":{"workingDir":"C:\\project","configHash":"one","schedules":[],"removedNames":[]}}`)
	if p.receiveEvents([]json.RawMessage{preview}) || len(p.imports) != 1 {
		t.Fatal("project preview requested a scheduler refresh")
	}
	if p.receiveEvents([]json.RawMessage{preview}) || len(p.imports) != 1 {
		t.Fatal("duplicate project preview accumulated")
	}
	if !p.receiveEvents([]json.RawMessage{json.RawMessage(`{"type":"reconciled","workingDir":"C:\\project","inserted":1}`)}) || len(p.imports) != 0 {
		t.Fatal("accepted project preview did not clear and refresh")
	}
}

func TestInvalidTimingBuffersBlockSaveAndRemainDirty(t *testing.T) {
	d := newDraft(nil, "Asia/Shanghai")
	d.Value.Name = "Check"
	d.Value.Prompt = "Inspect"
	if d.Value.Timezone != "Asia/Shanghai" {
		t.Fatal("the Host timezone was lost")
	}
	d.timing = "interval"
	d.interval = "not a number"
	if !d.changed() || d.validate() == nil {
		t.Fatal("invalid visible interval silently used the old value")
	}
	d.interval = "1"
	d.intervalUnit = "seconds"
	if err := d.validate(); err != nil {
		t.Fatal(err)
	}
	d.hookTimeout = "NaN"
	if d.validate() == nil {
		t.Fatal("non-finite timeout accepted")
	}
}

func TestCronPresetsRoundTripAndCustomInputIsNotRewritten(t *testing.T) {
	for _, expression := range []string{"* * * * *", "0 * * * *", "17 6 * * *", "17 6 * * 3", "17 6 * * 1-5", "17 6 * * 0,6", "17 6 31 * *", "0 */3 * * *", "*/15 * * * *"} {
		if got := readCron(expression).expression(); got != expression {
			t.Fatalf("%s became %s", expression, got)
		}
	}
	if readCron("5,25 6-18 * * 1,3,5").Mode != "custom" {
		t.Fatal("complex cron was coerced into a preset")
	}
	groups := groupRuns([]Run{{ID: "one", SessionID: "a"}, {ID: "two", SessionID: "b"}, {ID: "three", SessionID: "a"}, {ID: "skip"}}, true)
	if len(groups) != 3 || len(groups[0].runs) != 2 || groups[0].sessionID != "a" || groups[2].sessionID != "" {
		t.Fatalf("history grouping lost runs: %#v", groups)
	}
}

func TestIndependentDraftsDoNotShareHooksOrScriptCapabilities(t *testing.T) {
	timeout := int64(30000)
	s := Schedule{Definition: Definition{Name: "Run", Prompt: "Check", ScriptConfig: &ScriptConfig{Command: "old", TimeoutMs: &timeout, Capabilities: []string{"github.read"}}, PreRunHook: &Hook{Command: "check"}}}
	first, second := newDraft(&s), newDraft(&s)
	first.Value.ScriptConfig.Capabilities[0] = "jira.read"
	first.Value.PreRunHook.Command = "changed"
	if second.Value.ScriptConfig.Capabilities[0] != "github.read" || s.PreRunHook.Command != "check" {
		t.Fatal("editing one draft changed stored or sibling data")
	}
}

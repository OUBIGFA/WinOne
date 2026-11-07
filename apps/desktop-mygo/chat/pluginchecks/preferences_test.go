package pluginchecks

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func TestPluginPreferenceSaveKeepsChangesMadeWhileTheRequestWasPending(t *testing.T) {
	started, release := make(chan json.RawMessage, 1), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		if method != "chat.ghosts.configure" {
			t.Fatalf("unexpected reload while draft is newer: %s", method)
		}
		started <- append(json.RawMessage(nil), params...)
		<-release
		return map[string]bool{"ok": true}, nil
	})
	m := &Manager{env: h.Env, open: true, configuration: &plugin{PluginID: "fixture", Name: "Fixture"}, parameters: `{"value":1}`, baseline: "{}", showTaskPreferences: true,
		taskPreferences: taskPreferences{AgentKind: "pi", ProviderID: "vendor", Model: "model", Effort: "vendor-word"},
		secretText:      map[string]string{"token": "first"}, secretClear: map[string]bool{}, secrets: []secret{{Key: "token", Source: "user"}}}
	defer m.Close()
	m.save()
	var sent json.RawMessage
	select {
	case sent = <-started:
	case <-time.After(time.Second):
		t.Fatal("save did not start")
	}
	var request struct {
		TaskPreferences taskPreferences `json:"taskPreferences"`
	}
	if err := json.Unmarshal(sent, &request); err != nil || request.TaskPreferences.Effort != "vendor-word" {
		t.Fatalf("provider value lost: %s %v", sent, err)
	}
	m.parameters = `{"value":2}`
	m.taskPreferences.WorkingDir = `C:\new-selection`
	m.secretText["token"] = "second"
	close(release)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); m.View(c) }, 1000, 1000)
	for range 3 {
		h.Settle(tt)
	}
	if !m.Dirty() || m.parameters != `{"value":2}` || m.taskPreferences.WorkingDir != `C:\new-selection` || m.secretText["token"] != "second" {
		t.Fatalf("late save discarded a newer draft: %+v", m.taskPreferences)
	}
	if m.baseline != `{"value":1}` || m.taskBaseline.WorkingDir != "" {
		t.Fatalf("wrong save baseline: %s %+v", m.baseline, m.taskBaseline)
	}
}

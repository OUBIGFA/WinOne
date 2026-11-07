package host

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/settings"
)

// The same existing job can be read instantly by old callers, or awaited by a
// new caller. Completion must wake that reader, never masquerade as acceptance.
func TestConfigurationJobWaitUsesItsActualCompletionSignal(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := New(store)
	entered, release := make(chan struct{}), make(chan struct{})
	service.OnSettingsChanged(func(settings.Settings) { close(entered); <-release })
	queue := NewConfigurationQueue(service)
	response := queue.Handle(Request{Version: 1, ID: "save", Method: "settings.save", Params: json.RawMessage(`{"schemaVersion":1,"revision":0,"theme":"dark"}`)})
	var ticket struct {
		ID     string `json:"configurationJob"`
		WaitMs int    `json:"configurationJobWaitMs"`
	}
	if json.Unmarshal(response.Result, &ticket) != nil || ticket.ID == "" {
		t.Fatalf("no actual job receipt: %+v", response)
	}
	<-entered
	poll := func(wait int) Response {
		params, _ := json.Marshal(map[string]any{"id": ticket.ID, "waitMs": wait})
		return queue.Handle(Request{Version: 1, ID: "read", Method: "configuration.jobs.get", Params: params})
	}
	var current ConfigurationJob
	if json.Unmarshal(poll(0).Result, &current) != nil || current.State != "running" {
		t.Fatal("legacy immediate reader no longer works")
	}
	if ticket.WaitMs <= 0 || ticket.WaitMs > 1000 {
		close(release)
		t.Fatal("Host did not advertise its bounded completion wait")
	}
	finished := make(chan Response, 1)
	go func() { finished <- poll(ticket.WaitMs) }()
	select {
	case result := <-finished:
		close(release)
		t.Fatalf("completion waiter returned before the operation: %s", result.Result)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case result := <-finished:
		if json.Unmarshal(result.Result, &current) != nil || current.State != "completed" || current.Response == nil || current.Response.ID != "save" {
			t.Fatalf("lost original completed receipt: %s", result.Result)
		}
	case <-time.After(time.Second):
		t.Fatal("completed job did not wake its reader")
	}
}

func TestConfigurationJobWaitRejectsUnboundedOrNegativeWaits(t *testing.T) {
	queue := NewConfigurationQueue(nil)
	for _, milliseconds := range []int{-1, 1001} {
		params, _ := json.Marshal(map[string]any{"id": "unknown", "waitMs": milliseconds})
		response := queue.Handle(Request{Version: 1, ID: "read", Method: "configuration.jobs.get", Params: params})
		if response.Error == nil || response.Error.Code != "invalid" {
			t.Fatalf("unbounded wait was not rejected: %+v", response)
		}
	}
}

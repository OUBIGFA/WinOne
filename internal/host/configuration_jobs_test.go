package host

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"winone/internal/settings"
)

func TestMaintenanceConcurrentAdmissionDoesNotWaitOrOverlap(t *testing.T) {
	queue := NewConfigurationQueue(nil)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for attempt := 0; attempt < 500; attempt++ {
				if release, ok := queue.AcquireAction(); ok {
					release()
				}
				if release, ok := queue.AcquireMaintenance(); ok {
					if action, admitted := queue.AcquireAction(); admitted {
						action()
						t.Error("overlapping action")
					}
					release()
				}
			}
		}()
	}
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("admission stalled")
	}
}

func TestMaintenanceExcludesActionsAndConfigurationUntilReleased(t *testing.T) {
	store, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	queue := NewConfigurationQueue(New(store))
	action, ok := queue.AcquireAction()
	if !ok {
		t.Fatal("initial action rejected")
	}
	if release, ok := queue.AcquireMaintenance(); ok {
		release()
		t.Fatal("maintenance overtook action")
	}
	action()
	release, ok := queue.AcquireMaintenance()
	if !ok {
		t.Fatal("idle maintenance rejected")
	}
	defer release()
	if !queue.Busy() {
		t.Fatal("maintenance not visible")
	}
	if action, ok := queue.AcquireAction(); ok {
		action()
		t.Fatal("action admitted during maintenance")
	}
	if other, ok := queue.AcquireMaintenance(); ok {
		other()
		t.Fatal("overlapping maintenance admitted")
	}
	response := queue.Handle(Request{Version: 1, ID: "save", Method: "settings.save", Params: json.RawMessage(`{"schemaVersion":1,"revision":0,"theme":"dark"}`)})
	if response.Error == nil {
		t.Fatal("save admitted during maintenance")
	}
	release()
	release() // Failure cleanup can be reached through more than one owner.
	if queue.Busy() {
		t.Fatal("release did not reopen admission")
	}
	if action, ok := queue.AcquireAction(); !ok {
		t.Fatal("action rejected after release")
	} else {
		action()
	}
}

func TestConfigurationJobReturnsBeforeSaveAndPublishesFinalResult(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := New(store)
	entered, release := make(chan struct{}), make(chan struct{})
	service.OnSettingsChanged(func(settings.Settings) { close(entered); <-release })
	queue := NewConfigurationQueue(service)
	request := Request{Version: 1, ID: "save", Method: "settings.save", Params: json.RawMessage(`{"schemaVersion":1,"revision":0,"theme":"dark"}`)}
	response := queue.Handle(request)
	var ticket struct {
		ID string `json:"configurationJob"`
	}
	if err = json.Unmarshal(response.Result, &ticket); err != nil || ticket.ID == "" {
		t.Fatal(response, err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("save did not run")
	}
	params, _ := json.Marshal(map[string]string{"id": ticket.ID})
	poll := Request{Version: 1, ID: "poll", Method: "configuration.jobs.get", Params: params}
	response = queue.Handle(poll)
	var job ConfigurationJob
	if err = json.Unmarshal(response.Result, &job); err != nil {
		t.Fatal(err)
	}
	if job.State != "running" || job.Response != nil || !queue.Busy() {
		t.Fatal(job)
	}
	if duplicate := queue.Handle(request); duplicate.Error == nil {
		t.Fatal("overlapping configuration accepted")
	}
	if release, ok := queue.AcquireMaintenance(); ok {
		release()
		t.Fatal("maintenance admitted during save")
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for queue.Busy() {
		if time.Now().After(deadline) {
			t.Fatal("save did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	response = queue.Handle(poll)
	if err = json.Unmarshal(response.Result, &job); err != nil {
		t.Fatal(err)
	}
	if job.State != "completed" || job.Response == nil || job.Response.Error != nil {
		t.Fatal(job)
	}
	var saved settings.Settings
	if err = json.Unmarshal(job.Response.Result, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Theme != "dark" || saved.Revision != 1 {
		t.Fatal(saved)
	}
}

func TestConfigurationJobPreservesFailureCode(t *testing.T) {
	store, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	queue := NewConfigurationQueue(New(store))
	response := queue.Handle(Request{Version: 1, ID: "bad", Method: "settings.save", Params: json.RawMessage(`{"schemaVersion":1,"revision":99,"theme":"dark"}`)})
	var ticket struct {
		ID string `json:"configurationJob"`
	}
	if err := json.Unmarshal(response.Result, &ticket); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for queue.Busy() {
		if time.Now().After(deadline) {
			t.Fatal("failed save did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	params, _ := json.Marshal(map[string]string{"id": ticket.ID})
	response = queue.Handle(Request{Version: 1, ID: "poll", Method: "configuration.jobs.get", Params: params})
	var job ConfigurationJob
	if err := json.Unmarshal(response.Result, &job); err != nil {
		t.Fatal(err)
	}
	if job.State != "failed" || job.Response == nil || job.Response.Error == nil || job.Response.Error.Code != "conflict" {
		t.Fatal(job)
	}
}

func TestConfigurationWaitsForAdmittedActionAndRejectsNewActions(t *testing.T) {
	store, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	service := New(store)
	applied := make(chan struct{})
	service.OnSettingsChanged(func(settings.Settings) { close(applied) })
	queue := NewConfigurationQueue(service)
	release, allowed := queue.AcquireAction()
	if !allowed {
		t.Fatal("idle action rejected")
	}
	response := queue.Handle(Request{Version: 1, ID: "save", Method: "settings.save", Params: json.RawMessage(`{"schemaVersion":1,"revision":0,"theme":"dark"}`)})
	if response.Error != nil {
		release()
		t.Fatal(response.Error)
	}
	if later, allowed := queue.AcquireAction(); allowed {
		later()
		release()
		t.Fatal("action admitted during transaction")
	}
	select {
	case <-applied:
		release()
		t.Fatal("transaction overtook admitted action")
	default:
	}
	release()
	select {
	case <-applied:
	case <-time.After(3 * time.Second):
		t.Fatal("transaction did not resume")
	}
}

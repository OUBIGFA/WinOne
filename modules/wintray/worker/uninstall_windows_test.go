//go:build windows

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"winone/internal/host"
)

type testProductLogon struct {
	removes int
	err     error
}

func (p *testProductLogon) Sync(string, string) error { return nil }
func (p *testProductLogon) Remove() error             { p.removes++; return p.err }

func uninstallJob(t *testing.T, r *Runtime) job {
	t.Helper()
	response := r.Handle(host.Request{Version: 1, ID: "uninstall", Method: "uninstall.prepare"})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var result job
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatal(err)
	}
	r.work.Wait()
	if result.ID != "" {
		r.jobsMu.Lock()
		result = r.jobs[result.ID]
		r.jobsMu.Unlock()
	}
	return result
}

func TestUninstallPreparationRetriesProductLogonRemoval(t *testing.T) {
	task := &testProductLogon{err: errors.New("task scheduler unavailable")}
	r := &Runtime{ctx: context.Background(), jobs: map[string]job{}, deactivated: true, allowSystemStartup: true, productLogon: task}
	if got := uninstallJob(t, r); got.State != "failed" || task.removes != 1 {
		t.Fatalf("failure hidden: %+v", got)
	}
	task.err = nil
	if got := uninstallJob(t, r); got.State != "completed" || task.removes != 2 {
		t.Fatalf("retry failed: %+v", got)
	}
	if !r.deactivated {
		t.Fatal("restored resources were reactivated")
	}
}

func TestUninstallPreparationRejectsBusyAndPreservesOrdinaryDisable(t *testing.T) {
	task := &testProductLogon{}
	r := &Runtime{ctx: context.Background(), jobs: map[string]job{"active": {State: "running"}}, deactivated: true, allowSystemStartup: true, productLogon: task}
	if got := uninstallJob(t, r); got.State != "failed" || task.removes != 0 {
		t.Fatalf("busy accepted: %+v", got)
	}
	delete(r.jobs, "active")
	if got := r.startDeactivation(); got.State != "completed" || task.removes != 0 {
		t.Fatalf("disable removed login task: %+v", got)
	}
}

func TestUninstallPreparationTestProfileDoesNotRemoveLoginTask(t *testing.T) {
	task := &testProductLogon{}
	r := &Runtime{ctx: context.Background(), jobs: map[string]job{}, deactivated: true, productLogon: task}
	if got := uninstallJob(t, r); got.State != "completed" || task.removes != 0 {
		t.Fatalf("test profile modified system: %+v", got)
	}
}

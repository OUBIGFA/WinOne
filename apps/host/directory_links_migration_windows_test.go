//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"winone/internal/directorylinks"
	"winone/internal/host"
)

func awaitMigrationJob(t *testing.T, j *directoryMigrationJobs, owner uint32, id string) directoryMigrationJob {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		value, err := j.read(owner, id, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if value.State != "running" {
			return value
		}
		if time.Now().After(deadline) {
			t.Fatal("migration job did not settle")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func TestDirectoryMigrationJobsAreOwnedCancelableAndHoldActionLease(t *testing.T) {
	var leases atomic.Int32
	j := &directoryMigrationJobs{guard: func() (func(), bool) { leases.Add(1); return func() { leases.Add(-1) }, true }}
	defer j.Close()
	entered := make(chan struct{})
	value, err := j.start(11, "request-one", func(ctx context.Context, progress func(directorylinks.MigrationProgress)) (*directorylinks.MigrationPlan, *directorylinks.Record, error) {
		progress(directorylinks.MigrationProgress{Phase: "copying", Bytes: 5})
		close(entered)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	if err != nil || value.State != "running" {
		t.Fatal(value, err)
	}
	<-entered
	if leases.Load() != 1 {
		t.Fatal("action lease was released before background work")
	}
	if _, err = j.read(12, value.ID, "", true); err == nil {
		t.Fatal("foreign cancellation accepted")
	}
	if _, err = j.read(11, "", "other-request", false); err == nil {
		t.Fatal("different request shown as current outcome")
	}
	if recovered, err := j.read(11, "", "request-one", false); err != nil || recovered.ID != value.ID {
		t.Fatal("lost admission receipt cannot be recovered", recovered, err)
	}
	if _, err = j.read(11, value.ID, "", true); err != nil {
		t.Fatal(err)
	}
	if done := awaitMigrationJob(t, j, 11, value.ID); done.State != "cancelled" {
		t.Fatal(done)
	}
	j.Close()
	if leases.Load() != 0 {
		t.Fatal("terminal work kept action lease")
	}
}
func TestDirectoryMigrationJobCloseWaitsForRollbackAndPreviewTokensAreOwned(t *testing.T) {
	j := &directoryMigrationJobs{}
	value, err := j.start(1, "preview", func(context.Context, func(directorylinks.MigrationProgress)) (*directorylinks.MigrationPlan, *directorylinks.Record, error) {
		return &directorylinks.MigrationPlan{ID: "plan", ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)}, nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	awaitMigrationJob(t, j, 1, value.ID)
	if err = j.claim(2, "plan"); err == nil {
		t.Fatal("foreign plan accepted")
	}
	if err = j.claim(1, "plan"); err != nil {
		t.Fatal(err)
	}
	if err = j.claim(1, "plan"); err == nil {
		t.Fatal("token replay accepted")
	}
	rollback := make(chan struct{})
	release := make(chan struct{})
	closed := make(chan struct{})
	_, err = j.start(1, "execute", func(ctx context.Context, _ func(directorylinks.MigrationProgress)) (*directorylinks.MigrationPlan, *directorylinks.Record, error) {
		<-ctx.Done()
		close(rollback)
		<-release
		return nil, nil, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { j.Close(); close(closed) }()
	<-rollback
	select {
	case <-closed:
		t.Fatal("close returned before rollback finished")
	default:
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close did not settle")
	}
	if _, err = j.start(1, "late", nil); err == nil {
		t.Fatal("closed registry restarted")
	}
}
func TestDirectoryMigrationPreviewAndConfirmationRPC(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	for _, p := range []string{source, target} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := directorylinks.New(filepath.Join(root, "history"))
	if err != nil {
		t.Fatal(err)
	}
	jobs := &directoryMigrationJobs{}
	defer jobs.Close()
	handle := directoryLinksHandler(m, func(uint32) error { return nil }, jobs)
	call := func(owner uint32, method string, args any) host.Response {
		raw, _ := json.Marshal(args)
		return handle(host.Request{Version: 1, ID: method, PeerPID: owner, Method: method, Params: raw})
	}
	response := call(1, "migration.preview", directorylinks.MigrationRequest{Source: source, Directory: target, RequestID: "preview-one"})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var job directoryMigrationJob
	if err = json.Unmarshal(response.Result, &job); err != nil {
		t.Fatal(err)
	}
	job = awaitMigrationJob(t, jobs, 1, job.ID)
	if job.State != "completed" || job.Plan == nil {
		t.Fatal(job)
	}
	if len(m.List()) != 0 {
		t.Fatal("preview wrote history")
	}
	if r := call(1, "migration.execute", map[string]any{"token": job.Plan.ID, "requestId": "execute-one"}); r.Error == nil {
		t.Fatal("unconfirmed execution accepted")
	}
	if r := call(2, "migration.execute", map[string]any{"token": job.Plan.ID, "requestId": "execute-one", "confirmed": true}); r.Error == nil {
		t.Fatal("foreign execution accepted")
	}
	if _, err = os.Stat(filepath.Join(source, "file.txt")); err != nil {
		t.Fatal("rejected execution changed data", err)
	}
}
func TestDirectoryMigrationFailedAdmissionDoesNotStartWork(t *testing.T) {
	j := &directoryMigrationJobs{guard: func() (func(), bool) { return nil, false }}
	defer j.Close()
	called := false
	_, err := j.start(1, "blocked", func(context.Context, func(directorylinks.MigrationProgress)) (*directorylinks.MigrationPlan, *directorylinks.Record, error) {
		called = true
		return nil, nil, errors.New("not reachable")
	})
	if err == nil || called {
		t.Fatal("configuration guard bypassed", err)
	}
}

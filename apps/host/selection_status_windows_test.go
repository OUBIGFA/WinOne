//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectionOperationFailureIsNotALifecycleFaultAndSuccessClearsIt(t *testing.T) {
	r := &selectionRuntime{}
	current := func() bool { return true }
	r.finishOperation(r.beginOperation(), current, context.DeadlineExceeded)
	s := r.status()
	if s.State != "ready" || s.Detail != "" || !strings.Contains(s.Warning, "context deadline exceeded") {
		t.Fatalf("a single timeout became a module fault or lost its reason: %+v", s)
	}
	// Automatic and manual reads both publish through this completion path.
	r.finishOperation(r.beginOperation(), current, nil)
	if s := r.status(); s.State != "ready" || s.Warning != "" {
		t.Fatalf("success kept the old warning: %+v", s)
	}
	r.lastError = "划词读取组件不可用，请修复安装"
	r.finishOperation(r.beginOperation(), current, nil)
	if s := r.status(); s.State != "faulted" || s.Detail != r.lastError {
		t.Fatalf("an operation erased a real startup fault: %+v", s)
	}
}

func TestSelectionDiagnosticLogTracksFailuresAndRecoveryOnce(t *testing.T) {
	directory := t.TempDir()
	r := &selectionRuntime{results: &resultWindows{logDirectory: directory}}
	current := func() bool { return true }
	r.finishOperation(r.beginOperation(), current, context.DeadlineExceeded)
	r.finishOperation(r.beginOperation(), current, context.DeadlineExceeded)
	r.finishOperation(r.beginOperation(), current, nil)
	data, err := os.ReadFile(filepath.Join(directory, "logs", "host.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "operation-failed") != 1 || strings.Count(string(data), "operation-recovered") != 1 {
		t.Fatalf("diagnostics duplicated or omitted: %s", data)
	}
}

func TestSelectionOldOrCancelledOperationsCannotOverwriteNewerSuccess(t *testing.T) {
	r := &selectionRuntime{}
	current := func() bool { return true }
	old := r.beginOperation()
	r.finishOperation(r.beginOperation(), current, nil)
	r.finishOperation(old, current, context.DeadlineExceeded)
	if r.status().Warning != "" {
		t.Fatal("an old timeout overwrote a new success")
	}
	r.finishOperation(r.beginOperation(), func() bool { return false }, errors.New("obsolete gesture"))
	if r.status().Warning != "" {
		t.Fatal("obsolete gesture became a warning")
	}
	r.finishOperation(r.beginOperation(), current, context.Canceled)
	if r.status().Warning != "" {
		t.Fatal("normal cancellation became a warning")
	}
	r.finishOperation(r.beginOperation(), current, errors.New("reader unavailable"))
	r.finishOperation(r.beginOperation(), current, context.Canceled)
	if !strings.Contains(r.status().Warning, "reader unavailable") {
		t.Fatal("cancellation claimed a previous error recovered")
	}
}

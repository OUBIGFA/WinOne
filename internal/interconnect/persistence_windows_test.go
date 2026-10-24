package interconnect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Exercise a real filesystem write failure, not a mocked success path. The
// result remains completed and a later refresh saves the retained record.
func TestHistoryWriteFailureRetainsResultAndRetries(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "_temp", "interconnect-manager-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "persistence-")
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewTransfers(filepath.Join(dir, "unused.exe"), dir)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	close(done)
	m.jobs["test"] = &transferJob{record: transferRecord{ID: "test", Created: time.Now(), Spec: map[string]any{"mode": "store_send"}, State: map[string]any{"status": "completed", "result": map[string]any{"code": "sample", "uploadToken": "private-receipt"}}}, done: done}
	blocked := filepath.Join(dir, "tasks.json")
	if err = os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err = m.persist(); err == nil {
		t.Fatal("expected a persistence failure")
	}
	read := func() []transferRecord {
		t.Helper()
		raw, err := m.Call(context.Background(), "transfer_list", nil)
		if err != nil {
			t.Fatal(err)
		}
		var records []transferRecord
		if err = json.Unmarshal(raw, &records); err != nil {
			t.Fatal(err)
		}
		return records
	}
	records := read()
	if records[0].State["status"] != "completed" || records[0].State["persistenceError"] == nil {
		t.Fatalf("incorrect state: %+v", records)
	}
	if records[0].State["result"].(map[string]any)["uploadToken"] != nil {
		t.Fatal("receipt leaked")
	}
	if err = os.Rename(blocked, filepath.Join(dir, "blocked-target")); err != nil {
		t.Fatal(err)
	}
	if records = read(); records[0].State["persistenceError"] != nil {
		t.Fatal("retry did not clear the error")
	}
	restored, err := NewTransfers(filepath.Join(dir, "unused.exe"), dir)
	if err != nil {
		t.Fatal(err)
	}
	result := restored.jobs["test"].record.State["result"].(map[string]any)
	if result["uploadToken"] != "private-receipt" {
		t.Fatal("receipt was not preserved on disk")
	}
}

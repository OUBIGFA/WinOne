//go:build windows

package interconnect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGofileReceiptRecoveryAndPublicRedaction(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "_temp", "interconnect-manager-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "cloud-")
	if err != nil {
		t.Fatal(err)
	}
	taskDir := filepath.Join(dir, "tasks", "test-task")
	if err = os.MkdirAll(taskDir, 0700); err != nil {
		t.Fatal(err)
	}
	receipt := []byte(`{"service":"gofile","id":"folder-id","uploadToken":"private-revocation-token"}`)
	if err = os.WriteFile(filepath.Join(taskDir, "gofile-receipt.json"), receipt, 0600); err != nil {
		t.Fatal(err)
	}
	record := transferRecord{ID: "test-task", Spec: map[string]any{"mode": "store_send", "storeOrigin": "https://gofile.io", "accountDir": "private-account-directory"}, State: map[string]any{"status": "running"}}
	data, _ := json.Marshal(map[string]any{"tasks": []transferRecord{record}})
	if err = os.WriteFile(filepath.Join(dir, "tasks.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewTransfers(filepath.Join(dir, "absent-worker.exe"), dir)
	if err != nil {
		t.Fatal(err)
	}
	job := manager.jobs[record.ID]
	if job.record.State["status"] != "interrupted" {
		t.Fatal("interrupted task was not restored")
	}
	result := job.record.State["result"].(map[string]any)
	if result["uploadToken"] != "private-revocation-token" {
		t.Fatal("receipt lost")
	}
	public, err := manager.Call(context.Background(), "transfer_list", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), "private-") {
		t.Fatal("private account or receipt leaked")
	}
	if _, err = manager.Call(context.Background(), "transfer_remove", []byte(`{"id":"test-task"}`)); err == nil {
		t.Fatal("removal discarded an active revocation receipt")
	}
	// A cancelled worker has exited already; recovery reads its durable receipt.
	job.record.State = map[string]any{"status": "cancelled"}
	if err = manager.recoverGofileReceipt(&job.record); err != nil {
		t.Fatal(err)
	}
	if job.record.State["result"].(map[string]any)["uploadToken"] == nil {
		t.Fatal("cancelled receipt lost")
	}
	job.record.State = map[string]any{"status": "completed", "result": map[string]any{"revoked": true}}
	if err = manager.recoverGofileReceipt(&job.record); err != nil {
		t.Fatal(err)
	}
	if job.record.State["result"].(map[string]any)["uploadToken"] != nil {
		t.Fatal("revoked receipt resurrected")
	}
}

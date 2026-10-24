//go:build windows

package interconnect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTransferManagerPersistsAndReleases(t *testing.T) {
	exe := os.Getenv("WINONE_TRANSFER_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_TRANSFER_TEST_EXE to the built adapter")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "_temp", "interconnect-manager-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.MkdirTemp(root, "transfer-")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewTransfers(exe, data)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	// An invalid share is rejected by croc asynchronously, and still remains a
	// visible failed task after its worker exits and the manager is reopened.
	params, _ := json.Marshal(map[string]any{"mode": "store_inspect", "code": "invalid-share"})
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if _, err = manager.Call(ctx, "transfer_start", params); err != nil {
		t.Fatal(err)
	}
	var list []struct {
		ID    string
		State struct{ Status string }
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := manager.Call(ctx, "transfer_list", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &list); err != nil {
			t.Fatal(err)
		}
		if len(list) == 1 && list[0].State.Status == "failed" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if len(list) != 1 || list[0].State.Status != "failed" {
		t.Fatalf("failure was not visible: %+v", list)
	}
	if err = manager.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := NewTransfers(exe, data)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	raw, err := restored.Call(ctx, "transfer_list", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &list); err != nil || len(list) != 1 || list[0].State.Status != "failed" {
		t.Fatalf("task history lost: %s %v", raw, err)
	}
}

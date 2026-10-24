//go:build windows

package interconnect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSavePreferencesWithoutStartingShare(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "_temp", "interconnect-preferences-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.MkdirTemp(root, "run-")
	if err != nil {
		t.Fatal(err)
	}
	// A nonexistent executable proves saving never launches the LAN worker.
	exe := filepath.Join(data, "not-installed.exe")
	manager, err := New(exe, filepath.Join(data, "lan"))
	if err != nil {
		t.Fatal(err)
	}
	want := Preferences{Bind: "127.0.0.1", Port: 0, DownloadDir: filepath.Join(data, "received")}
	payload, _ := json.Marshal(want)
	reply, err := manager.Call(context.Background(), "preferences.save", payload)
	if err != nil {
		t.Fatal(err)
	}
	var got Preferences
	if err = json.Unmarshal(reply, &got); err != nil || got != want || manager.process != nil {
		t.Fatalf("save changed runtime or response: %s, %v", reply, err)
	}
	restored, err := New(exe, filepath.Join(data, "lan"))
	if err != nil || restored.preferences != want {
		t.Fatalf("saved preferences not restored: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(manager.data, "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		`{"bind":"::1","port":5421,"downloadDir":"C:/received"}`,
		`{"bind":"127.0.0.1","port":5421,"downloadDir":"relative"}`,
		`{"bind":"127.0.0.1","port":65536,"downloadDir":"C:/received"}`,
		`{"bind":"127.0.0.1","port":-1,"downloadDir":"C:/received"}`,
	} {
		if _, err = manager.Call(context.Background(), "preferences.save", json.RawMessage(invalid)); err == nil {
			t.Fatalf("invalid input accepted: %s", invalid)
		}
	}
	manager.process = &worker{}
	if _, err = manager.Call(context.Background(), "preferences.save", payload); err == nil {
		t.Fatal("active share preferences changed")
	}
	manager.process = nil
	after, err := os.ReadFile(filepath.Join(manager.data, "preferences.json"))
	if err != nil || string(before) != string(after) || manager.preferences != want {
		t.Fatal("rejected save changed persisted settings")
	}
	blocked := filepath.Join(data, "blocked")
	if err = os.WriteFile(blocked, []byte("file blocks directory"), 0600); err != nil {
		t.Fatal(err)
	}
	manager.data = blocked
	changed := want
	changed.Port = 5422
	payload, _ = json.Marshal(changed)
	if _, err = manager.Call(context.Background(), "preferences.save", payload); err == nil || manager.preferences != want {
		t.Fatal("failed write reported success or replaced saved settings")
	}
}

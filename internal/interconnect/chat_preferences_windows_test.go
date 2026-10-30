//go:build windows

package interconnect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatNicknamePersistsIndependently(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "_temp", "interconnect-preferences-tests"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.MkdirTemp(root, "nickname-")
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{data: filepath.Join(data, "lan")}
	initial, err := manager.ChatPreferences()
	if err != nil || initial.Nickname != "" {
		t.Fatalf("initial: %+v, %v", initial, err)
	}
	for _, name := range []string{" 小明 ", "Alice", "另一个昵称"} {
		saved, err := manager.SaveChatPreferences(ChatPreferences{Nickname: name})
		if err != nil || saved.Nickname != strings.TrimSpace(name) {
			t.Fatalf("save: %+v, %v", saved, err)
		}
		reopened := &Manager{data: manager.data}
		got, err := reopened.ChatPreferences()
		if err != nil || got != saved {
			t.Fatalf("reopen: %+v, %v", got, err)
		}
	}
	for _, name := range []string{strings.Repeat("名", 81), "first\nsecond", "a\x00b"} {
		if _, err = manager.SaveChatPreferences(ChatPreferences{Nickname: name}); err == nil {
			t.Fatal("invalid nickname accepted")
		}
	}
	got, err := manager.ChatPreferences()
	if err != nil || got.Nickname != "另一个昵称" {
		t.Fatalf("invalid save overwrote previous value: %+v, %v", got, err)
	}
	if err = os.WriteFile(filepath.Join(data, "chat.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.ChatPreferences(); err == nil {
		t.Fatal("corrupt preferences silently reset")
	}
}

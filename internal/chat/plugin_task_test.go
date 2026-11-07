package chat

import (
	"crypto/rand"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestPluginTaskCreationKeepsHiddenIdentityAndConfigRevision(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "profile")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	prepared := Conversation{ID: rand.Text(), PluginID: "fixture", Hidden: true, Title: "Plugin task", Permission: "acceptEdits", Model: ai.Selection{Provider: "fixture", Model: "model"}}
	task, err := store.NewPluginConversation(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if task.Revision != 1 || !task.Hidden || task.PluginID != "fixture" || len(store.VisibleSnapshot().Conversations) != 0 {
		t.Fatalf("creation: %+v", task)
	}
	// Activity/history writes are not model or permission configuration changes.
	task, err = store.UpdateConversation(task.ID, func(c *Conversation) error {
		c.HasMessages = true
		c.SessionFile = filepath.Join(c.SessionDirectory, "session.jsonl")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Revision != 1 {
		t.Fatalf("history changed config revision: %d", task.Revision)
	}
	task, err = store.UpdateConversation(task.ID, func(c *Conversation) error { c.Hidden = false; c.Model.Model = "another"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if task.Revision != 2 || len(store.VisibleSnapshot().Conversations) != 1 {
		t.Fatalf("config update: %+v", task)
	}
	reopened, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	read, err := reopened.Conversation(task.ID)
	if err != nil || read.PluginID != task.PluginID || read.Revision != 2 || read.Model.Model != "another" {
		t.Fatalf("reopen: %+v, %v", read, err)
	}
	if _, err = store.NewPluginConversation(prepared); err == nil {
		t.Fatal("replaced an existing task")
	}
}

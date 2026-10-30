//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/chat"
)

func TestOneBotHistorySearchUsesHostOwnedSessionScope(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_ONEBOT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit isolated runtime paths are required")
	}
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runner, err := chat.StartRunner(ctx, bun, worker, directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := chat.Open(filepath.Join(directory, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	control := newChatOneBot(&chatControl{ctx: ctx, directory: directory})
	control.runner, control.sessions = runner, &chatControl{store: store}
	defer control.close(context.Background())
	if _, err = runner.Call(ctx, "init", map[string]any{"directory": directory}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"reader", "other"} {
		if _, err = runner.Call(ctx, "profiles.create", map[string]any{"input": map[string]string{"id": id, "name": id}}); err != nil {
			t.Fatal(err)
		}
	}
	ids := []string{}
	for _, fixture := range []struct{ botID, role, marker string }{{"reader", "canonical", "VISIBLE_RECEIPT"}, {"reader", "channel", "HIDDEN_CHANNEL"}, {"other", "canonical", "OTHER_BOT"}, {"reader", "history", "VISIBLE_ARCHIVE"}} {
		conversation, err := store.NewConversation("", directory, fixture.marker, ai.Selection{})
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(conversation.SessionDirectory, "history.jsonl")
		output, err := os.Create(file)
		if err != nil {
			t.Fatal(err)
		}
		encoder := json.NewEncoder(output)
		for _, entry := range []any{
			map[string]any{"type": "session", "version": 3, "id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "cwd": directory, "timestamp": "2026-10-26T08:00:00Z"},
			map[string]any{"type": "message", "id": "entry1", "parentId": nil, "timestamp": "2026-10-26T08:00:01Z", "message": map[string]any{"role": "user", "content": fixture.marker + " receipt", "timestamp": 1793001601000}},
		} {
			if err := encoder.Encode(entry); err != nil {
				t.Fatal(err)
			}
		}
		if err = output.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err = store.UpdateConversation(conversation.ID, func(value *chat.Conversation) error {
			value.SessionFile = file
			value.HasMessages = true
			value.Archived = fixture.role == "history"
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err = runner.Call(ctx, "sessions.bind", map[string]string{"botId": fixture.botID, "sessionId": conversation.ID, "role": fixture.role, "routeKey": fixture.role}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, conversation.ID)
	}
	// Settings receive the durable Chat titles and timestamps, not bare Bot link
	// rows. A list reload must preserve the same history as the detail response.
	for _, method := range []string{"profiles.get", "profiles.list"} {
		raw, err := control.call(ctx, method, json.RawMessage(`{"botId":"reader"}`))
		if err != nil {
			t.Fatal(err)
		}
		var profiles []struct {
			ID       string `json:"id"`
			Sessions []struct {
				ID        string `json:"id"`
				Title     string `json:"title"`
				Kind      string `json:"kind"`
				Role      string `json:"role"`
				UpdatedAt int64  `json:"updatedAt"`
			} `json:"sessions"`
		}
		if method == "profiles.get" {
			raw = append(append([]byte{'['}, raw...), ']')
		}
		if err := json.Unmarshal(raw, &profiles); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, profile := range profiles {
			if profile.ID != "reader" {
				continue
			}
			if len(profile.Sessions) != 3 {
				t.Fatalf("%s omitted owned sessions: %s", method, raw)
			}
			for _, session := range profile.Sessions {
				if session.ID == ids[2] {
					t.Fatal("another Bot leaked into profile")
				}
				if session.ID == ids[3] {
					found = session.Title == "VISIBLE_ARCHIVE" && session.Kind == "history" && session.Role == "history" && session.UpdatedAt > 0
				}
			}
		}
		if !found {
			t.Fatalf("%s has no usable history projection: %s", method, raw)
		}
	}
	// Unknown input fields must not expand the Host-owned scope or choose files.
	input, _ := json.Marshal(map[string]any{"botId": "reader", "query": "receipt", "limit": 20, "sessionIds": ids, "sessions": []map[string]string{{"id": ids[2], "sessionFile": "untrusted-path"}}})
	result, err := control.call(ctx, "history.search", input)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Results []struct {
			Session struct {
				ID string `json:"id"`
			} `json:"session"`
		} `json:"results"`
	}
	if err = json.Unmarshal(result, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 2 || strings.Contains(string(result), "HIDDEN_CHANNEL") || strings.Contains(string(result), "OTHER_BOT") {
		t.Fatalf("search escaped its scope: %s", result)
	}
	if _, err = control.call(ctx, "history.query", input); err == nil {
		t.Fatal("renderer invoked private history query")
	}
	if err = control.close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = control.call(ctx, "history.search", input); err == nil {
		t.Fatal("closed Host restarted for search")
	}
}

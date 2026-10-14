//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
)

func TestTurnReviewAggregatesSnapshotsAndProtectsUndoFromLaterEdits(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", "", "Captured", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(c.Directory, "report.md")
	if err = os.WriteFile(target, []byte("final\n"), 0600); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolvePreviewPath(target)
	if err != nil {
		t.Fatal(err)
	}
	entries := []map[string]any{
		{"id": "receipt", "type": "custom", "customType": "winone.input", "data": map[string]string{"id": "turn"}},
		{"id": "user", "type": "message", "message": map[string]any{"role": "user", "content": "Edit"}},
	}
	for index, version := range [][2]string{{"old\n", "middle\n"}, {"middle\n", "final\n"}} {
		key := []string{"first", "last"}[index]
		entries = append(entries, map[string]any{"id": key, "type": "message", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "toolCall", "id": key, "name": "write", "arguments": map[string]string{"path": target}}}}})
		entries = append(entries, map[string]any{"id": key + "-result", "type": "message", "message": map[string]any{"role": "toolResult", "toolCallId": key, "details": map[string]any{"files": []any{map[string]any{"path": target, "resolvedPath": resolved, "status": "M", "patch": "recorded patch", "captured": true, "beforeExists": true, "beforeText": version[0], "afterText": version[1]}}}}})
	}
	var body strings.Builder
	parent := ""
	for _, entry := range entries {
		entry["parentId"] = parent
		parent = entry["id"].(string)
		raw, _ := json.Marshal(entry)
		body.Write(raw)
		body.WriteByte('\n')
	}
	os.MkdirAll(c.SessionDirectory, 0700)
	session := filepath.Join(c.SessionDirectory, "session.jsonl")
	if err = os.WriteFile(session, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateConversation(c.ID, func(next *Conversation) error { next.SessionFile = session; return nil }); err != nil {
		t.Fatal(err)
	}
	snapshot, patches, err := store.TurnReview(c.ID, "turn")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patches["report.md"], "-old") || !strings.Contains(patches["report.md"], "+final") || strings.Contains(patches["report.md"], "middle") {
		t.Fatal("turn diff must compare first before-image to final after-image", patches)
	}
	undo, ok := any(store).(interface {
		ApplyTurnReview(context.Context, string, string, string, string, func(string, func() error) error) (ReviewSnapshot, error)
	})
	if !ok {
		t.Fatal("turn undo/reapply is missing")
	}
	if _, err = undo.ApplyTurnReview(context.Background(), c.ID, "turn", snapshot.Revision, "undo", nil); err != nil {
		t.Fatal(err)
	}
	if contents, _ := os.ReadFile(target); string(contents) != "old\n" {
		t.Fatal("undo did not restore the original", string(contents))
	}
	if _, err = undo.ApplyTurnReview(context.Background(), c.ID, "turn", snapshot.Revision, "reapply", nil); err != nil {
		t.Fatal(err)
	}
	if contents, _ := os.ReadFile(target); string(contents) != "final\n" {
		t.Fatal("reapply did not restore the final version")
	}
	if err = os.WriteFile(target, []byte("manual edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = undo.ApplyTurnReview(context.Background(), c.ID, "turn", snapshot.Revision, "undo", nil); err == nil {
		t.Fatal("undo overwrote a later manual edit")
	}
	if contents, _ := os.ReadFile(target); string(contents) != "manual edit\n" {
		t.Fatal("conflict modified the file")
	}
}

func TestTurnReviewKeepsEachTurnsOwnFileChanges(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", "", "Turns", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	parent := ""
	appendEntry := func(entry map[string]any) {
		entry["parentId"] = parent
		parent = entry["id"].(string)
		raw, _ := json.Marshal(entry)
		body.Write(raw)
		body.WriteByte('\n')
	}
	for _, turn := range []string{"first", "second"} {
		appendEntry(map[string]any{"type": "custom", "id": turn + "-receipt", "customType": "winone.input", "data": map[string]string{"id": turn}})
		appendEntry(map[string]any{"type": "message", "id": turn + "-user", "message": map[string]any{"role": "user", "content": turn}})
		appendEntry(map[string]any{"type": "message", "id": turn + "-call", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "toolCall", "id": turn + "-write", "name": "write", "arguments": map[string]string{"path": "report.md"}}}}})
		appendEntry(map[string]any{"type": "message", "id": turn + "-result", "message": map[string]any{"role": "toolResult", "toolCallId": turn + "-write", "details": map[string]any{"files": []any{map[string]string{"path": "report.md", "status": "M", "patch": turn + " patch"}}}}})
	}
	os.MkdirAll(c.SessionDirectory, 0700)
	session := filepath.Join(c.SessionDirectory, "session.jsonl")
	if err = os.WriteFile(session, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateConversation(c.ID, func(next *Conversation) error { next.SessionFile = session; return nil }); err != nil {
		t.Fatal(err)
	}
	reader, ok := any(store).(interface {
		TurnReview(string, string) (ReviewSnapshot, map[string]string, error)
	})
	if !ok {
		t.Fatal("turn-specific file review is missing")
	}
	for _, turn := range []string{"first", "second"} {
		snapshot, patches, err := reader.TurnReview(c.ID, turn)
		if err != nil || len(snapshot.Files) != 1 || patches["report.md"] != turn+" patch" {
			t.Fatalf("turn %s: %+v %+v %v", turn, snapshot, patches, err)
		}
	}
	if _, _, err = reader.TurnReview(c.ID, "not-a-turn"); err == nil {
		t.Fatal("unknown turn accepted")
	}
}

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

	"winone/internal/chat"
)

func TestOneBotProjectsUseHostCatalogAndPublishChanges(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_ONEBOT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit isolated runtime paths are required")
	}
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := chat.Open(filepath.Join(directory, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(directory, "registered")
	if err = os.Mkdir(projectPath, 0700); err != nil {
		t.Fatal(err)
	}
	project, err := store.AddProject(projectPath, "Registered project")
	if err != nil {
		t.Fatal(err)
	}
	control := newChatOneBot(&chatControl{ctx: ctx, directory: directory, store: store})
	runner, err := chat.StartRunner(ctx, bun, worker, directory, control.event)
	if err != nil {
		t.Fatal(err)
	}
	control.runner = runner
	defer control.close(context.Background())
	if _, err = runner.Call(ctx, "init", map[string]any{"directory": directory}); err != nil {
		t.Fatal(err)
	}
	call := func(method string, input any) json.RawMessage {
		t.Helper()
		params, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := control.call(ctx, method, params)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	call("profiles.create", map[string]any{"input": map[string]string{"id": "reader", "name": "Reader"}})
	type option struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Path      string `json:"path"`
		Linked    bool   `json:"linked"`
		Available bool   `json:"available"`
	}
	var options []option
	// Renderer-supplied folders are ignored even if they imitate the private
	// snapshot. Only the registered WinOne catalog reaches the original service.
	spoof := map[string]any{"botId": "reader", "catalog": []map[string]any{{"path": directory, "name": "Spoofed", "exists": true}}}
	if err = json.Unmarshal(call("projects.list", spoof), &options); err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].Name != "Registered project" || options[0].Path != projectPath || options[0].Linked || !options[0].Available {
		t.Fatalf("wrong project catalog: %+v", options)
	}
	id := options[0].ID
	var initial struct {
		Cursor uint64 `json:"cursor"`
	}
	if err = json.Unmarshal(call("events", map[string]uint64{"cursor": 9007199254740991}), &initial); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(call("projects.set", map[string]any{"botId": "reader", "id": id, "linked": true}), &options); err != nil {
		t.Fatal(err)
	}
	if !options[0].Linked {
		t.Fatal("project link was not saved")
	}
	var page struct {
		Events []struct {
			Type  string `json:"type"`
			BotID string `json:"botId"`
		} `json:"events"`
	}
	if err = json.Unmarshal(call("events", map[string]uint64{"cursor": initial.Cursor}), &page); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range page.Events {
		found = found || event.Type == "workbench-changed" && event.BotID == "reader"
	}
	if !found {
		t.Fatalf("missing workbench notification: %+v", page)
	}
	if err = store.RemoveProject(project.ID); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(projectPath, projectPath+"-offline"); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(call("projects.list", map[string]string{"botId": "reader"}), &options); err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].ID != id || !options[0].Linked || options[0].Available {
		t.Fatalf("lost unavailable reference: %+v", options)
	}
	if err = json.Unmarshal(call("projects.set", map[string]any{"botId": "reader", "id": id, "linked": false}), &options); err != nil {
		t.Fatal(err)
	}
	if len(options) != 0 {
		t.Fatalf("reference remains after unlink: %+v", options)
	}
	for _, method := range []string{"projects.query", "projects.link"} {
		if _, err = control.call(ctx, method, json.RawMessage(`{"botId":"reader","catalog":[]}`)); err == nil {
			t.Fatalf("accepted private method %s", method)
		}
	}
	if err = control.close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = control.call(ctx, "projects.list", json.RawMessage(`{"botId":"reader"}`)); err == nil {
		t.Fatal("closed project service restarted")
	}
}

func TestOneBotEventCursorGapRequiresReloadWithoutLeakingWorkerDetails(t *testing.T) {
	control := newChatOneBot(&chatControl{})
	for i := 0; i < 300; i++ {
		control.event(json.RawMessage(`{"type":"onebot_event","event":{"type":"workbench-changed","botId":"reader","detail":"private contents"}}`))
	}
	raw, err := control.waitEvents(context.Background(), nil, json.RawMessage(`{"cursor":1}`))
	if err != nil {
		t.Fatal(err)
	}
	var page oneBotEventPage
	if err = json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	if !page.Reset || page.Cursor != 300 || len(page.Events) == 0 {
		t.Fatalf("dropped invalidations were not reported: %s", raw)
	}
	if strings.Contains(string(raw), "private contents") || strings.Contains(string(raw), "detail") {
		t.Fatal("private Worker detail escaped through invalidations")
	}
}

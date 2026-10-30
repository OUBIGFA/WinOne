//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"winone/internal/ai"
	"winone/internal/chat"
)

// The follow-up reads conversation facts from both Chat stores without
// starting a runtime; unknown ids are simply absent.
func TestOneBotAutonomyBridgeReadsBothStores(t *testing.T) {
	directory := t.TempDir()
	outer, err := chat.Open(filepath.Join(directory, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	inner, err := chat.Open(filepath.Join(directory, "onebot", "chat"))
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := outer.NewConversation("", directory, "Ordinary task", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = outer.UpdateConversation(ordinary.ID, func(c *chat.Conversation) error { c.Archived = true; return nil }); err != nil {
		t.Fatal(err)
	}
	canonical, err := inner.NewConversation("", directory, "Teammate chat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bot := newChatOneBot(&chatControl{ctx: ctx, directory: directory, store: outer, engine: chat.NewService(ctx, outer, chat.ServiceOptions{})})
	bot.sessions = &chatControl{store: inner, engine: chat.NewService(ctx, inner, chat.ServiceOptions{})}

	post := func(route string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		recorder := httptest.NewRecorder()
		bot.serveOneBotBridge(recorder, httptest.NewRequest(http.MethodPost, "/v1/onebot/"+route, bytes.NewReader(raw)), route, raw)
		return recorder
	}
	response := post("autonomy-sessions", map[string]any{"ids": []string{ordinary.ID, canonical.ID, "missing"}})
	var sessions struct {
		Sessions []struct {
			ID, Title, WorkingDir, Store string
			Archived                     bool
		}
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &sessions) != nil || len(sessions.Sessions) != 2 {
		t.Fatalf("session metadata: %d %s", response.Code, response.Body)
	}
	first, second := sessions.Sessions[0], sessions.Sessions[1]
	if first.ID != ordinary.ID || first.Store != "chat" || !first.Archived || first.Title != "Ordinary task" || first.WorkingDir != directory {
		t.Fatalf("ordinary conversation: %+v", first)
	}
	if second.ID != canonical.ID || second.Store != "onebot" || second.Archived {
		t.Fatalf("teammate conversation: %+v", second)
	}
	if response = post("autonomy-sessions", map[string]any{"ids": make([]string, maximumAutonomySessionIDs+1)}); response.Code != 400 {
		t.Fatalf("unbounded metadata read: %d", response.Code)
	}

	// Nothing runs, so the full activity read is an empty list, never null.
	if response = post("autonomy-activity", map[string]any{}); response.Code != 200 || strings.TrimSpace(response.Body.String()) != `{"snapshots":[]}` {
		t.Fatalf("activity read: %d %s", response.Code, response.Body)
	}
}

func TestOneBotAutonomyReminderUsesTray(t *testing.T) {
	language := "zh-CN"
	bot := newChatOneBot(&chatControl{language: func() string { return language }})
	var title, text string
	bot.remindNotify = func(heading, body string) error { title, text = heading, body; return nil }
	raw, _ := json.Marshal(map[string]string{"sessionId": "s", "title": "整理报表", "detail": "要覆盖旧文件吗？"})
	if _, err := bot.remindOwner(raw); err != nil {
		t.Fatal(err)
	}
	if title != "WinOne · 任务在等你回复" || text != "整理报表\n要覆盖旧文件吗？" {
		t.Fatalf("reminder: %q %q", title, text)
	}
	language = "en-US"
	raw, _ = json.Marshal(map[string]string{"sessionId": "s", "detail": strings.Repeat("x", 2000)})
	if _, err := bot.remindOwner(raw); err != nil {
		t.Fatal(err)
	}
	if title != "WinOne · A task is waiting for you" || !strings.HasPrefix(text, "Untitled conversation\n") || len([]rune(text)) != 1000 {
		t.Fatalf("english reminder: %q %d", title, len([]rune(text)))
	}
}

// Routines of a teammate that is not paused start the Worker after launch, so
// scheduled routines and follow-up keep running without opening OneBot.
func TestOneBotSavedRoutinesStartTheWorker(t *testing.T) {
	directory := t.TempDir()
	bot := newChatOneBot(&chatControl{directory: directory})
	if active, err := bot.savedRoutinesActive(); active || err != nil {
		t.Fatalf("missing routines: %v %v", active, err)
	}
	file := filepath.Join(directory, "onebot", "routines", "routines.json")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		state  string
		active bool
	}{
		{`{"version":1,"routines":[],"runs":[],"receipts":{},"next":{}}`, false},
		{`{"version":1,"routines":[{"id":"r","botId":"paused"}],"pausedBotIds":["paused"]}`, false},
		{`{"version":1,"routines":[{"id":"r","botId":"paused"},{"id":"n","botId":"active"}],"pausedBotIds":["paused"]}`, true},
	} {
		if err := os.WriteFile(file, []byte(test.state), 0600); err != nil {
			t.Fatal(err)
		}
		if active, err := bot.savedRoutinesActive(); active != test.active || err != nil {
			t.Fatalf("%s: %v %v", test.state, active, err)
		}
	}
	if err := os.WriteFile(file, []byte(`{`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.savedRoutinesActive(); err == nil {
		t.Fatal("accepted a damaged routine record")
	}
}

// Activity changes collapse into one pending set and never start the Worker.
func TestOneBotActivityPushNeedsRunningWorker(t *testing.T) {
	bot := newChatOneBot(&chatControl{ctx: context.Background()})
	bot.noteActivity("a")
	bot.noteActivity("b")
	bot.noteActivity("a")
	bot.activityMu.Lock()
	pending, scheduled := len(bot.activityPending), bot.activityScheduled
	bot.activityMu.Unlock()
	if pending != 2 || !scheduled {
		t.Fatalf("coalesced activity: %d %v", pending, scheduled)
	}
	bot.flushActivity()
	bot.activityMu.Lock()
	defer bot.activityMu.Unlock()
	if len(bot.activityPending) != 0 || bot.activityScheduled || bot.runner != nil {
		t.Fatal("activity push started the Worker or kept stale ids")
	}
}

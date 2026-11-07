package pluginhost

import (
	"encoding/json"
	"testing"

	"winone/desktopmygo/testhost"
)

func TestInboxRetractsExpiredPromptsAndPreservesOnlyLiveAnswers(t *testing.T) {
	env := testhost.New(t, func(string, json.RawMessage) (any, error) { return true, nil }).Env
	h := For(env)
	defer h.Close()
	raw := json.RawMessage(`{"kind":"confirm","ghostName":"Fixture","body":"Continue?"}`)
	h.apply(1, nil, []*prompt{{ID: "old", Request: raw}})
	first := h.prompts[0]
	first.value = "draft"
	h.apply(2, nil, []*prompt{{ID: "old", Request: raw}, {ID: "next", Request: raw}})
	if len(h.prompts) != 2 || h.prompts[0] != first || h.prompts[0].value != "draft" {
		t.Fatal("poll replaced a live prompt's draft")
	}
	h.apply(3, nil, []*prompt{{ID: "next", Request: raw}})
	if len(h.prompts) != 1 || h.prompts[0].ID != "next" || h.seen["old"] {
		t.Fatal("expired request still asks for permission or retains state")
	}
	h.apply(4, nil, nil)
	if len(h.prompts) != 0 || len(h.seen) != 0 {
		t.Fatal("settled requests are retained")
	}
}

func TestPluginPushesKeepDraftsAndTaskViewsInTheirActualDestination(t *testing.T) {
	env := testhost.New(t, func(string, json.RawMessage) (any, error) { return true, nil }).Env
	h := For(env)
	defer h.Close()
	draft := json.RawMessage(`{"requestId":"draft","name":"Reminder","prompt":"Check","intervalMs":3600000}`)
	h.handle(event{Channel: "schedule-draft", Payload: draft})
	intents := env.TakeIntents("chat")
	if len(intents) != 1 || intents[0].NewChat || string(intents[0].AutomationDraft) != string(draft) {
		t.Fatalf("draft became a new chat: %+v", intents)
	}
	h.handle(event{Channel: "task-view", Payload: json.RawMessage(`{"instanceId":"plugin","sessionId":"task-a"}`)})
	if len(h.TakeTaskViews("task-b")) != 0 || h.pages.HasMainPage() {
		t.Fatal("task view leaked into the active/global page")
	}
	views := h.TakeTaskViews("task-a")
	if len(views) != 1 || views[0] != "plugin" || len(h.TakeTaskViews("task-a")) != 0 {
		t.Fatalf("task-scoped request: %v", views)
	}
}

func TestCompanionRevealKeepsTheOneBotConversationOwner(t *testing.T) {
	env := testhost.New(t, func(string, json.RawMessage) (any, error) { return nil, nil }).Env
	h := For(env)
	defer h.Close()
	h.handle(event{Channel: "reveal-session", Payload: json.RawMessage(`{"sessionId":"owned-session","botId":"partner"}`)})
	intents := env.TakeIntents("onebot")
	if len(intents) != 1 || len(env.TakeIntents("chat")) != 0 {
		t.Fatalf("companion was routed into ordinary Chat: %+v", intents)
	}
	var target struct{ BotID, SessionID string }
	if err := json.Unmarshal(intents[0].Options, &target); err != nil || target.BotID != "partner" || target.SessionID != "owned-session" {
		t.Fatalf("conversation binding lost: %+v %v", target, err)
	}
}

func TestClosingPluginUIReleasesRegistryAndStopsPolling(t *testing.T) {
	env := testhost.New(t, func(string, json.RawMessage) (any, error) { return nil, nil }).Env
	h := For(env)
	h.Close()
	h.Close()
	select {
	case <-h.stop:
	default:
		t.Fatal("poll was not stopped")
	}
	h.apply(1, nil, []*prompt{{ID: "late", Request: json.RawMessage(`{"kind":"confirm"}`)}})
	if len(h.prompts) != 0 {
		t.Fatal("late poll revived a closed UI")
	}
	next := For(env)
	defer next.Close()
	if next == h {
		t.Fatal("new window reused a closed host")
	}
}

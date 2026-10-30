//go:build windows

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"winone/internal/ai"
	"winone/internal/chat"
)

func TestOneBotModelFallbackAcknowledgementRejectsUnrelatedAndStaleChoices(t *testing.T) {
	directory := t.TempDir()
	service := newAIService(context.Background(), directory, "")
	var err error
	service.store, err = ai.Open(service.storePath)
	if err != nil {
		t.Fatal(err)
	}
	configuration := service.store.Configuration()
	configuration.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.test/v1", Models: []string{"first", "second", "unrelated"}}}
	configuration.Tasks["chat"] = ai.Selection{Provider: "fixture", Model: "first"}
	if _, err = service.store.SaveConfiguration(configuration, nil); err != nil {
		t.Fatal(err)
	}
	store, err := chat.Open(filepath.Join(directory, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", directory, "Model fallback", ai.Selection{Provider: "fixture", Model: "first"}, directory)
	if err != nil {
		t.Fatal(err)
	}
	var active oneBotContext
	if err = json.Unmarshal([]byte(`{"capabilities":{"modelChainOverride":[{"harness":"pi","providerId":"fixture","model":"first","effort":"low","fastMode":false},{"harness":"pi","providerId":"fixture","model":"second","effort":"high","fastMode":true}]}}`), &active); err != nil {
		t.Fatal(err)
	}
	plan, err := resolveOneBotModel(active, configuration, conversation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateConversation(conversation.ID, func(c *chat.Conversation) error { c.OneBotModelKey = plan.Key; return nil }); err != nil {
		t.Fatal(err)
	}
	bot := newChatOneBot(&chatControl{ai: service})
	bot.sessions = &chatControl{store: store}
	for _, request := range []struct{ previous, selected, key string }{
		{"fixture/first", "fixture/unrelated", plan.Key},
		{"fixture/first", "fixture/first", plan.Key},
		{"fixture/first", "fixture/second", "stale-chain"},
		{"fixture/unrelated", "fixture/second", plan.Key},
	} {
		if err = bot.runtimeModelApplied(conversation.ID, request.previous, request.selected, request.key, active); err == nil {
			t.Fatalf("accepted invalid fallback: %+v", request)
		}
		unchanged, err := store.Conversation(conversation.ID)
		if err != nil || unchanged.Model.Model != "first" {
			t.Fatalf("rejected fallback changed current model: %+v %v", unchanged, err)
		}
	}
	if err = bot.runtimeModelApplied(conversation.ID, "fixture/first", "fixture/second", plan.Key, active); err != nil {
		t.Fatal(err)
	}
	selected, err := store.Conversation(conversation.ID)
	if err != nil || selected.Model.Model != "second" || selected.ThinkingLevel != "high" || !selected.FastMode {
		t.Fatalf("fallback lost route parameters: %+v %v", selected, err)
	}
	if _, err = store.UpdateConversation(conversation.ID, func(c *chat.Conversation) error { c.OneBotModelKey = "new-user-choice"; return nil }); err != nil {
		t.Fatal(err)
	}
	if err = bot.runtimeModelApplied(conversation.ID, "fixture/second", "fixture/first", plan.Key, active); err == nil {
		t.Fatal("late acknowledgement replaced a newer choice")
	}
}

func TestOneBotModelPlanPreservesFallbackUntilTheConfiguredChainChanges(t *testing.T) {
	configuration := ai.Configuration{Providers: []ai.Provider{{ID: "first", Models: []string{"a"}}, {ID: "second", Models: []string{"b"}}, {ID: "manual", Models: []string{"c"}}}, Tasks: map[string]ai.Selection{"default": {Provider: "first", Model: "a"}}}
	var value oneBotContext
	if err := json.Unmarshal([]byte(`{"capabilities":{"modelChainOverride":[{"harness":"pi","providerId":"first","model":"a","effort":"low","fastMode":false},{"harness":"pi","providerId":"second","model":"b","effort":"high","fastMode":true}]}}`), &value); err != nil {
		t.Fatal(err)
	}
	initial, err := resolveOneBotModel(value, configuration, chat.Conversation{})
	if err != nil {
		t.Fatal(err)
	}
	if initial.Model.Provider != "first" || initial.Model.Model != "a" || initial.ThinkingLevel != "low" || initial.FastMode {
		t.Fatalf("wrong primary: %+v", initial)
	}
	current := chat.Conversation{Model: ai.Selection{Provider: "second", Model: "b"}, ThinkingLevel: "high", FastMode: true, OneBotModelKey: initial.Key}
	fallback, err := resolveOneBotModel(value, configuration, current)
	if err != nil || fallback.Model != current.Model || fallback.ThinkingLevel != "high" || !fallback.FastMode {
		t.Fatalf("fallback reset: %+v %v", fallback, err)
	}
	current.Model = ai.Selection{Provider: "manual", Model: "c"}
	manual, err := resolveOneBotModel(value, configuration, current)
	if err != nil || manual.Model != current.Model {
		t.Fatalf("manual selection reset: %+v %v", manual, err)
	}
	if err = json.Unmarshal([]byte(`{"capabilities":{"modelChainOverride":[{"harness":"pi","providerId":"first","model":"a","effort":"high","fastMode":false},{"harness":"pi","providerId":"second","model":"b","effort":"low","fastMode":false}]}}`), &value); err != nil {
		t.Fatal(err)
	}
	changed, err := resolveOneBotModel(value, configuration, current)
	if err != nil || changed.Model.Provider != "first" || changed.ThinkingLevel != "high" || changed.Key == initial.Key {
		t.Fatalf("new selection was not applied: %+v %v", changed, err)
	}
}

func TestOneBotModelPlanUsesOnlyDeclaredOrFollowedRoutes(t *testing.T) {
	configuration := ai.Configuration{Providers: []ai.Provider{{ID: "enabled", Models: []string{"model"}}}, Tasks: map[string]ai.Selection{"default": {Provider: "enabled", Model: "model"}}}
	var value oneBotContext
	json.Unmarshal([]byte(`{"capabilities":{"modelChainOverride":[{"harness":"pi","providerId":"gone","model":"missing"},{"harness":"pi","providerId":"enabled","model":"model","effort":"high","fastMode":true}]}}`), &value)
	plan, err := resolveOneBotModel(value, configuration, chat.Conversation{})
	if err != nil || plan.Model.Provider != "enabled" || len(plan.Chain) != 2 || !plan.FastMode {
		t.Fatalf("declared available route: %+v %v", plan, err)
	}
	json.Unmarshal([]byte(`{"capabilities":{"modelChainOverride":[{"harness":"pi","providerId":"gone","model":"missing"}]}}`), &value)
	if _, err = resolveOneBotModel(value, configuration, chat.Conversation{}); err == nil {
		t.Fatal("invented a route from the global catalog")
	}
	json.Unmarshal([]byte(`{"capabilities":{"modelChainOverride":null,"model":"obsolete","providerId":"gone"}}`), &value)
	plan, err = resolveOneBotModel(value, configuration, chat.Conversation{})
	if err != nil || plan.Model.Provider != "enabled" {
		t.Fatalf("followed default used stale mirror: %+v %v", plan, err)
	}
}

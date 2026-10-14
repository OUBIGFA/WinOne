package resultui

import (
	"encoding/json"
	"testing"
	"winone/internal/ai"
	"winone/internal/resultwindow"
)

func TestSelectionActionUsesItsModelAndCanRestoreIt(t *testing.T) {
	var initial resultwindow.Initial
	if err := json.Unmarshal([]byte(`{"id":"test","mode":"translate","source":"selection","action":"explain","actions":[{"id":"explain","kind":"ai","selection":{"provider":"work","model":"precise"}}]}`), &initial); err != nil {
		t.Fatal(err)
	}
	c := NewController(nil, initial)
	defer c.close()
	expected := ai.Selection{Provider: "work", Model: "precise"}
	if c.parameters(map[string]any{})["selection"] != expected {
		t.Fatal("action did not select its configured provider/model")
	}
	temporary := ai.Selection{Provider: "personal", Model: "fast"}
	c.Panel.Actions.Selection(&temporary)
	if c.parameters(map[string]any{})["selection"] != temporary {
		t.Fatal("temporary selection was ignored")
	}
	c.Panel.Actions.Selection(nil)
	if c.parameters(map[string]any{})["selection"] != expected {
		t.Fatal("restore did not return to action settings")
	}
}

func TestEverySelectionAIActionUsesAIServiceAndNeverBuiltinFallback(t *testing.T) {
	for _, action := range []string{"translate", "explain", "summarize", "polish", "code", "custom"} {
		c := NewController(nil, resultwindow.Initial{Mode: "translate", Source: "selection", Action: action})
		want := ai.Selection{Provider: "default-service", Model: "default-model"}
		c.Panel.Configuration = ai.Configuration{Engine: "google", Tasks: map[string]ai.Selection{"default": want, "translate": {Provider: "capture", Model: "capture-model"}, "chat": {Provider: "chat", Model: "chat-model"}}}
		if got := c.parameters(map[string]any{})["selection"]; got != want {
			t.Fatal("selection action did not use the AI service default", action, got)
		}
		c.Panel.Configuration.Tasks["default"] = ai.Selection{}
		if got, present := c.parameters(map[string]any{})["selection"]; !present || got != (ai.Selection{}) {
			t.Fatal("missing AI configuration must be rejected, not use Google or another task", action)
		}
		capture := NewController(nil, resultwindow.Initial{Mode: "translate"})
		capture.Panel.Configuration = c.Panel.Configuration
		if _, present := capture.parameters(map[string]any{})["selection"]; present {
			t.Fatal("selection policy changed screenshot's engine")
		}
		capture.close()
		c.close()
	}
}

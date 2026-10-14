package resultui

import (
	"testing"
	"winone/internal/ai"
	"winone/internal/resultwindow"
)

func TestFirstConversationUsesFeatureModelAndKeepsManualOverride(t *testing.T) {
	for _, mode := range []string{"ocr", "translate"} {
		t.Run(mode, func(t *testing.T) {
			c := NewController(nil, resultwindow.Initial{ID: "test", Mode: mode})
			defer c.close()
			expected := ai.Selection{Provider: "feature", Model: "qmodel"}
			c.Panel.Configuration = ai.Configuration{Tasks: map[string]ai.Selection{mode: expected, "chat": {Provider: "other", Model: "chat"}}}
			if got := c.chatParameters(nil)["selection"]; got != expected {
				t.Fatalf("first follow-up requires a manual model choice: %v", got)
			}
			c.acceptResult(Outcome{Provider: "actual", Model: "routed", ModelName: "Readable model", Text: "result"})
			if got := c.chatParameters(nil)["selection"]; got != (ai.Selection{Provider: "actual", Model: "routed"}) {
				t.Fatal("follow-up lost the actual result route", got)
			}
			temporary := ai.Selection{Provider: "manual", Model: "custom"}
			c.Panel.Actions.Selection(&temporary)
			if got := c.chatParameters(nil)["selection"]; got != temporary {
				t.Fatal("manual override lost", got)
			}
			c.Panel.Actions.Selection(nil)
			if got := c.chatParameters(nil)["selection"]; got != expected {
				t.Fatal("reset did not restore feature selection", got)
			}
		})
	}
}

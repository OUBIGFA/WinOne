package resultui

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestThinkingInlineMatchesCindySharedFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../tests/chat/thinking-text.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Input  string
		Tokens []thinkingToken
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		if actual := thinkingTokens(f.Input); !reflect.DeepEqual(actual, f.Tokens) {
			t.Fatalf("%q: %#v != %#v", f.Input, actual, f.Tokens)
		}
	}
}
func TestThinkingClockStopsWhenAnswerStartsAndRetainsExpansion(t *testing.T) {
	now := time.Unix(100, 0)
	message := Message{ReasoningOpen: true}
	message.updateThinking("first", false, now)
	message.updateThinking("more", false, now.Add(time.Second))
	if !message.ReasoningActive || !message.ReasoningOpen {
		t.Fatal(message)
	}
	message.updateThinking("more", true, now.Add(5*time.Second))
	message.finishThinking(now.Add(20 * time.Second))
	if message.ReasoningActive || message.ReasoningDurationMs != 5000 || !message.ReasoningOpen {
		t.Fatal(message)
	}
	legacy := Message{}
	legacy.updateThinking("arrived with final answer", true, now)
	if legacy.ReasoningStartedAt != 0 {
		t.Fatal("invented a duration for a non-streamed response")
	}
}

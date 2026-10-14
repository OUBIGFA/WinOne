package ai

import (
	"strings"
	"testing"
)

func TestConversationPreservesRolesAndRejectsUnboundedHistory(t *testing.T) {
	history := []Message{{Role: "user", Content: "Recognized text: Hello"}, {Role: "assistant", Content: "你好"}, {Role: "user", Content: "Explain the translation"}}
	input := Generation{Messages: history}
	if err := ValidateGeneration(input); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []Generation{
		{},
		{Messages: []Message{{Role: "system", Content: "Unexpected role"}}},
		{Messages: []Message{{Role: "assistant", Content: "No question"}}},
		{Messages: []Message{{Role: "user", Content: ""}}},
		{Text: "Question", Messages: history},
		{Messages: []Message{{Role: "user", Content: strings.Repeat("x", 200001)}}},
	} {
		if err := ValidateGeneration(invalid); err == nil {
			t.Fatal("invalid conversation accepted")
		}
	}
}

package ai

import (
	"strings"
	"testing"
)

// Nightly learning uses the upstream 60k-message/40k-memory/30k-skill
// snapshot budgets. Chinese UTF-8 text must fit without dropping evidence.
func TestNightlyLearningSnapshotFitsGenerationBoundary(t *testing.T) {
	if err := ValidateGeneration(Generation{Text: strings.Repeat("学", 150000), System: "Return the learning plan as JSON."}); err != nil {
		t.Fatal("bounded upstream learning snapshot was rejected", err)
	}
	if err := ValidateGeneration(Generation{Text: strings.Repeat("x", (1<<20)+1)}); err == nil {
		t.Fatal("unbounded learning input accepted")
	}
}

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

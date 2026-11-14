package ai

import (
	"strings"
	"testing"
)

// Model windows are decided by the upstream catalog and provider. The Host
// must not invent a 64-message or 200,000-byte conversation limit.
func TestGenerationDoesNotImposeItsOwnModelContext(t *testing.T) {
	messages := make([]Message, 65)
	for i := range messages {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = Message{Role: role, Content: strings.Repeat("context ", 500)}
	}
	if err := ValidateGeneration(Generation{Messages: messages}); err != nil {
		t.Fatal(err)
	}
}

// Nightly learning uses the upstream 60k-message/40k-memory/30k-skill
// snapshot budgets. Chinese UTF-8 text must fit without dropping evidence.
func TestNightlyLearningSnapshotFitsGenerationBoundary(t *testing.T) {
	if err := ValidateGeneration(Generation{Text: strings.Repeat("学", 150000), System: "Return the learning plan as JSON."}); err != nil {
		t.Fatal("bounded upstream learning snapshot was rejected", err)
	}
	if err := ValidateGeneration(Generation{Text: strings.Repeat("x", (32<<20)+1)}); err == nil {
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
		{Messages: []Message{{Role: "user", Content: strings.Repeat("x", (32<<20)+1)}}},
	} {
		if err := ValidateGeneration(invalid); err == nil {
			t.Fatal("invalid conversation accepted")
		}
	}
}

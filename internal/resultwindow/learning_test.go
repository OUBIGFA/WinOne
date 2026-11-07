package resultwindow

import (
	"fmt"
	"strings"
	"testing"
	"winone/internal/learning"
)

func TestLearningDefaultAndUpgrade(t *testing.T) {
	defaults := DefaultSelectionActions()
	if len(defaults) < 2 || defaults[1].ID != "learn" || defaults[1].Kind != "learn" {
		t.Fatalf("missing learning after translate: %+v", defaults)
	}
	if err := ValidateActions(defaults); err != nil {
		t.Fatal(err)
	}
	old := previousSelectionActions()
	old[1].Enabled = false
	got := AddLearningAction(old)
	if got[1].ID != "learn" || got[2].Enabled {
		t.Fatal("migration loses position or user visibility")
	}
	if len(AddLearningAction(got)) != len(got) {
		t.Fatal("migration duplicated learning")
	}
}

func TestLearningUpgradePreservesFullToolbarAndCustomID(t *testing.T) {
	var full []SelectionAction
	for i := 0; i < 24; i++ {
		full = append(full, SelectionAction{ID: fmt.Sprintf("custom-%d", i), Kind: "ai", Name: "Custom", Enabled: true})
	}
	if len(AddLearningAction(full)) != 24 || AddLearningAction(full)[0].ID != "custom-0" {
		t.Fatal("full toolbar replaced")
	}
	custom := []SelectionAction{{ID: "learn", Kind: "ai", Prompt: "User's own action"}}
	if got := AddLearningAction(custom); len(got) != 1 || got[0].Prompt != custom[0].Prompt {
		t.Fatal("custom action overwritten")
	}
}

func TestLearningRequestUsesSharedAI(t *testing.T) {
	a := LearningAction()
	a.Learning = &learning.Options{Level: "beginner", Accent: "uk", HumanVoice: true, Deck: "Study", CardMode: "typing"}
	method, params, err := ActionRequest(a, "went", "auto", "zh-CN")
	if err != nil || method != "ai.generate.start" {
		t.Fatalf("%s %v", method, err)
	}
	if !strings.Contains(params["system"].(string), "beginner") {
		t.Fatal("level ignored")
	}
}

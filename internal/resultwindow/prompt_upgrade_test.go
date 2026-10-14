package resultwindow

import "testing"

func TestBuiltinPromptUpgradeKeepsUserEditsAndCopy(t *testing.T) {
	actions := DefaultSelectionActions()
	actions[0].Prompt = ""
	actions[1].Prompt = "Explain the selected content clearly and concisely. Match the user's language."
	actions[1].Name = "My explanation"
	actions[2].Prompt = "My saved instruction."
	copy := actions[5]
	upgraded := UpgradeBuiltinActions(actions)
	if upgraded[0].Prompt == "" || upgraded[1].Prompt == actions[1].Prompt {
		t.Fatal("legacy prompt not upgraded")
	}
	if upgraded[1].Name != "My explanation" || upgraded[2].Prompt != actions[2].Prompt || upgraded[5] != copy {
		t.Fatal("user edit or copy changed")
	}
	if actions[0].Prompt != "" {
		t.Fatal("upgrade changed caller's settings")
	}
}

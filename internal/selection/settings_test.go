package selection

import (
	"encoding/json"
	"testing"
	"winone/internal/resultwindow"
)

func TestCopyMovesLastOnceWithoutDiscardingCustomOrder(t *testing.T) {
	var old Preferences
	if err := json.Unmarshal([]byte(`{"schemaVersion":1,"actions":[{"id":"custom-reply"},{"id":"copy"},{"id":"translate"}]}`), &old); err != nil {
		t.Fatal(err)
	}
	actions := old.EffectiveActions()
	if actions[0].ID != "custom-reply" || actions[1].ID != "translate" || actions[2].ID != "copy" {
		t.Fatal("copy was not placed last while preserving the other actions")
	}
	if old.Actions[1].ID != "copy" {
		t.Fatal("migration mutated its source")
	}
	current := Defaults()
	if current.EffectiveActions()[len(current.Actions)-1].ID != "copy" {
		t.Fatal("new toolbar did not put copy last")
	}
	current.Actions = old.Actions
	if current.EffectiveActions()[1].ID != "copy" {
		t.Fatal("explicit current-version order was overwritten")
	}
}

func TestCustomActionsKeepOrderAndRejectInvalidConfiguration(t *testing.T) {
	value := Defaults()
	custom := resultwindow.SelectionAction{ID: "custom-email", Name: "邮件回复", Icon: "conversation", Kind: "ai", Prompt: "根据选中文字草拟一封简短回复。", Enabled: true}
	value.Actions = append([]resultwindow.SelectionAction{custom}, value.Actions...)
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	copy := value.EffectiveActions()
	copy[0].Name = "changed"
	if value.Actions[0].Name != "邮件回复" {
		t.Fatal("toolbar snapshot mutated saved order or label")
	}
	invalid := value
	invalid.Actions = append(invalid.EffectiveActions(), custom)
	if invalid.Validate() == nil {
		t.Fatal("duplicate action accepted")
	}
	invalid.Actions = []resultwindow.SelectionAction{custom}
	invalid.Actions[0].Prompt = ""
	if invalid.Validate() == nil {
		t.Fatal("empty custom instruction accepted")
	}
	invalid.Actions = []resultwindow.SelectionAction{custom}
	invalid.Actions[0].Enabled = false
	if invalid.Validate() == nil {
		t.Fatal("empty toolbar accepted")
	}
}

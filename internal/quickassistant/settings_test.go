package quickassistant

import (
	"testing"
	"winone/internal/resultwindow"
)

func TestSharedDefaultsHaveIndependentOrderAndCustomActions(t *testing.T) {
	first, second := Defaults(), Defaults()
	first.Actions[0], first.Actions[1] = first.Actions[1], first.Actions[0]
	first.Actions = append(first.Actions, resultwindow.SelectionAction{ID: "custom-email", Name: "邮件", Icon: "conversation", Kind: "ai", Prompt: "Draft a reply.", Enabled: true})
	if err := first.Validate(); err != nil {
		t.Fatal(err)
	}
	if second.Actions[0].ID != "translate" || len(second.Actions) != 5 {
		t.Fatal("separate assistant settings mutated defaults")
	}
	snapshot := first.EffectiveActions()
	snapshot[0].Name = "Changed"
	if first.Actions[0].Name == "Changed" {
		t.Fatal("snapshot aliased saved settings")
	}
	first.Actions[1].ID = first.Actions[0].ID
	if first.Validate() == nil {
		t.Fatal("duplicate action accepted")
	}
}

func TestQuickAssistantOmitsCopyFromLegacyActions(t *testing.T) {
	p := Preferences{SchemaVersion: 1, Actions: resultwindow.DefaultSelectionActions()}
	if len(p.EffectiveActions()) != 5 {
		t.Fatal("legacy copy action retained")
	}
	for _, action := range p.EffectiveActions() {
		if action.Kind == "copy" {
			t.Fatal("toolbar copy retained")
		}
	}
	if len(resultwindow.DefaultSelectionActions()) != 6 {
		t.Fatal("selection copy was removed")
	}
}

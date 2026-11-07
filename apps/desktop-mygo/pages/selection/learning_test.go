package selection

import (
	"testing"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/internal/learning"
)

func TestLearningSettingsPreserveDraftAndIndependentOptions(t *testing.T) {
	doc := selectionDocument()
	doc["learningVersion"] = 1
	doc["actions"] = []map[string]any{{"id": "learn", "name": "学习", "englishName": "Learn", "icon": "book", "kind": "learn", "enabled": true}}
	_, h, p, tt := setup(t, "selection", "running", doc)
	if p.dirty() {
		t.Fatal("loading default learning options created unsaved edits")
	}
	click(t, tt, i18n.T("learning.title"))
	if !tt.HasText(i18n.T("learning.level")) || !tt.HasText(i18n.T("learning.cardMode")) {
		t.Fatalf("missing controls: %v", tt.Texts())
	}
	p.learningAction().Learning.Level = "advanced"
	p.learningAction().Learning.Templates = map[learning.Kind]string{learning.Word: "Custom word lesson"}
	if p.saved.Value.Actions[0].Value.Learning.Level != "intermediate" {
		t.Fatal("draft aliases saved settings")
	}
	p.save()
	h.Settle(tt)
	if p.dirty() {
		t.Fatal("save did not accept learning settings")
	}
	if p.learningAction().Learning.Templates[learning.Word] != "Custom word lesson" {
		t.Fatal("template lost")
	}
}

func TestClonedLearningTemplatesDoNotMutateOriginal(t *testing.T) {
	o := learning.Defaults()
	o.Templates = map[learning.Kind]string{learning.Word: "original"}
	value := document{Value: preferences{Actions: []core.Preserved[action]{{Value: action{Kind: "learn", Learning: &o}}}}}
	copy := clone(value)
	copy.Value.Actions[0].Value.Learning.Templates[learning.Word] = "edited"
	if o.Templates[learning.Word] != "original" {
		t.Fatal("cloned template aliased original")
	}
}

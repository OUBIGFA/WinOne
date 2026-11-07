package profiles

import (
	"encoding/json"
	"testing"
	"winone/internal/learning"
	"winone/internal/resultwindow"
)

func TestSelectionProfileRoundTripOmitsRuntimeDataAndRejectsMachinePaths(t *testing.T) {
	raw := json.RawMessage(`{"schemaVersion":1,"revision":8,"automatic":true,"keyboard":false,"excludedApps":["notepad.exe"],"selectedText":"private","pid":9}`)
	value, err := ExportBuiltinSettings("selection", raw, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	json.Unmarshal(value.Module.Settings, &fields)
	if len(fields) != 3 {
		t.Fatal("runtime state escaped", fields)
	}
	preview, err := PrepareBuiltinImport("selection", value.Module, nil, nil)
	if err != nil || len(preview.Patch) == 0 || len(preview.RequiredPaths) != 0 {
		t.Fatal(preview, err)
	}
	invalid := ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(`{"automatic":true,"keyboard":false,"excludedApps":["C:\\private\\app.exe"]}`)}
	if _, err = PrepareBuiltinImport("selection", invalid, nil, nil); err == nil {
		t.Fatal("machine path accepted without mapping")
	}
}

func TestLearningProfileRoundTripKeepsOptionsAndMigrationVersion(t *testing.T) {
	for _, module := range []string{"selection", "quick-assistant"} {
		a := resultwindow.LearningAction()
		o := learning.Defaults()
		o.Level = "advanced"
		o.Templates = map[learning.Kind]string{learning.Word: "Custom lesson"}
		a.Learning = &o
		fields := map[string]any{"schemaVersion": 1, "learningVersion": 1, "actions": []resultwindow.SelectionAction{a}}
		if module == "selection" {
			fields["automatic"] = true
			fields["keyboard"] = true
			fields["excludedApps"] = []string{}
		}
		raw, _ := json.Marshal(fields)
		exported, err := ExportBuiltinSettings(module, raw, ExportOptions{})
		if err != nil {
			t.Fatal(module, err)
		}
		imported, err := PrepareBuiltinImport(module, exported.Module, nil, nil)
		if err != nil {
			t.Fatal(module, err)
		}
		data, _ := json.Marshal(imported.Patch)
		var roundtrip struct {
			LearningVersion int
			Actions         []resultwindow.SelectionAction
		}
		if err = json.Unmarshal(data, &roundtrip); err != nil {
			t.Fatal(err)
		}
		if roundtrip.LearningVersion != 1 || len(roundtrip.Actions) != 1 || roundtrip.Actions[0].Learning.Level != "advanced" || roundtrip.Actions[0].Learning.Templates[learning.Word] != "Custom lesson" {
			t.Fatalf("%s: %s", module, data)
		}
	}
}

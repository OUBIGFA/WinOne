package profiles

import (
	"encoding/json"
	"testing"
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

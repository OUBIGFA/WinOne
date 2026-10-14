package profiles

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"winone/modules/stroke"
)

func TestStrokeProfilePreservesActionsAndRequiresLocalPathMapping(t *testing.T) {
	prefs := stroke.Defaults()
	prefs.Scopes = append(prefs.Scopes, stroke.Scope{ID: "custom", Name: "Editor", Kind: "application", Enabled: false, ProcessPath: `C:\Private\editor.exe`, Packages: []string{prefs.ActionPackages[0].ID}})
	prefs.Assemblies = append(prefs.Assemblies, `C:\Private\custom.dll`)
	raw, _ := json.Marshal(prefs)
	exported, err := ExportBuiltinSettings("stroke", raw, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(exported.Module.Settings, []byte("Private")) {
		t.Fatal("local path leaked into portable settings")
	}
	preview, err := PrepareBuiltinImport("stroke", exported.Module, exported.Paths, nil)
	if err != nil || len(preview.MissingPaths) != 2 || len(preview.Patch) != 0 {
		t.Fatalf("mapping was bypassed: %+v %v", preview, err)
	}
	bindings := map[string]string{}
	for key, ref := range exported.Paths {
		if ref.Kind == "program" {
			bindings[key] = `D:\Programs\editor.exe`
		} else {
			bindings[key] = `D:\Plugins\custom.dll`
		}
	}
	preview, err = PrepareBuiltinImport("stroke", exported.Module, exported.Paths, bindings)
	if err != nil || len(preview.MissingPaths) != 0 {
		t.Fatal(err, preview.MissingPaths)
	}
	var got stroke.Preferences
	if err = json.Unmarshal(preview.Patch, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.ActionPackages, prefs.ActionPackages) || !reflect.DeepEqual(got.Gestures, prefs.Gestures) || got.Scopes[len(got.Scopes)-1].ProcessPath != `D:\Programs\editor.exe` || got.Assemblies[len(got.Assemblies)-1] != `D:\Plugins\custom.dll` {
		t.Fatal("gesture/action semantics or mappings changed")
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStrokeProfileRejectsUnmappedExecutablePaths(t *testing.T) {
	prefs := stroke.Defaults()
	raw, _ := json.Marshal(prefs)
	exported, err := ExportBuiltinSettings("stroke", raw, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(exported.Module.Settings, &body)
	first := body["scopes"].([]any)[0].(map[string]any)
	first["processPath"] = `C:\Unreviewed\app.exe`
	exported.Module.Settings, _ = json.Marshal(body)
	if _, err := PrepareBuiltinImport("stroke", exported.Module, exported.Paths, nil); err == nil {
		t.Fatal("raw device path accepted")
	}
}

package profiles

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// Portable preferences must survive without copying a source-machine folder.
func TestWinTrayLoginAndDirectoryPortableRoundTrip(t *testing.T) {
	raw := strings.Replace(portableTray, `"programRef":"wintray.editor"`, `"exePath":"D:\\Apps\\QQ.exe","workingDirectory":"D:\\Private Data","qqWaitForLogin":true`, 1)
	raw = strings.Replace(raw, `{"exitAfterManagedAppsCompleted"`, `{"schemaVersion":3,"exitAfterManagedAppsCompleted"`, 1)
	exported, err := ExportBuiltinSettings("wintray", json.RawMessage(raw), ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(exported.Module.Settings), "Private Data") {
		t.Fatal("source directory leaked into portable settings")
	}
	directory := t.TempDir()
	mappings := map[string]string{"wintray.program.0": filepath.Join(directory, "QQ.exe"), "wintray.directory.0": directory}
	preview, err := PrepareBuiltinImport("wintray", exported.Module, exported.Paths, mappings)
	if err != nil {
		t.Fatal(err)
	}
	var restored struct {
		ManagedApps []struct {
			QQWaitForLogin   bool
			WorkingDirectory string
		}
	}
	if err := json.Unmarshal(preview.Patch, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.ManagedApps) != 1 || !restored.ManagedApps[0].QQWaitForLogin || restored.ManagedApps[0].WorkingDirectory != directory {
		t.Fatalf("lost login/directory settings: %s", preview.Patch)
	}
	delete(mappings, "wintray.directory.0")
	preview, err = PrepareBuiltinImport("wintray", exported.Module, exported.Paths, mappings)
	if err != nil || len(preview.MissingPaths) != 1 {
		t.Fatalf("unmapped directory was not reported: %+v %v", preview, err)
	}
}

func TestWinTrayQQPortableImportAcceptsOnlyBooleans(t *testing.T) {
	paths := map[string]PathReference{"wintray.editor": {Kind: "program", Label: "QQ"}}
	mappings := map[string]string{"wintray.editor": filepath.Join(t.TempDir(), "QQ.exe")}
	for _, value := range []string{"true", "false", "null", `"true"`} {
		raw := strings.Replace(portableTray, `"launchHiddenInBackground":false`, `"launchHiddenInBackground":false,"qqWaitForLogin":`+value, 1)
		_, err := PrepareBuiltinImport("wintray", ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(raw)}, paths, mappings)
		if (err == nil) != (value == "true" || value == "false") {
			t.Fatalf("QQ preference %s: %v", value, err)
		}
	}
}

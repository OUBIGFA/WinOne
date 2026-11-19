package profiles

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWinTrayPackagedAppPortableRoundTrip(t *testing.T) {
	raw := strings.Replace(portableTray, `"programRef":"wintray.editor"`, `"exePath":"shell:AppsFolder\\Example.Reader_123456789abcd!App"`, 1)
	raw = strings.Replace(raw, `{"exitAfterManagedAppsCompleted"`, `{"schemaVersion":4,"exitAfterManagedAppsCompleted"`, 1)
	exported, err := ExportBuiltinSettings("wintray", json.RawMessage(raw), ExportOptions{IncludeArguments: map[string]bool{"editor": true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.Paths) != 0 {
		t.Fatalf("stable application identity became a file mapping: %+v", exported.Paths)
	}
	preview, err := PrepareBuiltinImport("wintray", exported.Module, exported.Paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ManagedApps []struct {
			ExePath, Args   string
			CollectTrayIcon bool
		}
	}
	if err := json.Unmarshal(preview.Patch, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.ManagedApps) != 1 || result.ManagedApps[0].ExePath != `shell:AppsFolder\Example.Reader_123456789abcd!App` || result.ManagedApps[0].Args != "--profile work" || !result.ManagedApps[0].CollectTrayIcon {
		t.Fatalf("packaged app settings lost: %s", preview.Patch)
	}
}

func TestLegacyPortablePausedModeDoesNotStartOnImport(t *testing.T) {
	raw := strings.ReplaceAll(portableTray, `"runOnStartup":true`, `"runOnStartup":false`)
	raw = strings.ReplaceAll(raw, `"enabled":true`, `"enabled":false`)
	paths := map[string]PathReference{"wintray.editor": {Kind: "program", Label: "Editor"}}
	preview, err := PrepareBuiltinImport("wintray", ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(raw)}, paths, map[string]string{"wintray.editor": `C:\Apps\editor.exe`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(preview.Patch), `"autoMinimizeAndHideOnLaunch":true`) {
		t.Fatalf("paused legacy entry re-enabled: %s", preview.Patch)
	}
}

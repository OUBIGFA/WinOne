package profiles

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// Auto-login is a portable, explicit preference, not runtime account state.
// Export/import must retain it without opting legacy profiles into login.
func TestWinTrayWeChatAutoLoginSurvivesPortableRoundTrip(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy-off", true: "enabled"}[enabled], func(t *testing.T) {
			raw := strings.Replace(portableTray, `"programRef":"wintray.editor"`, `"exePath":"D:\\Apps\\Weixin.exe"`, 1)
			raw = strings.Replace(raw, `{"exitAfterManagedAppsCompleted"`, `{"schemaVersion":3,"exitAfterManagedAppsCompleted"`, 1)
			if enabled {
				raw = strings.Replace(raw, `"launchHiddenInBackground":false`, `"launchHiddenInBackground":false,"wechatAutoLogin":true`, 1)
			}
			exported, err := ExportBuiltinSettings("wintray", json.RawMessage(raw), ExportOptions{})
			if err != nil {
				t.Fatal(err)
			}
			mappings := map[string]string{"wintray.program.0": filepath.Join(t.TempDir(), "WeChat.exe")}
			preview, err := PrepareBuiltinImport("wintray", exported.Module, exported.Paths, mappings)
			if err != nil {
				t.Fatal(err)
			}
			var restored struct {
				ManagedApps []struct{ WeChatAutoLogin bool } `json:"managedApps"`
			}
			if err := json.Unmarshal(preview.Patch, &restored); err != nil {
				t.Fatal(err)
			}
			if len(restored.ManagedApps) != 1 || restored.ManagedApps[0].WeChatAutoLogin != enabled {
				t.Fatalf("auto-login changed in portable transfer: enabled=%t patch=%s", enabled, preview.Patch)
			}
		})
	}
}

func TestWinTrayWeChatPortableImportAcceptsOnlyBooleans(t *testing.T) {
	paths := map[string]PathReference{"wintray.editor": {Kind: "program", Label: "微信"}}
	mappings := map[string]string{"wintray.editor": filepath.Join(t.TempDir(), "Weixin.exe")}
	for _, value := range []string{"true", "false", "null", `"true"`, `{"secret":"do not import"}`} {
		raw := strings.Replace(portableTray, `"launchHiddenInBackground":false`, `"launchHiddenInBackground":false,"wechatAutoLogin":`+value, 1)
		preview, err := PrepareBuiltinImport("wintray", ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(raw)}, paths, mappings)
		valid := value == "true" || value == "false"
		if (err == nil) != valid {
			t.Fatalf("wechatAutoLogin=%s: error=%v patch=%s", value, err, preview.Patch)
		}
	}
}

func TestWinTrayPortableAutoHotkeyMappingKeepsScriptLaunchSemantics(t *testing.T) {
	raw := strings.NewReplacer(`"collectTrayIcon":true`, `"collectTrayIcon":false`, `"autoMinimizeAndHideOnLaunch":true`, `"autoMinimizeAndHideOnLaunch":false`, `"launchHiddenInBackground":false`, `"launchHiddenInBackground":true`).Replace(portableTray)
	paths := map[string]PathReference{"wintray.editor": {Kind: "program", Label: "Hotkeys"}}
	path := filepath.Join(t.TempDir(), "中文 hotkeys.AHK")
	preview, err := PrepareBuiltinImport("wintray", ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(raw)}, paths, map[string]string{"wintray.editor": path})
	if err != nil {
		t.Fatal(err)
	}
	var restored struct {
		ManagedApps []struct {
			ExePath                  string
			Args                     string
			LaunchHiddenInBackground bool
		} `json:"managedApps"`
	}
	if err := json.Unmarshal(preview.Patch, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.ManagedApps) != 1 || restored.ManagedApps[0].ExePath != path || !restored.ManagedApps[0].LaunchHiddenInBackground || restored.ManagedApps[0].Args != "--profile work" {
		t.Fatalf("AutoHotkey transfer changed launch preferences: %s", preview.Patch)
	}
}

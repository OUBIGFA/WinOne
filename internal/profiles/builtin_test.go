package profiles

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExportCaptureReplacesDevicePath(t *testing.T) {
	raw := json.RawMessage(`{"schemaVersion":1,"revision":18,"outputDirectory":"C:\\Users\\Alice\\Pictures","copyToClipboard":true,"askWhereToSave":false,"openEditor":true,"pinAfterCapture":false,"theme":"dark","scrollDelay":300,"scrollFromTop":true,"last":{"path":"private.png"}}`)
	result, err := ExportBuiltinSettings("capture", raw, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"Alice", "revision", "private.png", "theme"} {
		if strings.Contains(string(result.Module.Settings), private) {
			t.Fatalf("export leaked %s", private)
		}
	}
	var settings map[string]any
	if err := json.Unmarshal(result.Module.Settings, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["outputDirectoryRef"] != "capture.output" || settings["openEditor"] != true {
		t.Fatal(settings)
	}
	if result.Paths["capture.output"].Kind != "directory" || result.Paths["capture.output"].Hint != "" {
		t.Fatal(result.Paths)
	}
	archive := sampleArchive()
	archive.Modules = map[string]ModuleSettings{"capture": result.Module}
	archive.Paths = result.Paths
	data, err := Encode(archive)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Alice") || strings.Contains(string(data), "private.png") {
		t.Fatal("archive leaked source-machine data")
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded.Modules["capture"].Settings) != string(result.Module.Settings) {
		t.Fatal("portable settings changed in archive")
	}
}

func TestExportWinTrayStripsNestedRuntimeAndRequiresArgumentChoice(t *testing.T) {
	raw := json.RawMessage(`{"schemaVersion":3,"language":"zh-CN","runAtLogon":true,"startMinimizedToTray":true,"exitAfterManagedAppsCompleted":true,"closeWindowRetrySeconds":10,"startupIntervalSeconds":3,"managedApps":[{"id":"app-1","name":"Editor","exePath":"D:\\Apps\\edit.exe","args":"--token=private","runOnStartup":true,"collectTrayIcon":true,"launchViaLogonTask":false,"launchHiddenInBackground":false,"hwnd":123,"trayBehavior":{"autoMinimizeAndHideOnLaunch":true,"closeDelaySeconds":0,"pid":999},"schedule":{"enabled":true,"startDelayMinutes":5,"autoExitMinutes":30,"frequencyEnabled":true,"frequencyDays":1,"frequencyRuns":1,"history":"private"}}]}`)
	result, err := ExportBuiltinSettings("wintray", raw, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"D:", "private", "hwnd", "pid", "runAtLogon", "language"} {
		if strings.Contains(string(result.Module.Settings), private) {
			t.Fatalf("export leaked %s: %s", private, result.Module.Settings)
		}
	}
	if len(result.OmittedArguments) != 1 || result.OmittedArguments[0] != "app-1" {
		t.Fatal(result.OmittedArguments)
	}
	if len(result.Paths) != 1 {
		t.Fatal(result.Paths)
	}
	selected, err := ExportBuiltinSettings("wintray", raw, ExportOptions{IncludeArguments: map[string]bool{"app-1": true}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(selected.Module.Settings), "--token=private") || len(selected.OmittedArguments) != 0 {
		t.Fatal(string(selected.Module.Settings))
	}
	if _, err := ExportBuiltinSettings("wintray", raw, ExportOptions{IncludeArguments: map[string]bool{"missing": true}}); err == nil {
		t.Fatal("stale selection accepted")
	}
	malformed := strings.Replace(string(raw), `"closeDelaySeconds":0`, `"closeDelaySeconds":{"secret":"private"}`, 1)
	if _, err := ExportBuiltinSettings("wintray", json.RawMessage(malformed), ExportOptions{}); err == nil {
		t.Fatal("nested payload accepted as numeric preference")
	}
}

func TestExportAOTSeparatesNamesFromPaths(t *testing.T) {
	raw := json.RawMessage(`{"schemaVersion":1,"revision":7,"enabled":false,"frame":true,"frameThickness":3,"frameColor":"#808080","frameOpacity":100,"useAccent":true,"roundCorners":true,"blockInGameMode":true,"sound":false,"excludedApps":["game.exe","E:\\Games\\game.exe"],"ownedWindows":[123]}`)
	result, err := ExportBuiltinSettings("always-on-top", raw, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"E:", "revision", "enabled", "ownedWindows"} {
		if strings.Contains(string(result.Module.Settings), private) {
			t.Fatalf("export leaked %s", private)
		}
	}
	if !strings.Contains(string(result.Module.Settings), "game.exe") || len(result.Paths) != 1 {
		t.Fatal(result)
	}
}

func TestExportRejectsUnsupportedOrMalformedSource(t *testing.T) {
	for _, tt := range []struct{ id, raw string }{
		{"unknown", `{}`}, {"capture", `{"schemaVersion":2}`},
		{"capture", `{"schemaVersion":1,"schemaVersion":1}`},
		{"wintray", `{"schemaVersion":3,"managedApps":null}`},
		{"always-on-top", `{"schemaVersion":1,"frame":null}`},
	} {
		if _, err := ExportBuiltinSettings(tt.id, json.RawMessage(tt.raw), ExportOptions{}); err == nil {
			t.Fatalf("accepted %s", tt.raw)
		}
	}
}

package profiles

import (
	"encoding/json"
	"strings"
	"testing"
)

const portableCapture = `{"outputDirectoryRef":"capture.output","copyToClipboard":true,"askWhereToSave":false,"openEditor":true,"pinAfterCapture":false,"scrollDelay":300,"scrollFromTop":false}`
const portableTray = `{"exitAfterManagedAppsCompleted":true,"closeWindowRetrySeconds":10,"startupIntervalSeconds":3,"managedApps":[{"id":"editor","name":"Editor","programRef":"wintray.editor","args":"--profile work","runOnStartup":true,"collectTrayIcon":true,"launchViaLogonTask":false,"launchHiddenInBackground":false,"trayBehavior":{"autoMinimizeAndHideOnLaunch":true,"closeDelaySeconds":0},"schedule":{"enabled":true,"startDelayMinutes":1,"autoExitMinutes":30,"frequencyEnabled":false,"frequencyDays":1,"frequencyRuns":1}}]}`

func TestImportRequiresExplicitPathMappings(t *testing.T) {
	module := ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(portableCapture)}
	paths := map[string]PathReference{"capture.output": {Kind: "directory", Label: "Pictures", Hint: `C:\DoNotUseHint`}}
	preview, err := PrepareBuiltinImport("capture", module, paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.MissingPaths) != 1 || preview.MissingPaths[0] != "capture.output" || preview.Patch != nil {
		t.Fatal(preview)
	}
	preview, err = PrepareBuiltinImport("capture", module, paths, map[string]string{"capture.output": `D:\Pictures`})
	if err != nil {
		t.Fatal(err)
	}
	var patch map[string]any
	if err = json.Unmarshal(preview.Patch, &patch); err != nil {
		t.Fatal(err)
	}
	if patch["outputDirectory"] != `D:\Pictures` || patch["openEditor"] != true {
		t.Fatal(patch)
	}
	for _, field := range []string{"revision", "theme", "outputDirectoryRef"} {
		if _, exists := patch[field]; exists {
			t.Fatal(field)
		}
	}
}

func TestImportRejectsUnknownAndInvalidPortableSettings(t *testing.T) {
	paths := map[string]PathReference{"capture.output": {Kind: "directory", Label: "Pictures"}}
	for _, raw := range []string{
		strings.Replace(portableCapture, `"scrollDelay":300`, `"scrollDelay":99`, 1),
		strings.Replace(portableCapture, `"openEditor":true`, `"openEditor":null`, 1),
		strings.Replace(portableCapture, `"openEditor":true`, `"openEditor":true,"revision":1`, 1),
		strings.Replace(portableCapture, `"outputDirectoryRef":"capture.output"`, `"outputDirectoryRef":"missing"`, 1),
	} {
		if _, err := PrepareBuiltinImport("capture", ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(raw)}, paths, nil); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, path := range []string{`relative\Pictures`, `C:Pictures`, `\\.\pipe\test`, `C:\Pictures:stream`} {
		if _, err := PrepareBuiltinImport("capture", ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(portableCapture)}, paths, map[string]string{"capture.output": path}); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	for _, version := range []int{0, 2} {
		if _, err := PrepareBuiltinImport("capture", ModuleSettings{SchemaVersion: version, Settings: json.RawMessage(portableCapture)}, paths, nil); err == nil {
			t.Fatal("unsupported version accepted")
		}
	}
}

func TestImportWinTrayReportsArgumentsAndValidatesLaunchModes(t *testing.T) {
	paths := map[string]PathReference{"wintray.editor": {Kind: "program", Label: "Editor"}}
	mappings := map[string]string{"wintray.editor": `D:\Apps\edit.exe`}
	module := ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(portableTray)}
	preview, err := PrepareBuiltinImport("wintray", module, paths, mappings)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ArgumentApps) != 1 || preview.ArgumentApps[0] != "editor" {
		t.Fatal(preview)
	}
	unmapped, err := PrepareBuiltinImport("wintray", module, paths, nil)
	if err != nil || unmapped.Patch != nil || len(unmapped.MissingPaths) != 1 || len(unmapped.ArgumentApps) != 1 {
		t.Fatalf("missing path preview: %+v, %v", unmapped, err)
	}
	if !strings.Contains(string(preview.Patch), `"runOnStartup":true`) || !strings.Contains(string(preview.Patch), `"exePath"`) {
		t.Fatal(string(preview.Patch))
	}
	for _, raw := range []string{
		strings.Replace(portableTray, `"closeDelaySeconds":0`, `"closeDelaySeconds":601`, 1),
		strings.Replace(portableTray, `"frequencyDays":1`, `"frequencyDays":1,"pid":12`, 1),
		strings.Replace(portableTray, `"launchViaLogonTask":false`, `"launchViaLogonTask":true`, 1),
	} {
		module.Settings = json.RawMessage(raw)
		if _, err = PrepareBuiltinImport("wintray", module, paths, mappings); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	module.Settings = json.RawMessage(portableTray)
	mappings["wintray.editor"] = `D:\Apps\script.ps1`
	if _, err = PrepareBuiltinImport("wintray", module, paths, mappings); err == nil {
		t.Fatal("script accepted with tray mode")
	}
}

func TestImportRejectsDuplicateApplications(t *testing.T) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(portableTray), &payload); err != nil {
		t.Fatal(err)
	}
	var apps []json.RawMessage
	if err := json.Unmarshal(payload["managedApps"], &apps); err != nil {
		t.Fatal(err)
	}
	duplicate, err := json.Marshal([]json.RawMessage{apps[0], apps[0]})
	if err != nil {
		t.Fatal(err)
	}
	payload["managedApps"] = duplicate
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]PathReference{"wintray.editor": {Kind: "program", Label: "Editor"}}
	if _, err := PrepareBuiltinImport("wintray", ModuleSettings{SchemaVersion: 1, Settings: raw}, paths, nil); err == nil {
		t.Fatal("duplicate application accepted")
	}
}

func TestImportAOTMapsExcludedPrograms(t *testing.T) {
	raw := json.RawMessage(`{"frame":true,"frameThickness":3,"frameColor":"#808080","frameOpacity":100,"useAccent":true,"roundCorners":true,"blockInGameMode":true,"sound":false,"excludedApps":[{"name":"game.exe"},{"pathRef":"always-on-top.game"}]}`)
	paths := map[string]PathReference{"always-on-top.game": {Kind: "program", Label: "Game"}}
	preview, err := PrepareBuiltinImport("always-on-top", ModuleSettings{SchemaVersion: 1, Settings: raw}, paths, map[string]string{"always-on-top.game": `D:\Games\game.exe`})
	if err != nil {
		t.Fatal(err)
	}
	var patch struct {
		ExcludedApps []string `json:"excludedApps"`
	}
	if err = json.Unmarshal(preview.Patch, &patch); err != nil {
		t.Fatal(err)
	}
	if len(patch.ExcludedApps) != 2 || patch.ExcludedApps[1] != `D:\Games\game.exe` {
		t.Fatal(patch)
	}
	raw = json.RawMessage(strings.Replace(string(raw), `"name":"game.exe"`, `"name":"C:\\game.exe"`, 1))
	if _, err = PrepareBuiltinImport("always-on-top", ModuleSettings{SchemaVersion: 1, Settings: raw}, paths, nil); err == nil {
		t.Fatal("path bypassed mapping")
	}
}

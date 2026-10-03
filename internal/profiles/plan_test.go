package profiles

import (
	"encoding/json"
	"testing"
)

func TestArchivePlanKeepsMissingModulesAndIdentifiesPathRequirements(t *testing.T) {
	archive := sampleArchive()
	archive.Modules["capture"] = ModuleSettings{SchemaVersion: 1, Settings: json.RawMessage(portableCapture)}
	archive.Paths = map[string]PathReference{"capture.output": {Kind: "directory", Label: "Pictures"}}
	plan, err := PrepareArchive(archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Modules) != 2 || plan.Modules[0].ID != "capture" || plan.Modules[0].State != "needs_mapping" || plan.Modules[1].State != "missing_module" {
		t.Fatal(plan.Modules)
	}
	if len(plan.Modules[0].MissingPaths) != 1 || plan.Modules[0].Patch != nil {
		t.Fatal(plan.Modules[0])
	}
	if string(archive.Modules["example.optional"].Settings) != `{"value":"保留未安装模块"}` {
		t.Fatal("unknown settings changed")
	}
	plan, err = PrepareArchive(archive, map[string]string{"capture.output": `D:\Pictures`})
	if err != nil || plan.Modules[0].State != "ready" || len(plan.Modules[0].Patch) == 0 {
		t.Fatal(plan, err)
	}
}

func TestArchivePlanReportsEquivalentShortcutConflicts(t *testing.T) {
	archive := sampleArchive()
	archive.Modules = map[string]ModuleSettings{}
	archive.Preferences.Shortcuts = map[string]string{"capture.region": "Ctrl+Alt+S", "always-on-top.toggle": "alt+ctrl+s"}
	plan, err := PrepareArchive(archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ShortcutIssues) != 1 || plan.ShortcutIssues[0].Kind != "conflict" {
		t.Fatal(plan.ShortcutIssues)
	}
	archive.Preferences.Shortcuts = map[string]string{"wintray.launch": "Ctrl+Alt+W"}
	plan, err = PrepareArchive(archive, nil)
	if err != nil || len(plan.ShortcutIssues) != 1 || plan.ShortcutIssues[0].Kind != "unsupported" {
		t.Fatal(plan, err)
	}
}

func TestInvalidModuleDoesNotHideOtherPreviewResults(t *testing.T) {
	archive := sampleArchive()
	archive.Modules["capture"] = ModuleSettings{SchemaVersion: 99, Settings: json.RawMessage(`{}`)}
	plan, err := PrepareArchive(archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Modules[0].State != "invalid" || plan.Modules[0].Error == "" || plan.Modules[1].State != "missing_module" {
		t.Fatal(plan.Modules)
	}
}

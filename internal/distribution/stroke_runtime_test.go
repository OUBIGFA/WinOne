package distribution

import (
	"slices"
	"testing"
)

func TestStrokeRuntimeIsCompleteAndOldPackagesRemainValid(t *testing.T) {
	files := []string{"stroke/WinOne.Stroke.exe", "stroke/Stroke.Base.dll", "stroke/Base.dll", "stroke/Stroke.dll", "stroke/Stroke-LICENSE.txt", "stroke/schema.json", "stroke/defaults.json", "stroke/migrations.json"}
	current := requiredForPayload(map[string]bool{files[0]: true})
	old := requiredForPayload(map[string]bool{"WinOne.exe": true})
	for _, file := range files {
		if !allowed(file) || !slices.Contains(current, file) {
			t.Fatalf("incomplete Stroke payload: %s", file)
		}
		if slices.Contains(old, file) {
			t.Fatalf("old rollback unexpectedly needs %s", file)
		}
	}
	for _, file := range []string{"stroke/managed-settings.json", "stroke/tests/FixturePlugin.dll", "stroke/sources/Script.cs", "stroke/Configure.exe"} {
		if allowed(file) {
			t.Fatalf("unexpected payload: %s", file)
		}
	}
}

func TestImageRuntimeKeepsLegacyChatRollbackReadable(t *testing.T) {
	if !allowed("shared/ai/runtime/photon_rs_bg.wasm") {
		t.Fatal("Chat image runtime not packaged")
	}
	old := requiredForPayload(map[string]bool{"shared/chat/worker.js": true})
	if slices.Contains(old, "shared/ai/runtime/photon_rs_bg.wasm") {
		t.Fatal("legacy Chat rollback requires a new runtime")
	}
}

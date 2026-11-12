package distribution

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestAccountPluginPayloadIsPinnedAndOptionalForOlderPackages(t *testing.T) {
	if !allowed("shared/ai/plugins/cline/index.mjs") {
		t.Fatal("bundled Cline entry is rejected")
	}
	for _, name := range []string{"shared/ai/plugins/cline/debug.mjs", "shared/ai/plugins/cline/plugin-auth.json", "shared/ai/runtime/debug.exe"} {
		if allowed(name) {
			t.Fatalf("unreviewed plugin payload accepted: %s", name)
		}
	}
	old := requiredForPayload(map[string]bool{"shared/ai/WinOne.AI.exe": true})
	if slices.Contains(old, "shared/ai/runtime/bun.exe") {
		t.Fatal("older verified packages now require the new plugin runtime")
	}
	current := requiredForPayload(map[string]bool{"shared/ai/plugins/cline/index.mjs": true})
	for _, name := range []string{"shared/ai/runtime/bun.exe", "shared/ai/plugins/cline/index.mjs", "shared/ai/plugins/zen-free/provider.mjs"} {
		if !slices.Contains(current, name) {
			t.Fatalf("partial plugin payload can omit %s", name)
		}
	}
}

func TestOCRPayloadUsesPinnedInventory(t *testing.T) {
	if !allowed("capture/Ocr/recognize.py") || !allowed("shared/ai/WinOne.AI.exe") || !allowed("shared/ui/WinOne.Results.exe") {
		t.Fatal("text feature runtime is not packaged")
	}
	for _, name := range []string{"capture/Ocr/local-debug.py", "capture/Ocr/python/Lib/site-packages/secret.txt", "shared/ai/test-config/settings.json", "shared/ui/config.json", "shared/ui/debug.exe"} {
		if allowed(name) {
			t.Fatalf("unexpected runtime file accepted: %s", name)
		}
	}
	root, license := fixture(t)
	dir := filepath.Join(root, "shared", "ai")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "WinOne.AI.exe"), []byte("incomplete"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := inputs(root, license); err == nil {
		t.Fatal("partial text feature runtime accepted")
	}
}

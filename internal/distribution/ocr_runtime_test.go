package distribution

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestChatRuntimeRequiresItsCompleteProtocolBridge(t *testing.T) {
	for _, name := range []string{"shared/chat/worker.js", "shared/chat/runtime.json"} {
		if !allowed(name) {
			t.Fatal("chat bundle rejected", name)
		}
	}
	for _, name := range []string{"shared/chat/settings.json", "shared/chat/node_modules/token.txt"} {
		if allowed(name) {
			t.Fatal("unreviewed chat data allowed", name)
		}
	}
	current := requiredForPayload(map[string]bool{"shared/chat/worker.js": true})
	for _, name := range []string{"shared/chat/runtime.json", "shared/ai/runtime/bun.exe", "shared/ai/WinOne.AI.exe"} {
		if !slices.Contains(current, name) {
			t.Fatal("incomplete agent package", name)
		}
	}
	old := requiredForPayload(map[string]bool{"shared/ai/WinOne.AI.exe": true})
	if slices.Contains(old, "shared/chat/worker.js") {
		t.Fatal("legacy rollback package suddenly requires Chat")
	}
}

func TestCodeModeRuntimeIsPinnedAndOptionalForOlderChatBundles(t *testing.T) {
	for _, name := range []string{"shared/chat/codemode-worker.js", "shared/chat/quickjs.wasm", "shared/chat/licenses/chat-codemode-notices.json", "shared/chat/licenses/quickjs-wasi-3.6.2-LICENSE.txt"} {
		if !allowed(name) {
			t.Fatal("Code Mode runtime rejected", name)
		}
	}
	for _, name := range []string{"shared/chat/quickjs-debug.wasm", "shared/chat/licenses/codemode-extra.txt"} {
		if allowed(name) {
			t.Fatal("unreviewed Code Mode data accepted", name)
		}
	}
	current := requiredForPayload(map[string]bool{"shared/chat/codemode-worker.js": true})
	for _, name := range []string{"shared/chat/quickjs.wasm", "shared/chat/licenses/chat-codemode-notices.json", "shared/chat/licenses/quickjs-compiler-rt-4434dab-LICENSE.txt"} {
		if !slices.Contains(current, name) {
			t.Fatal("incomplete Code Mode package", name)
		}
	}
	old := requiredForPayload(map[string]bool{"shared/chat/worker.js": true})
	if slices.Contains(old, "shared/chat/codemode-worker.js") {
		t.Fatal("older Chat bundle suddenly requires Code Mode")
	}
}

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

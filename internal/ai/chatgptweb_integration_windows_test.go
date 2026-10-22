//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The test runner substitutes a local website, not the plugin's auth, browser
// or worker. This crosses WinOne's real short-lived process and profile boundary.
func TestChatGPTWebPluginSignInAndReply(t *testing.T) {
	fixture, executable := os.Getenv("WINONE_CHATGPT_WEB_FIXTURE"), os.Getenv("WINONE_AI_PLUGIN_TEST_EXE")
	if fixture == "" || executable == "" {
		t.Skip("explicit reviewed ChatGPT Web fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 330*time.Second)
	defer cancel()
	runtime := Translator{Executable: executable, DataDirectory: t.TempDir(), AccountBackend: "plugin", PluginLifetime: ctx}
	var options map[string]any
	if err := json.Unmarshal([]byte(os.Getenv("WINONE_CHATGPT_WEB_OPTIONS")), &options); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(runtime.DataDirectory, "third-party", "plugins.json")
	if err := os.MkdirAll(filepath.Dir(registry), 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"plugins": []any{map[string]any{"spec": fixture, "options": options}}})
	if err := os.WriteFile(registry, raw, 0600); err != nil {
		t.Fatal(err)
	}
	listing, err := runtime.Accounts(ctx, "accounts.list", "chatgpt-web")
	if err != nil {
		t.Fatal(err)
	}
	var before struct{ Methods []json.RawMessage }
	if json.Unmarshal(listing, &before) != nil || len(before.Methods) != 4 {
		t.Fatalf("all four authorization methods required: %s", listing)
	}
	method := 0
	done := false
	err = runtime.SignInMethod(ctx, "chatgpt-web", &method, make(chan string), func(raw json.RawMessage) {
		var event struct {
			Error  string
			SignIn struct{ State, Error string }
		}
		if json.Unmarshal(raw, &event) != nil {
			t.Errorf("invalid event: %s", raw)
			return
		}
		if event.Error != "" || event.SignIn.Error != "" {
			t.Errorf("authorization failure: %s", raw)
		}
		if event.SignIn.State == "done" {
			done = true
		}
	})
	if err != nil || !done {
		t.Fatalf("dedicated browser login did not finish: %v", err)
	}
	listing, err = runtime.Accounts(ctx, "accounts.list", "chatgpt-web")
	if err != nil || !strings.Contains(string(listing), "fixture@example.invalid") {
		t.Fatalf("login lost across workers: %s %v", listing, err)
	}
	reply, err := runtime.Translate(ctx, Preferences{Engine: "ai", AuthMode: "account", AgentBackend: "plugin", Agent: "chatgpt-web", Model: "web-current"}, "", "本地网页文本测试")
	if err != nil || !strings.Contains(reply, "中文流式") {
		t.Fatalf("browser reply across workers: %q %v", reply, err)
	}
}

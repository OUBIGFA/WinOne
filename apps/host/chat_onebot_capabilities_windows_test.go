//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"winone/internal/ai"
	"winone/internal/chat"
	"winone/internal/host"
	"winone/internal/processidentity"
)

func TestOneBotCapabilityCatalogUsesHostResourcesAndGlobalDefault(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_ONEBOT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit isolated runtime paths are required")
	}
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := chat.Open(filepath.Join(directory, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(directory, "plugin")
	if err = os.Mkdir(plugin, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(plugin, "package.json"), []byte(`{"name":"optional-tools","pi":{"extensions":["extension.mjs"]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(plugin, "extension.mjs"), []byte(`throw Error('must not execute during catalog inspection');`), 0600); err != nil {
		t.Fatal(err)
	}
	prefs := store.Preferences()
	prefs.PluginDirectories = []string{plugin}
	prefs.DefaultThinkingLevel = "low"
	prefs.MCPServers = []chat.MCPServer{{Name: "private", Enabled: true, URL: "https://private.example.test/mcp", Headers: map[string]string{"Authorization": "Bearer PRIVATE_CREDENTIAL"}}}
	if _, err = store.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	service := newAIService(ctx, directory, "")
	service.store, err = ai.Open(service.storePath)
	if err != nil {
		t.Fatal(err)
	}
	config := service.store.Configuration()
	config.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.test/v1", Models: []string{"model"}}}
	delete(config.Tasks, "chat")
	config.Tasks["default"] = ai.Selection{Provider: "fixture", Model: "model"}
	if _, err = service.store.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	control := newChatOneBot(&chatControl{ctx: ctx, directory: directory, store: store, ai: service})
	runner, err := chat.StartRunner(ctx, bun, worker, directory, control.event)
	if err != nil {
		t.Fatal(err)
	}
	control.runner = runner
	defer control.close(context.Background())
	if _, err = runner.Call(ctx, "init", map[string]string{"directory": directory}); err != nil {
		t.Fatal(err)
	}
	if _, err = control.call(ctx, "profiles.create", json.RawMessage(`{"input":{"id":"reader","name":"Reader"}}`)); err != nil {
		t.Fatal(err)
	}
	raw, err := control.call(ctx, "capabilities.get", json.RawMessage(`{"botId":"reader","resources":{"pluginDirectories":["untrusted"],"mcpServers":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Entries struct {
			Toolset []struct {
				ID string `json:"id"`
			} `json:"toolset"`
			MCP []struct {
				ID string `json:"id"`
			} `json:"mcp"`
		} `json:"entries"`
		DefaultModelChain []struct {
			ProviderID string `json:"providerId"`
			Model      string `json:"model"`
			Effort     string `json:"effort"`
		} `json:"defaultModelChain"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries.Toolset) != 1 || result.Entries.Toolset[0].ID != "optional-tools" || len(result.Entries.MCP) != 1 || result.Entries.MCP[0].ID != "private" {
		t.Fatalf("catalog did not use Host resources: %s", raw)
	}
	if len(result.DefaultModelChain) != 1 || result.DefaultModelChain[0].ProviderID != "fixture" || result.DefaultModelChain[0].Model != "model" || result.DefaultModelChain[0].Effort != "low" {
		t.Fatalf("incorrect default model route: %s", raw)
	}
	if strings.Contains(string(raw), "PRIVATE_CREDENTIAL") || strings.Contains(string(raw), "private.example.test") {
		t.Fatal("connection secrets escaped through catalog")
	}
	if _, err = control.call(ctx, "capabilities.query", json.RawMessage(`{"botId":"reader"}`)); err == nil {
		t.Fatal("private inspection method was exposed")
	}
}

func TestChatResourceSaveInvalidatesExistingOneBotRoutes(t *testing.T) {
	directory := t.TempDir()
	store, err := chat.Open(filepath.Join(directory, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	botStore, err := chat.Open(filepath.Join(directory, "onebot", "chat"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := botStore.NewConversation("", directory, "Existing OneBot", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pid := uint32(os.Getpid())
	peer, err := processidentity.Open(pid, executable)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	control := &chatControl{ctx: context.Background(), store: store, owners: map[uint32]*processidentity.Process{pid: peer}}
	bot := newChatOneBot(control)
	control.onebot = bot
	bot.sessions = &chatControl{store: botStore, engine: chat.NewService(context.Background(), botStore, chat.ServiceOptions{})}
	params, err := json.Marshal(map[string]any{"settings": store.Preferences()})
	if err != nil {
		t.Fatal(err)
	}
	response := control.handle(host.Request{Method: "settings.save", Params: params, PeerPID: pid})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	if bot.refresh[conversation.ID] == 0 {
		t.Fatal("saved resource choices did not invalidate an existing OneBot runtime")
	}
}

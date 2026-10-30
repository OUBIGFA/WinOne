//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"

	"winone/internal/chat"
)

// Catalog reads use the registered Host resources, never renderer-supplied
// paths, commands or headers. The Worker returns display metadata only and
// cannot execute plugins or start MCP connections on this path.
func (b *chatOneBot) capabilityCatalog(ctx context.Context, runner *chat.Runner, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		BotID string `json:"botId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if b.control.store == nil {
		return nil, errors.New("WinOne 能力目录尚未就绪")
	}
	prefs := b.control.store.Preferences()
	resources := map[string]any{"agentDir": filepath.Join(b.control.directory, "chat", "agents", "onebot-catalog"), "skillDirectories": prefs.SkillDirectories, "pluginDirectories": prefs.PluginDirectories, "mcpServers": prefs.MCPServers}
	value, err := runner.Call(ctx, "capabilities.query", map[string]any{"botId": input.BotID, "resources": resources})
	if err != nil {
		return nil, err
	}
	var result map[string]json.RawMessage
	if err = json.Unmarshal(value, &result); err != nil {
		return nil, err
	}
	defaults := []oneBotModelRoute{}
	if b.control.ai != nil {
		configuration, err := b.control.aiConfiguration()
		if err != nil {
			return nil, err
		}
		for _, route := range b.defaultModelChain(configuration) {
			if route.Model != "" {
				defaults = append(defaults, route)
			}
		}
	}
	result["defaultModelChain"], err = json.Marshal(defaults)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// Saving shared resource preferences invalidates existing companion routes,
// but does not start a OneBot worker merely because Chat settings were saved.
func (b *chatOneBot) resourcesChanged() {
	b.startMu.Lock()
	sessions := b.sessions
	b.startMu.Unlock()
	if sessions != nil {
		b.refreshMu.Lock()
		for _, conversation := range sessions.store.Snapshot().Conversations {
			b.refresh[conversation.ID]++
		}
		b.refreshMu.Unlock()
	}
	b.publishEvent("capabilities-changed", "")
}

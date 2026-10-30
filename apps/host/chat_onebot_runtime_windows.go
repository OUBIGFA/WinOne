//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"winone/internal/ai"
	"winone/internal/chat"
)

type oneBotRuntimeCandidate struct {
	Revision      string
	Plan          oneBotModelPlan
	Permission    string
	Applied       bool
	Context       oneBotContext
	ActiveContext oneBotContext
}

// Supply current Host facts through the conversation's private bridge. The
// caller cannot choose another Bot, resource path, credential or model catalog.
func (b *chatOneBot) runtimeConfiguration(ctx context.Context, id string) (map[string]any, error) {
	b.startMu.Lock()
	sessions, stopped := b.sessions, b.stopped
	b.startMu.Unlock()
	if stopped || sessions == nil {
		return nil, errors.New("OneBot 正在退出")
	}
	conversation, err := sessions.store.Conversation(id)
	if err != nil {
		return nil, err
	}
	value, err := b.contextFor(ctx, map[string]string{"sessionId": id})
	if err != nil {
		return nil, err
	}
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return nil, err
	}
	prefs := b.control.store.Preferences()
	value.DefaultModelChain = b.defaultModelChain(configuration)
	plan, err := resolveOneBotModel(value, configuration, conversation)
	if err != nil {
		return nil, err
	}
	model, err := chatModel(configuration, plan.Model)
	if err != nil {
		return nil, err
	}
	models := []map[string]any{}
	for _, provider := range configuration.Providers {
		for _, modelID := range provider.Models {
			entry, err := chatModel(configuration, ai.Selection{Provider: provider.ID, Model: modelID})
			if err != nil {
				return nil, err
			}
			models = append(models, entry)
		}
	}
	permission := oneBotPermission(value.Capabilities.Permissions)
	patch := map[string]any{"model": model, "models": models, "thinkingLevel": plan.ThinkingLevel, "fastMode": plan.FastMode, "permission": permission,
		"skillDirectories": prefs.SkillDirectories, "pluginDirectories": prefs.PluginDirectories, "mcpServers": prefs.MCPServers,
		"onebot": map[string]any{"botId": value.BotID, "homeDir": value.HomeDir, "systemPrompt": value.SystemPrompt, "memoryEnabled": value.MemoryEnabled, "channelScoped": value.Role == "channel", "receiptScoped": true, "profileVersion": value.ProfileVersion, "modelChain": oneBotResolvedModelChain(plan, configuration), "modelChainKey": plan.Key}}
	applyOneBotResources(patch, value)
	raw, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	revision := hex.EncodeToString(digest[:])
	b.startMu.Lock()
	defer b.startMu.Unlock()
	if b.stopped || b.sessions != sessions {
		return nil, errors.New("OneBot 运行实例已改变")
	}
	b.refreshMu.Lock()
	b.runtimeCandidates[id] = oneBotRuntimeCandidate{Revision: revision, Plan: plan, Permission: permission, Context: value, ActiveContext: b.runtimeCandidates[id].ActiveContext}
	b.refreshMu.Unlock()
	return map[string]any{"revision": revision, "config": patch}, nil
}

// Metadata moves only after the Worker has configured the corresponding
// runtime. An acknowledged duplicate does not rewrite the conversation.
func (b *chatOneBot) runtimeApplied(id, revision string, resources json.RawMessage) error {
	b.startMu.Lock()
	defer b.startMu.Unlock()
	if b.stopped || b.sessions == nil {
		return errors.New("OneBot 正在退出")
	}
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	candidate, ok := b.runtimeCandidates[id]
	if !ok || candidate.Revision != revision {
		return errors.New("OneBot 运行时回执已过期")
	}
	if candidate.Applied {
		return nil
	}
	if err := b.sessions.store.RememberResources(id, resources); err != nil {
		return err
	}
	_, err := b.sessions.store.UpdateConversation(id, func(c *chat.Conversation) error {
		c.Model, c.ThinkingLevel, c.FastMode, c.Permission, c.OneBotModelKey = candidate.Plan.Model, candidate.Plan.ThinkingLevel, candidate.Plan.FastMode, candidate.Permission, candidate.Plan.Key
		return nil
	})
	if err != nil {
		return err
	}
	candidate.Applied = true
	candidate.ActiveContext = candidate.Context
	b.runtimeCandidates[id] = candidate
	return nil
}

func (b *chatOneBot) activeRuntimeContext(id string, initial oneBotContext) oneBotContext {
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	if candidate := b.runtimeCandidates[id]; candidate.ActiveContext.BotID != "" {
		return candidate.ActiveContext
	}
	return initial
}

// A fallback acknowledges an actual Worker selection from its active profile
// snapshot. A delayed acknowledgement cannot replace a newer user selection.
func (b *chatOneBot) runtimeModelApplied(id, previous, selected, key string, active oneBotContext) error {
	b.startMu.Lock()
	defer b.startMu.Unlock()
	if b.stopped || b.sessions == nil {
		return errors.New("OneBot 正在退出")
	}
	current, err := b.sessions.store.Conversation(id)
	if err != nil {
		return err
	}
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return err
	}
	plan, err := resolveOneBotModel(active, configuration, current)
	if err != nil {
		return err
	}
	if plan.Key != key || current.OneBotModelKey != key {
		return errors.New("OneBot 模型选择回执已过期")
	}
	for _, route := range plan.Chain[oneBotModelRouteIndex(plan.Chain, current.Model)+1:] {
		model, fast, err := oneBotRouteSelection(route, configuration)
		if err != nil || model.Provider+"/"+model.Model != selected {
			continue
		}
		_, err = b.sessions.store.UpdateConversation(id, func(c *chat.Conversation) error {
			if c.OneBotModelKey != key || c.Model.Provider+"/"+c.Model.Model != previous {
				return errors.New("OneBot 模型选择回执已过期")
			}
			c.Model, c.ThinkingLevel, c.FastMode = model, route.Effort, fast
			return nil
		})
		return err
	}
	return errors.New("OneBot 模型切换必须使用配置链中的后续可用路线")
}

// Deliver invalidations to existing workers without restarting running tools.
// Cold sessions keep their marker and initialize from current Host settings.
func (b *chatOneBot) flushRuntimeRefresh(ctx context.Context) error {
	b.startMu.Lock()
	sessions, stopped := b.sessions, b.stopped
	b.startMu.Unlock()
	if stopped || sessions == nil {
		return nil
	}
	b.refreshMu.Lock()
	pending := make(map[string]uint64, len(b.refresh))
	for id, revision := range b.refresh {
		pending[id] = revision
	}
	b.refreshMu.Unlock()
	for id, revision := range pending {
		accepted, err := sessions.engine.RequestRuntimeRefresh(ctx, id)
		if err != nil {
			return fmt.Errorf("OneBot 设置待应用：%w", err)
		}
		if accepted {
			b.refreshMu.Lock()
			if b.refresh[id] == revision {
				delete(b.refresh, id)
			}
			b.refreshMu.Unlock()
		}
	}
	return nil
}

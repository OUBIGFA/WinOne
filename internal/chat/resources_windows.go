//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
)

// Only an already-open worker accepts this private host notification. Its
// Durable run boundary owns the deferred swap; no model or tool is restarted
// here while a request is executing.
func (s *Service) RequestRuntimeRefresh(ctx context.Context, id string) (bool, error) {
	s.mu.Lock()
	p, stopped := s.sessions[id], s.paused
	s.mu.Unlock()
	if stopped || p == nil {
		return false, nil
	}
	if err := p.lock(ctx); err != nil {
		return false, err
	}
	defer p.unlock()
	if p.closed || !p.initialized || p.runner == nil {
		return false, nil
	}
	_, err := p.runner.Call(ctx, "runtime.refresh", nil)
	return err == nil, err
}

// Resources and the visible transcript share the same Open barrier. A sidebar
// can mount before the asynchronous conversation open has finished, or after
// an idle runtime was released; neither means the saved conversation is absent.
func (s *Service) Resources(ctx context.Context, id string, owner uint32) (json.RawMessage, error) {
	raw, err := s.Open(ctx, id, owner)
	if err != nil {
		return nil, err
	}
	if owner == 0 {
		defer s.Release(id, owner)
	}
	var state struct {
		Resources  json.RawMessage `json:"resources"`
		ModelError string          `json:"modelError"`
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	if len(state.Resources) == 0 || string(state.Resources) == "null" {
		// A removed/expired provider must not turn local AGENTS, skills and
		// plugin manifests into an unavailable pane. Inspect them without
		// loading executable extensions or starting any MCP server.
		conversation, err := s.store.Conversation(id)
		if err != nil {
			return nil, err
		}
		live, err := s.session(id)
		if err != nil {
			return nil, err
		}
		if err = live.lock(ctx); err != nil {
			return nil, err
		}
		defer live.unlock()
		if live.runner == nil {
			return nil, errors.New("对话资源读取器尚未就绪")
		}
		prefs := s.store.Preferences()
		config := map[string]any{"cwd": conversation.Directory, "agentDir": filepath.Join(s.store.root, "agents", id), "skillDirectories": prefs.SkillDirectories, "pluginDirectories": prefs.PluginDirectories, "mcpServers": prefs.MCPServers}
		if s.options.ResourceConfig != nil {
			config, err = s.options.ResourceConfig(ctx, conversation)
			if err != nil {
				return nil, err
			}
		}
		resources, err := live.runner.Call(ctx, "resources.inspect", config)
		if err != nil {
			return nil, err
		}
		if err = s.store.RememberResources(id, resources); err != nil {
			return nil, err
		}
		return resources, nil
	}
	// Open already validates and registers the exact read-only resource paths.
	return state.Resources, nil
}

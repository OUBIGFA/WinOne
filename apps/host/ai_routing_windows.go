//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"winone/internal/ai"
	"winone/internal/host"
)

type routingControl struct {
	mu      sync.Mutex
	process *ai.RoutingProcess
	cancel  context.CancelFunc
	viewers map[string]bool
}

func (s *aiService) routingEnabled() bool {
	store, err := s.magpieStore()
	return err == nil && store.Configuration().RoutingEnabled
}

func (s *aiService) startRouting() (*ai.RoutingProcess, error) {
	s.routing.mu.Lock()
	defer s.routing.mu.Unlock()
	if s.routing.process != nil {
		if s.routing.process.Alive() {
			return s.routing.process, nil
		}
		s.routing.cancel()
		s.routing.process.Close()
		s.routing.process = nil
	}
	store, err := s.magpieStore()
	if err != nil {
		return nil, err
	}
	prefs, err := store.MagpieRuntime()
	if err != nil {
		return nil, err
	}
	providers, err := store.MagpieProviders()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(s.ctx)
	process, err := s.translator.StartRouting(ctx, prefs, providers)
	if err != nil {
		cancel()
		return nil, err
	}
	s.routing.process = process
	s.routing.cancel = cancel
	return process, nil
}
func (s *aiService) closeRouting() {
	s.routing.mu.Lock()
	defer s.routing.mu.Unlock()
	if s.routing.process != nil {
		s.routing.cancel()
		s.routing.process.Close()
		s.routing.process = nil
	}
}
func (s *aiService) handleRouting(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "routing", Message: err.Error()}}
	}
	viewer := "routing"
	if strings.HasPrefix(r.Method, "routing.plugins.") {
		viewer = "plugins"
	}
	if strings.HasPrefix(r.Method, "routing.extensions.") {
		var input struct {
			Viewer string `json:"viewer"`
		}
		if json.Unmarshal(r.Params, &input) != nil || len(input.Viewer) < 1 || len(input.Viewer) > 80 || strings.Trim(input.Viewer, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-") != "" {
			return fail(errors.New("插件页面标识无效"))
		}
		// Each settings owner has a separate lease; a stale open/close response
		// cannot stop another provider panel or a newly reopened settings page.
		viewer = "extensions:" + input.Viewer
	}
	switch r.Method {
	case "routing.open", "routing.plugins.open", "routing.extensions.open":
		s.routing.mu.Lock()
		if s.routing.viewers == nil {
			s.routing.viewers = map[string]bool{}
		}
		s.routing.viewers[viewer] = true
		s.routing.mu.Unlock()
		process, err := s.startRouting()
		if err != nil {
			s.routing.mu.Lock()
			delete(s.routing.viewers, viewer)
			s.routing.mu.Unlock()
			return fail(err)
		}
		if r.Method == "routing.extensions.open" {
			ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
			defer cancel()
			state, err := process.PluginWindowState(ctx)
			if err != nil {
				s.releaseRoutingViewer(viewer)
				return fail(err)
			}
			raw, _ := json.Marshal(state)
			return host.Response{ID: r.ID, Result: raw}
		}
		pageURL := process.PageURL()
		if viewer == "plugins" {
			pageURL = process.PluginsPageURL()
		}
		raw, _ := json.Marshal(map[string]any{"url": pageURL, "enabled": s.routingEnabled()})
		return host.Response{ID: r.ID, Result: raw}
	case "routing.close", "routing.plugins.close", "routing.extensions.close":
		s.releaseRoutingViewer(viewer)
		return host.Response{ID: r.ID, Result: json.RawMessage("{}")}
	case "routing.enable", "routing.disable":
		return fail(errors.New("请在 AI 服务中保存路由开关"))
	default:
		return fail(errors.New("不支持的路由操作"))
	}
}

// All embedded surfaces share the runtime, not each other's view/lifetime.
func (s *aiService) releaseRoutingViewer(viewer string) {
	enabled := s.routingEnabled()
	s.routing.mu.Lock()
	defer s.routing.mu.Unlock()
	delete(s.routing.viewers, viewer)
	if !enabled && len(s.routing.viewers) == 0 && s.routing.process != nil {
		s.routing.cancel()
		s.routing.process.Close()
		s.routing.process = nil
	}
}

func (s *aiService) serveRoutedModel(w http.ResponseWriter, r *http.Request, path string, body []byte, model string, allow func(ai.Selection) bool) {
	process, err := s.startRouting()
	if err != nil {
		ai.GatewayError(w, 503, "路由服务未能启动")
		return
	}
	permitted, err := process.AllowsModel(r.Context(), model, allow)
	if err != nil {
		ai.GatewayError(w, 503, "路由模型状态读取失败")
		return
	}
	if !permitted {
		ai.GatewayError(w, 403, "路由模型或分组成员未在本地网关中启用")
		return
	}
	select {
	case s.gatewaySlots <- struct{}{}:
		defer func() { <-s.gatewaySlots }()
	default:
		ai.GatewayError(w, 429, "本地网关请求较多，请稍后重试")
		return
	}
	process.ServeBody(w, r, path, body)
}

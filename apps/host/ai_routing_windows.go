//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"winone/internal/ai"
	"winone/internal/host"
)

type routingControl struct {
	mu      sync.Mutex
	process *ai.RoutingProcess
	cancel  context.CancelFunc
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
	switch r.Method {
	case "routing.open":
		process, err := s.startRouting()
		if err != nil {
			return fail(err)
		}
		raw, _ := json.Marshal(map[string]any{"url": process.PageURL(), "enabled": s.routingEnabled()})
		return host.Response{ID: r.ID, Result: raw}
	case "routing.close":
		if !s.routingEnabled() {
			s.closeRouting()
		}
		return host.Response{ID: r.ID, Result: json.RawMessage("{}")}
	case "routing.enable", "routing.disable":
		return fail(errors.New("请在 AI 服务中保存路由开关"))
	default:
		return fail(errors.New("不支持的路由操作"))
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

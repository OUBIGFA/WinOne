//go:build windows

package main

import (
	"net/http"
	"strings"
	"winone/internal/ai"
	"winone/internal/chat"
)

// The existing Magpie service owns asynchronous jobs, image files, videos and
// MCP protocol state. A one-shot worker would lose them when tools/call ends.
func (s *aiService) serveMediaMCP(w http.ResponseWriter, r *http.Request, store *ai.Store, allow func(ai.Selection) bool) {
	models := []string{}
	config := store.Configuration()
	for _, provider := range config.Providers {
		if provider.Off || provider.AuthMode == "account" && config.MagpieSettings.NoAccountDiscovery {
			continue
		}
		for _, model := range provider.Models {
			if allow(ai.Selection{Provider: provider.ID, Model: model}) {
				models = append(models, provider.ID+"/"+model)
				if alias := provider.ModelMetadata[model].Alias; alias != "" {
					models = append(models, provider.ID+"/"+alias)
				}
			}
		}
	}
	select {
	case s.gatewaySlots <- struct{}{}:
		defer func() { <-s.gatewaySlots }()
	default:
		ai.GatewayError(w, 429, "本地网关请求较多，请稍后重试")
		return
	}
	s.routing.mu.Lock()
	if s.routing.viewers == nil {
		s.routing.viewers = map[string]bool{}
	}
	s.routing.viewers["gateway-media"] = true
	s.routing.mu.Unlock()
	process, err := s.startRouting()
	if err != nil {
		s.releaseRoutingViewer("gateway-media")
		ai.GatewayError(w, 503, "图像 MCP 服务无法启动")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, chat.MaximumChatRequestBytes)
	if strings.HasPrefix(r.URL.Path, "/v1/magpie/native/") {
		// A vendor's arbitrary API cannot be checked against a model list.
		// Require its own opt-in; upstream still refuses scoped/budgeted keys,
		// account providers and unsafe paths and applies privacy protection.
		if !s.gateway.Status().Native {
			ai.GatewayError(w, 403, "请先在本地网关的高级选项中开启供应商原生 API")
			return
		}
		process.ServeHTTP(w, r)
		return
	}
	groups, err := process.GatewayGroups(r.Context(), allow, config.RoutingEnabled)
	if err != nil {
		ai.GatewayError(w, 503, "路由模型状态读取失败")
		return
	}
	models = append(models, groups...)
	if len(models) == 0 {
		models = []string{"/"}
	}
	process.ServeMedia(w, r, models)
}

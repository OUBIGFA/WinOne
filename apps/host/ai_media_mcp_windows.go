//go:build windows

package main

import (
	"net/http"
	"winone/internal/ai"
	"winone/internal/chat"
)

// The existing Magpie service owns asynchronous jobs, image files, videos and
// MCP protocol state. A one-shot worker would lose them when tools/call ends.
func (s *aiService) serveMediaMCP(w http.ResponseWriter, r *http.Request, store *ai.Store, allow func(ai.Selection) bool) {
	models := []string{}
	config := store.Configuration()
	for _, provider := range config.Providers {
		if provider.AuthMode == "account" && config.MagpieSettings.NoAccountDiscovery {
			continue
		}
		for _, model := range provider.Models {
			if allow(ai.Selection{Provider: provider.ID, Model: model}) {
				models = append(models, provider.ID+"/"+model)
			}
		}
	}
	if len(models) == 0 {
		ai.GatewayError(w, 403, "本地网关没有启用可用模型")
		return
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
	process.ServeMedia(w, r, models)
}

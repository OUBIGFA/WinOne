//go:build windows

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"winone/internal/ai"
)

// Keep Magpie's full Ollama response/metadata implementation. The Host only
// intersects discovery with the same model policy used for actual requests;
// a hidden group or disabled model cannot reappear through a second protocol.
func (s *aiService) serveOllama(w http.ResponseWriter, r *http.Request, configuration ai.Configuration, allow func(ai.Selection) bool) {
	fail := func(status int, message string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
	}
	path := r.URL.Path
	if r.Method != http.MethodGet && path != "/api/show" || path == "/api/show" && r.Method != http.MethodPost {
		fail(405, "接口不支持此请求方式")
		return
	}
	process, err := s.startRouting()
	if err != nil {
		fail(503, "AI 模型目录服务无法启动")
		return
	}
	if path == "/api/version" {
		process.ServeHTTP(w, r)
		return
	}
	allowed := map[string]bool{}
	if configuration.RoutingEnabled {
		raw, err := process.Models(r.Context(), allow)
		if err != nil {
			fail(503, "路由模型目录读取失败")
			return
		}
		var list struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &list) != nil {
			fail(503, "路由模型目录无效")
			return
		}
		for _, model := range list.Data {
			allowed[model.ID] = true
		}
	} else {
		for _, provider := range configuration.Providers {
			if provider.AuthMode == "account" && configuration.MagpieSettings.NoAccountDiscovery {
				continue
			}
			for _, model := range provider.Models {
				if allow(ai.Selection{Provider: provider.ID, Model: model}) {
					allowed[provider.ID+"/"+model] = true
				}
			}
		}
	}
	if path == "/api/tags" {
		raw, err := process.OllamaTags(r.Context(), allowed)
		if err != nil {
			fail(503, "Ollama 模型目录读取失败")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		fail(413, "请求内容过大")
		return
	}
	var input struct {
		Model string `json:"model"`
		Name  string `json:"name"`
	}
	if json.Unmarshal(body, &input) != nil {
		fail(400, "请求必须为 JSON")
		return
	}
	id := strings.TrimSpace(input.Model)
	if id == "" {
		id = strings.TrimSpace(input.Name)
	}
	if id == "" {
		fail(400, "请求须包含模型 ID")
		return
	}
	if !allowed[id] {
		fail(404, "模型不存在或未在本地网关启用")
		return
	}
	process.ServeBody(w, r, path, body)
}

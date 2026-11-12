//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/host"
)

// The request limit belongs to the gateway, independently of any product UI.
const maximumGatewayRequestBytes = 16 << 20

func (s *aiService) initGateway() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gateway = ai.NewGateway(s.ctx, filepath.Join(filepath.Dir(s.storePath), "ai-gateway.json"), http.HandlerFunc(s.serveGateway), http.HandlerFunc(s.serveNamedGateway))
}

func (s *aiService) handleGateway(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai_gateway", Message: err.Error()}}
	}
	var result any
	switch r.Method {
	case "gateway.open", "gateway.close":
		var input struct {
			Viewer string `json:"viewer"`
		}
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return fail(err)
		}
		if len(input.Viewer) < 16 || len(input.Viewer) > 128 || strings.ContainsAny(input.Viewer, "/\\\x00") {
			return fail(errors.New("网关窗口标识无效"))
		}
		if r.Method == "gateway.close" {
			s.releaseGatewayBackend(input.Viewer)
			result = map[string]bool{"closed": true}
			break
		}
		process, err := s.openGatewayBackend(input.Viewer, r.PeerPID)
		if err != nil {
			return fail(err)
		}
		result = map[string]string{"url": process.AdminURL()}
	case "gateway.get":
		result = s.gateway.Status()
	case "gateway.key":
		key, err := s.gateway.Key()
		if err != nil {
			return fail(err)
		}
		result = map[string]string{"key": key}
	case "gateway.save":
		var input struct {
			Settings ai.GatewaySettings `json:"settings"`
			ResetKey bool               `json:"resetKey"`
		}
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return fail(err)
		}
		value, err := s.gateway.Save(input.Settings, input.ResetKey)
		if err != nil {
			return fail(err)
		}
		result = value
		if !value.Enabled {
			s.releaseGatewayBackend("")
		}
	default:
		return fail(errors.New("不支持的本地网关操作"))
	}
	data, _ := json.Marshal(result)
	return host.Response{ID: r.ID, Result: data}
}

// Both credentials use one gateway backend, with the Host's enabled-model
// boundary and upstream named-key limits. No Chat or route-group service runs.
func (s *aiService) serveGateway(w http.ResponseWriter, r *http.Request) {
	s.serveConfiguredGateway(w, r, false)
}
func (s *aiService) serveNamedGateway(w http.ResponseWriter, r *http.Request) {
	s.serveConfiguredGateway(w, r, true)
}

func (s *aiService) serveConfiguredGateway(w http.ResponseWriter, r *http.Request, named bool) {
	if !ai.IsGatewayServicePath(r.URL.Path) {
		ai.GatewayError(w, 404, "不支持的本地网关接口")
		return
	}
	// Preserve Ollama's discovery contract before the generic protocol mux's
	// catch-all route. These paths have exactly one supported HTTP method.
	switch r.URL.Path {
	case "/api/version", "/api/tags":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			ai.GatewayError(w, 405, "此接口只支持 GET")
			return
		}
	case "/api/show":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			ai.GatewayError(w, 405, "此接口只支持 POST")
			return
		}
	}
	s.mu.Lock()
	guard, gateway := s.gatewayGuard, s.gateway
	s.mu.Unlock()
	if guard != nil {
		release, ok := guard()
		if !ok {
			ai.GatewayError(w, 503, "WinOne 正在维护，请稍后重试")
			return
		}
		defer release()
	}
	store, err := s.magpieStore()
	if err != nil {
		ai.GatewayError(w, 503, "AI 服务配置无法读取")
		return
	}
	config := store.Configuration()
	allow := gateway.ModelFilter()
	if strings.HasPrefix(r.URL.Path, "/v1/magpie/native/") && !gateway.Status().Native {
		ai.GatewayError(w, 403, "请先开启供应商原生 API")
		return
	}
	if !named && r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		// Saved discovery requires neither a worker nor live vendor credentials.
		entries := []map[string]any{}
		for _, p := range config.Providers {
			if p.Off {
				continue
			}
			for _, model := range p.Models {
				if !allow(ai.Selection{Provider: p.ID, Model: model}) {
					continue
				}
				metadata := p.ModelMetadataFor(model)
				id := p.ID + "/" + model
				if metadata.Alias != "" {
					id = p.ID + "/" + metadata.Alias
				}
				name := p.ModelName(model)
				if !strings.Contains(strings.ToLower(name), strings.ToLower(p.Name)) {
					name += " · " + p.Name
				}
				entry := map[string]any{"id": id, "slug": id, "object": "model", "created": 0, "owned_by": p.Name, "name": name, "display_name": name, "metadata": metadata}
				if metadata.ContextWindow > 0 {
					window := metadata.ContextWindow
					threshold := metadata.CompactionThreshold
					if metadata.CompactionPercent > 0 {
						threshold = int(float64(window) * metadata.CompactionPercent / 100)
					}
					if threshold > 0 && threshold < window {
						entry["max_context_window"] = window
						window = threshold
					}
					entry["context_window"] = window
				}
				if metadata.MaxTokens > 0 {
					entry["max_tokens"] = metadata.MaxTokens
				}
				if len(metadata.Efforts) > 0 {
					levels := []map[string]string{}
					for _, effort := range metadata.Efforts {
						levels = append(levels, map[string]string{"effort": effort})
					}
					entry["supported_reasoning_levels"] = levels
				}
				entries = append(entries, entry)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": entries})
		return
	}
	models := []string{}
	for _, provider := range config.Providers {
		if provider.Off {
			continue
		}
		for _, model := range provider.Models {
			if !allow(ai.Selection{Provider: provider.ID, Model: model}) {
				continue
			}
			models = append(models, provider.ID+"/"+model)
			if alias := provider.ModelMetadata[model].Alias; alias != "" {
				models = append(models, provider.ID+"/"+alias)
			}
		}
	}
	// An empty explicit list must deny every model, never mean unrestricted.
	if len(models) == 0 {
		models = []string{"/"}
	}
	limit := int64(maximumGatewayRequestBytes)
	if r.URL.Path == "/api/show" {
		limit = 1 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	// Reject disabled JSON model calls before loading any account runtime. Named
	// keys still authenticate inside their owner before their policy is applied.
	if !named && r.Method == http.MethodPost && (strings.Contains(r.Header.Get("Content-Type"), "json") || r.URL.Path == "/v1/chat/completions" || r.URL.Path == "/v1/responses" || r.URL.Path == "/v1/messages") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			ai.GatewayError(w, 413, "请求内容过大")
			return
		}
		var input struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &input) != nil {
			ai.GatewayError(w, 400, "请求必须为 JSON")
			return
		}
		if input.Model != "" && r.URL.Path != "/api/show" {
			permitted := false
			for _, model := range models {
				permitted = permitted || model == input.Model
			}
			if !permitted {
				ai.GatewayError(w, 403, "模型未在本地网关中启用")
				return
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	select {
	case s.gatewaySlots <- struct{}{}:
		defer func() { <-s.gatewaySlots }()
	default:
		ai.GatewayError(w, 429, "本地网关请求较多，请稍后重试")
		return
	}
	process, err := s.startGatewayBackend()
	if err != nil {
		ai.GatewayError(w, 503, "本地网关服务无法启动")
		return
	}
	process.ServeMedia(w, r, models)
}

//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"winone/internal/ai"
	"winone/internal/chat"
	"winone/internal/configjson"
	"winone/internal/host"
)

func (s *aiService) initGateway() {
	// A saved gateway may start serving immediately. Publish it before the
	// handler can capture its model policy or configuration guard.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gateway = ai.NewGateway(s.ctx, filepath.Join(filepath.Dir(s.storePath), "ai-gateway.json"), http.HandlerFunc(s.serveGateway))
}
func (s *aiService) handleGateway(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai_gateway", Message: err.Error()}}
	}
	var result any
	switch r.Method {
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
	default:
		return fail(errors.New("不支持的本地网关操作"))
	}
	data, _ := json.Marshal(result)
	return host.Response{ID: r.ID, Result: data}
}

func (s *aiService) serveGateway(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	guard := s.gatewayGuard
	gateway := s.gateway
	s.mu.Unlock()
	allowModel := gateway.ModelFilter()
	s.serveAIProtocol(w, r, guard, allowModel, false)
}

// Chat uses the enabled AI service catalog, independently of the public gateway.
// The per-worker private listener performs authentication before reaching here.
func (s *aiService) serveChat(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	guard := s.gatewayGuard
	s.mu.Unlock()
	s.serveAIProtocol(w, r, guard, func(ai.Selection) bool { return true }, true)
}

func (s *aiService) serveAIProtocol(w http.ResponseWriter, r *http.Request, guard func() (func(), bool), allowModel func(ai.Selection) bool, inProduct bool) {
	if guard != nil {
		release, ok := guard()
		if !ok {
			ai.GatewayError(w, 503, "WinOne 正在维护，请稍后重试")
			return
		}
		defer release()
	}
	s.mu.Lock()
	var err error
	if s.store == nil {
		s.store, err = ai.Open(s.storePath)
	}
	store := s.store
	s.mu.Unlock()
	if err != nil {
		ai.GatewayError(w, 503, "AI 服务配置无法读取")
		return
	}
	if r.URL.Path == "/api/version" || r.URL.Path == "/api/tags" || r.URL.Path == "/api/show" {
		s.serveOllama(w, r, store.Configuration(), allowModel)
		return
	}
	if r.Method == http.MethodGet && (r.URL.Path == "/v1/models" || r.URL.Path == "/models") {
		if store.Configuration().RoutingEnabled {
			process, err := s.startRouting()
			if err != nil {
				ai.GatewayError(w, 503, "路由服务未能启动")
				return
			}
			models, err := process.Models(r.Context(), allowModel)
			if err != nil {
				ai.GatewayError(w, 503, "路由模型读取失败")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(models)
			return
		}
		models := []map[string]any{}
		configuration := store.Configuration()
		settings := configuration.MagpieSettings
		for _, p := range configuration.Providers {
			if p.AuthMode == "account" && settings.NoAccountDiscovery {
				continue
			}
			for _, model := range p.Models {
				if !allowModel(ai.Selection{Provider: p.ID, Model: model}) {
					continue
				}
				metadata := p.ModelMetadataFor(model)
				entry := map[string]any{"id": p.ID + "/" + model, "object": "model", "created": 0, "owned_by": p.Name, "name": p.ModelName(model), "metadata": metadata}
				name := p.ModelName(model)
				if !settings.PlainNames && !(settings.PlainOwnNames && p.ModelNames[model] != "") && !strings.Contains(strings.ToLower(name), strings.ToLower(p.Name)) {
					name += " · " + p.Name
				}
				entry["name"], entry["display_name"], entry["slug"] = name, name, p.ID+"/"+model
				if settings.CodexAgentsV1 && p.Agent == "codex" {
					entry["multi_agent_version"] = "v1"
				}
				if settings.CodexAutoReview != "" {
					entry["auto_review_model_override"] = settings.CodexAutoReview
				}
				if len(metadata.Efforts) > 0 {
					levels := []map[string]string{}
					for _, effort := range metadata.Efforts {
						levels = append(levels, map[string]string{"effort": effort})
					}
					entry["supported_reasoning_levels"] = levels
				}
				// Unknown limits stay absent, rather than advertising a zero-sized
				// context. Preserve explicit false capabilities inside metadata.
				if metadata.ContextWindow > 0 {
					window := metadata.ContextWindow
					threshold := metadata.CompactionThreshold
					if metadata.CompactionPercent > 0 {
						threshold = int(float64(window) * metadata.CompactionPercent / 100)
					}
					if threshold == 0 && !settings.FullContext {
						threshold = settings.CompactAt
						if threshold == 0 {
							threshold = 272000
						}
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
				models = append(models, entry)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": models})
		return
	}
	path := r.URL.Path
	if !strings.HasPrefix(path, "/v1/") && path != "/mcp/magpie/web-search" && path != "/api/chat" && path != "/api/generate" {
		path = "/v1" + path
	}
	supported := false
	for _, route := range []string{"/api/chat", "/api/generate", "/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/messages/count_tokens", "/v1/images/generations", "/v1/embeddings", "/v1/rerank", "/mcp/magpie/web-search"} {
		supported = supported || path == route
	}
	if r.Method != http.MethodPost || !supported {
		ai.GatewayError(w, 404, "不支持的本地网关接口")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, chat.MaximumChatRequestBytes))
	if err != nil {
		ai.GatewayError(w, 413, "请求内容过大")
		return
	}
	if path == "/mcp/magpie/web-search" {
		s.serveMagpieProtocol(w, r, store, path, body, allowModel)
		return
	}
	var request struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &request) != nil {
		ai.GatewayError(w, 400, "请求必须为 JSON 并包含模型 ID")
		return
	}
	if path == "/v1/images/generations" && request.Model == "" {
		s.serveMagpieProtocol(w, r, store, path, body, allowModel)
		return
	}
	if store.Configuration().RoutingEnabled {
		s.serveRoutedModel(w, r, path, body, request.Model, allowModel)
		return
	}
	provider, model, found := strings.Cut(request.Model, "/")
	if !found {
		ai.GatewayError(w, 400, "请使用模型列表中的完整模型 ID")
		return
	}
	prefs, keys, err := store.ForTaskKeys("chat", &ai.Selection{Provider: provider, Model: model})
	if err != nil {
		ai.GatewayError(w, 400, "模型未启用或已移除，请刷新模型列表")
		return
	}
	if !allowModel(ai.Selection{Provider: provider, Model: model}) {
		ai.GatewayError(w, 403, "模型未在本地网关中启用")
		return
	}
	// Limit workers and isolate each account exactly like in-product requests.
	select {
	case s.gatewaySlots <- struct{}{}:
		defer func() { <-s.gatewaySlots }()
	default:
		ai.GatewayError(w, 429, "本地网关请求较多，请稍后重试")
		return
	}
	runtime := s.translator
	if prefs.AuthMode == "account" {
		gate := s.accountGateFor(prefs.ProviderID)
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		default:
			ai.GatewayError(w, 409, "账号正在登录或使用，请稍后重试")
			return
		}
		runtime = runtime.ForProvider(prefs.ProviderID)
	}
	ctx, cancel := context.WithTimeout(r.Context(), prefs.MagpieSettings.GatewayWait(path))
	defer cancel()
	headers := make(http.Header)
	for _, name := range []string{"Anthropic-Version", "Anthropic-Beta", "Openai-Beta", "User-Agent", "Originator", "X-OpenAI-Subagent"} {
		if value := r.Header.Get(name); value != "" {
			headers.Set(name, value)
		}
	}
	if inProduct {
		headers.Set("X-WinOne-Session", r.Header.Get("X-WinOne-Session"))
	} else {
		headers.Set("X-WinOne-Caller-Key", ai.GatewayCallerKeyID(r.Context()))
	}
	partial := false
	err = store.WithKeys(ctx, keys, func(ctx context.Context, key string) (bool, error) {
		var failure error
		if inProduct {
			partial, failure = runtime.ChatRequest(ctx, prefs, key, path, body, headers, w)
		} else {
			partial, failure = runtime.GatewayRequest(ctx, prefs, key, path, body, headers, w)
		}
		return partial, failure
	})
	if err != nil {
		if partial {
			panic(http.ErrAbortHandler)
		} // Terminate a truncated stream, never send a false completion.
		status := 502
		var rejected *ai.AttemptFailure
		if errors.As(err, &rejected) {
			status = rejected.Status
			if rejected.RetryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int((rejected.RetryAfter+time.Second-1)/time.Second)))
			}
		}
		ai.GatewayError(w, status, "AI 服务调用失败，请检查供应商状态后重试")
	}
}

// Global Magpie routes select their own helper provider. Limit that catalog to
// models the authenticated gateway caller can use, just like a normal request.
func (s *aiService) serveMagpieProtocol(w http.ResponseWriter, r *http.Request, store *ai.Store, path string, body []byte, allowModel func(ai.Selection) bool) {
	prefs, err := store.MagpieRuntime()
	if err != nil {
		ai.GatewayError(w, 503, "AI 设置无法读取")
		return
	}
	providers, err := store.MagpieProviders()
	if err != nil {
		ai.GatewayError(w, 503, "辅助模型配置无法读取")
		return
	}
	for i := range providers {
		enabled := []string{}
		for _, model := range providers[i].Provider.Models {
			if allowModel(ai.Selection{Provider: providers[i].Provider.ID, Model: model}) {
				enabled = append(enabled, model)
			}
		}
		providers[i].Provider.Models = enabled
	}
	select {
	case s.gatewaySlots <- struct{}{}:
		defer func() { <-s.gatewaySlots }()
	default:
		ai.GatewayError(w, 429, "本地网关请求较多，请稍后重试")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), prefs.GatewayWait(path))
	defer cancel()
	headers := make(http.Header)
	for _, name := range []string{"MCP-Protocol-Version", "Accept", "User-Agent", "Originator"} {
		if value := r.Header.Get(name); value != "" {
			headers.Set(name, value)
		}
	}
	partial, err := s.translator.MagpieGateway(ctx, prefs, providers, path, body, headers, w)
	if err != nil {
		if partial {
			panic(http.ErrAbortHandler)
		}
		ai.GatewayError(w, 502, "Magpie 辅助服务调用失败，请检查模型或搜索 API 设置")
	}
}

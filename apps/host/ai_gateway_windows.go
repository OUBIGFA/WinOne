//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/host"
)

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
	// The upstream handler owns request parsing, size limits and scheduling.
	// The authenticated model scope below also covers native/media helpers.
	process, err := s.startGatewayBackend()
	if err != nil {
		ai.GatewayError(w, 503, "本地网关服务无法启动")
		return
	}
	process.ServeMedia(w, r, models)
}

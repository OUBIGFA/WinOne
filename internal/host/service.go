// Package host is the UI-independent application command boundary.
package host

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"winone/internal/settings"
)

type Request struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}
type RPCError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}
type Module struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}
type Snapshot struct {
	Settings settings.Settings `json:"settings"`
	Modules  []Module          `json:"modules"`
}
type moduleBinding struct {
	status func() Module
	handle func(Request) Response
}
type Service struct {
	settings *settings.Store
	mu       sync.RWMutex
	modules  map[string]moduleBinding
}

func New(store *settings.Store) *Service {
	return &Service{settings: store, modules: map[string]moduleBinding{}}
}

// RegisterModule binds one namespace; feature implementations remain outside the Host.
func (s *Service) RegisterModule(id string, status func() Module, handle func(Request) Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modules[id] = moduleBinding{status, handle}
}

// Handle exposes only explicit application commands; no arbitrary execution endpoint exists.
func (s *Service) Handle(r Request) Response {
	response := Response{ID: r.ID}
	fail := func(code, message string) Response { response.Error = &RPCError{code, message}; return response }
	if r.Version != 1 {
		return fail("protocol", "不支持的通信版本")
	}
	if r.ID == "" {
		return fail("invalid", "缺少请求标识")
	}
	if module, method, ok := strings.Cut(r.Method, "."); ok && module != "settings" {
		s.mu.RLock()
		binding, exists := s.modules[module]
		s.mu.RUnlock()
		if exists {
			r.Method = method
			return binding.handle(r)
		}
	}
	var result any
	switch r.Method {
	case "snapshot":
		modules := []Module{
			{"capture", "截图", "unavailable", "截图组件尚未接入"},
			{"always-on-top", "窗口置顶", "unavailable", "置顶组件尚未接入"},
			{"wintray", "启动与托盘", "unavailable", "WinTray 组件尚未接入"},
		}
		s.mu.RLock()
		bindings := make(map[string]moduleBinding, len(s.modules))
		for id, b := range s.modules {
			bindings[id] = b
		}
		s.mu.RUnlock()
		for i, m := range modules {
			if binding, ok := bindings[m.ID]; ok {
				modules[i] = binding.status()
			}
		}
		result = Snapshot{Settings: s.settings.Snapshot(), Modules: modules}
	case "settings.save":
		var next settings.Settings
		if err := json.Unmarshal(r.Params, &next); err != nil {
			return fail("invalid", "设置格式无效")
		}
		saved, err := s.settings.Save(next)
		if errors.Is(err, settings.ErrConflict) {
			return fail("conflict", "设置已变更，请刷新后重试")
		}
		if err != nil {
			return fail("save_failed", err.Error())
		}
		result = saved
	default:
		return fail("unknown_method", "不支持的操作")
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fail("internal", "无法生成操作结果")
	}
	response.Result = data
	return response
}

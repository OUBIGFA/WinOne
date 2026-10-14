// Package host is the UI-independent application command boundary.
package host

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"winone/internal/catalog"
	"winone/internal/settings"
)

type Request struct {
	// PeerPID is supplied by the local transport, never by serialized input.
	PeerPID uint32          `json:"-"`
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
	Settings       settings.Settings `json:"settings"`
	Modules        []Module          `json:"modules"`
	Locale         string            `json:"locale,omitempty"`
	Administrator  bool              `json:"administrator"`
	RuntimeWarning string            `json:"runtimeWarning,omitempty"`
	RestartPending bool              `json:"restartPending,omitempty"`
}
type moduleBinding struct {
	status       func() Module
	handle       func(Request) Response
	registration *FeatureRegistration
}

// SettingsStore permits the Host to migrate persistence without changing UI RPCs.
type SettingsStore interface {
	Snapshot() settings.Settings
	Save(settings.Settings) (settings.Settings, error)
}
type Service struct {
	settings        SettingsStore
	mu              sync.RWMutex
	modules         map[string]moduleBinding
	settingsMu      sync.Mutex
	settingsChanged func(settings.Settings)
	features        []catalog.Manifest
	locale          func(settings.Settings) string
	administrator   bool
	runtimeWarning  func() string
	restartPending  func() bool
}

func (s *Service) SetRestartPendingResolver(resolve func() bool) { s.restartPending = resolve }

// Configured before serving requests; runtime failures are not saved preferences.
func (s *Service) SetRuntimeWarningResolver(resolve func() string) { s.runtimeWarning = resolve }

func (s *Service) SetAdministrator(value bool) { s.mu.Lock(); s.administrator = value; s.mu.Unlock() }

// SetLocaleResolver is configured before serving requests; the shell does not
// independently guess a locale that could disagree with native module menus.
func (s *Service) SetLocaleResolver(resolve func(settings.Settings) string) { s.locale = resolve }

// OnSettingsChanged installs the Host's runtime preference observer before serving clients.
func (s *Service) OnSettingsChanged(callback func(settings.Settings)) { s.settingsChanged = callback }

func New(store SettingsStore) *Service {
	return &Service{settings: store, modules: map[string]moduleBinding{}, features: catalog.Builtins()}
}

// FeatureCatalog is the shared registered-feature source for navigation and
// lifecycle controls. Callers receive copies and cannot mutate registration.
func (s *Service) FeatureCatalog() []catalog.Manifest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	manifests := make([]catalog.Manifest, len(s.features))
	for i, manifest := range s.features {
		manifests[i] = manifest.Clone()
	}
	return manifests
}

// RegisterFeature is an internal registration boundary, not an RPC installation API.
// Callers must verify package trust and dependencies before supplying external manifests.
func (s *Service) RegisterFeature(manifest catalog.Manifest, status func() Module, handle func(Request) Response) error {
	_, err := s.BindFeature(manifest, status, handle)
	return err
}

// BindFeature returns ownership of this exact registration for later removal.
// Lifecycle callers must stop and retire their handlers before releasing it.
func (s *Service) BindFeature(manifest catalog.Manifest, status func() Module, handle func(Request) Response) (*FeatureRegistration, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if status == nil || handle == nil {
		return nil, errors.New("module callbacks are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.features {
		if existing.ID == manifest.ID || strings.HasPrefix(existing.ID, manifest.ID+".") || strings.HasPrefix(manifest.ID, existing.ID+".") {
			return nil, errors.New("module namespace overlaps an existing module")
		}
	}
	// Longest-prefix routing also makes parent/child service names overlap.
	// Validate every installed service before changing either registration map.
	for namespace := range s.modules {
		if namespace == manifest.ID || strings.HasPrefix(namespace, manifest.ID+".") || strings.HasPrefix(manifest.ID, namespace+".") {
			return nil, errors.New("module namespace overlaps an existing service")
		}
	}
	registration := &FeatureRegistration{service: s, id: manifest.ID}
	s.features = append(s.features, manifest.Clone())
	s.modules[manifest.ID] = moduleBinding{status: status, handle: handle, registration: registration}
	return registration, nil
}

// RegisterModule binds one namespace; feature implementations remain outside the Host.
func (s *Service) RegisterModule(id string, status func() Module, handle func(Request) Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modules[id] = moduleBinding{status: status, handle: handle}
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
	// Qualified extension IDs contain dots; select the longest registered namespace.
	s.mu.RLock()
	var routed moduleBinding
	matched := ""
	for namespace, binding := range s.modules {
		if strings.HasPrefix(r.Method, namespace+".") && len(namespace) > len(matched) {
			matched = namespace
			routed = binding
		}
	}
	s.mu.RUnlock()
	if matched != "" {
		r.Method = strings.TrimPrefix(r.Method, matched+".")
		return routed.handle(r)
	}
	var result any
	switch r.Method {
	case "modules.catalog":
		result = s.FeatureCatalog()
	case "snapshot":
		s.mu.RLock()
		modules := make([]Module, 0, len(s.features))
		for _, m := range s.features {
			modules = append(modules, Module{ID: m.ID, Name: m.Name.Chinese, State: "unavailable", Detail: "组件未就绪"})
		}
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
		locale := ""
		global := s.settings.Snapshot()
		if s.locale != nil {
			locale = s.locale(global)
		}
		s.mu.RLock()
		administrator := s.administrator
		s.mu.RUnlock()
		warning := ""
		if s.runtimeWarning != nil {
			warning = s.runtimeWarning()
		}
		pending := false
		if s.restartPending != nil {
			pending = s.restartPending()
		}
		result = Snapshot{Settings: global.ResolvedShortcuts(), Modules: modules, Locale: locale, Administrator: administrator, RuntimeWarning: warning, RestartPending: pending}
	case "settings.save":
		s.settingsMu.Lock()
		defer s.settingsMu.Unlock()
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
		if s.settingsChanged != nil {
			s.settingsChanged(saved)
		}
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

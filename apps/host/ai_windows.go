//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/diagnostics"
	"winone/internal/host"
)

type aiService struct {
	backend           gatewayBackend
	modelRefresh      *aiModelRefresh
	avatarMu          sync.Mutex
	avatarUploads     map[string]*avatarUpload
	gateway           *ai.Gateway
	gatewayGuard      func() (func(), bool)
	mu                sync.Mutex
	store             *ai.Store
	storePath         string
	desktopExecutable string
	translator        ai.Translator
	localOCR          *ai.LocalOCR
	captureEnabled    func() bool // Published before serving: controls local OCR residency.
	jobs              map[string]*aiJob
	ctx               context.Context
	accountGate       chan struct{}
	accountGates      map[string]chan struct{}
	signin            *aiSignInJob
	resultSessions    interface {
		authorize(string, uint32) error
		image(string, uint32) ([]byte, error)
	}
}

func registerAI(service *host.Service, ctx context.Context, directory, executable string) error {
	bindAIService(service, newAIService(ctx, directory, executable))
	return nil
}
func newAIService(ctx context.Context, directory, executable string) *aiService {
	runtime := ai.Translator{Executable: filepath.Join(filepath.Dir(executable), "shared", "ai", "WinOne.AI.exe"), DataDirectory: filepath.Join(directory, "ai-runtime")}
	engine := &aiService{storePath: filepath.Join(directory, "ai-settings.json"), desktopExecutable: filepath.Join(filepath.Dir(executable), "WinOne.exe"), translator: runtime, localOCR: ai.NewLocalOCR(ctx, runtime), jobs: map[string]*aiJob{}, ctx: ctx, accountGate: make(chan struct{}, 1)}
	engine.translator.PluginLifetime = ctx
	// Timing lines go to the product log off the OCR gate; a log failure
	// never affects recognition.
	engine.localOCR.Observe = func(event, message string) {
		go func() {
			_ = diagnostics.Record(directory, diagnostics.Event{Source: "host", Event: event, Message: message})
		}()
	}
	return engine
}
func bindAIService(service *host.Service, engine *aiService) {
	engine.initGateway()
	status := func() host.Module { return host.Module{ID: "ai", State: "ready"} }
	service.RegisterModule("ai", status, engine.handle)
	// The existing settings bridge can edit shared AI preferences without a
	// duplicate provider store. Future modules use the same ai namespace.
	service.RegisterModule("capture.ai", status, engine.handle)
}
func (s *aiService) handle(r host.Request) host.Response {
	if r.Method == "models.import" {
		var input struct {
			Text string `json:"text"`
		}
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai", Message: err.Error()}}
		}
		output, err := s.translator.ImportProviderSetup(s.ctx, input.Text)
		if err != nil {
			return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai", Message: err.Error()}}
		}
		return host.Response{ID: r.ID, Result: output}
	}
	if r.Method == "models.test" {
		return s.handleModelTest(r)
	}
	if r.Method == "proxies.test" {
		return s.handleProxyProbe(r)
	}
	if strings.HasPrefix(r.Method, "icons.") {
		return s.handleAvatar(r)
	}
	if strings.HasPrefix(r.Method, "gateway.") {
		return s.handleGateway(r)
	}
	if strings.HasPrefix(r.Method, "accounts.") {
		return s.handleAccounts(r)
	}
	if strings.HasPrefix(r.Method, "plans.") {
		return s.handlePlans(r)
	}
	response := host.Response{ID: r.ID}
	fail := func(err error) host.Response {
		response.Error = &host.RPCError{Code: "ai", Message: err.Error()}
		return response
	}
	result := func(value any) host.Response { response.Result, _ = json.Marshal(value); return response }
	// Settings-backed operations share one initialization boundary. Keep
	// certificate inspection here too: it can be the first request after launch.
	switch r.Method {
	case "translate.target", "settings.get", "settings.save", "configuration.get", "configuration.save", "keys.reveal", "models.fetch", "models.certificate", "translate.start", "generate.start", "recognize.start":
		s.mu.Lock()
		var err error
		if s.store == nil {
			s.store, err = ai.Open(s.storePath)
		}
		s.mu.Unlock()
		if err != nil {
			return fail(errors.New("AI 设置无法读取，请恢复配置文件后重试：" + err.Error()))
		}
	}
	switch r.Method {
	case "ocr.status":
		if s.localOCR == nil {
			return result(map[string]any{"ready": false, "processId": 0})
		}
		ready, pid := s.localOCR.Status()
		return result(map[string]any{"ready": ready, "processId": pid})
	case "ocr.prepare":
		queued, err := s.prepareLocalOCRAfterResume()
		if err != nil {
			return fail(err)
		}
		return result(map[string]bool{"queued": queued})
	case "models.fetch", "models.certificate":
		var params struct {
			Provider   string                   `json:"provider"`
			Draft      *ai.AccountDraft         `json:"draft,omitempty"`
			Connection *ai.ModelConnectionDraft `json:"connection,omitempty"`
			KeyUpdates map[string]*string       `json:"keyUpdates,omitempty"`
		}
		if err := configjson.DecodeObject(r.Params, &params); err != nil {
			return fail(err)
		}
		if params.Connection != nil && params.Draft != nil || params.Connection == nil && len(params.KeyUpdates) != 0 {
			return fail(errors.New("模型连接参数无效"))
		}
		var provider ai.Provider
		var keyPlan ai.KeyPlan
		var err error
		if params.Connection != nil {
			provider, err = params.Connection.Provider(params.Provider)
		} else if params.Draft != nil {
			provider, err = s.accountProvider(params.Provider, params.Draft.Agent, params.Draft)
		} else {
			provider, keyPlan, err = s.store.ProviderKeysForModels(params.Provider)
		}
		if err != nil {
			return fail(err)
		}
		gate := s.accountGateFor(provider.ID)
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		default:
			return fail(errors.New("供应商正在登录或使用，请稍后再试"))
		}
		ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
		defer cancel()
		var models []ai.ModelOption
		// Discovery uses the provider's saved connection preferences, just like
		// generation and account requests.
		runtime, err := s.magpieTranslator()
		if err != nil {
			return fail(err)
		}
		if r.Method == "models.certificate" {
			if provider.AuthMode == "account" {
				return fail(errors.New("账号供应商不使用自定义服务器证书"))
			}
			certificate, err := runtime.ServerCertificate(ctx, provider)
			if err != nil {
				return fail(err)
			}
			return host.Response{ID: r.ID, Result: certificate}
		}
		if params.Connection != nil {
			err = s.store.WithModelDraft(ctx, params.Provider, *params.Connection, params.KeyUpdates, func(ctx context.Context, p ai.Provider, key string) error {
				var err error
				models, err = runtime.Models(ctx, p, key)
				return err
			})
		} else if params.Draft != nil {
			models, err = runtime.Models(ctx, provider, "")
		} else {
			err = s.store.WithKeys(ctx, keyPlan, func(ctx context.Context, key string) (bool, error) {
				var err error
				models, err = runtime.Models(ctx, provider, key)
				return false, err
			})
		}
		if err != nil {
			return fail(err)
		}
		return result(map[string]any{"models": models})
	case "keys.reveal":
		var input struct {
			Provider string `json:"provider"`
			ID       string `json:"id"`
			All      bool   `json:"all,omitempty"`
		}
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return fail(err)
		}
		if input.All {
			if input.ID != "" {
				return fail(errors.New("不能同时选择单个和全部密钥"))
			}
			keys, err := s.store.RevealCredentials(input.Provider)
			if err != nil {
				return fail(err)
			}
			return result(map[string]any{"keys": keys})
		}
		key, err := s.store.RevealCredential(input.Provider, input.ID)
		if err != nil {
			return fail(err)
		}
		return result(map[string]string{"key": key})
	case "configuration.get":
		return result(s.store.Configuration())
	case "configuration.save":
		var input struct {
			Configuration ai.Configuration     `json:"configuration"`
			Base          *ai.Configuration    `json:"base,omitempty"`
			Keys          map[string]*string   `json:"keys,omitempty"`
			KeyUpdates    ai.CredentialUpdates `json:"keyUpdates,omitempty"`
		}
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return fail(err)
		}
		saved, err := s.store.SaveConfigurationEdit(input.Configuration, input.Base, input.Keys, input.KeyUpdates)
		if err != nil {
			return fail(err)
		}
		s.updateModelRefresh()
		s.refreshLocalOCRStandby()
		s.retireGatewayBackend()
		return result(saved)
	case "translate.target":
		var input struct {
			Text string `json:"text"`
		}
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return fail(err)
		}
		return result(map[string]string{"target": ai.TargetLanguage(s.store.Snapshot().Target, input.Text)})
	case "settings.get":
		return result(s.store.Snapshot())
	case "settings.save":
		var p struct {
			Preferences ai.Preferences `json:"preferences"`
			Key         *string        `json:"key"`
		}
		if err := configjson.DecodeObject(r.Params, &p); err != nil {
			return fail(err)
		}
		saved, err := s.store.Save(p.Preferences, p.Key)
		if err != nil {
			return fail(err)
		}
		s.updateModelRefresh()
		s.refreshLocalOCRStandby()
		return result(saved)
	case "translate.start", "generate.start", "recognize.start":
		value, err := s.startJob(r)
		if err != nil {
			return fail(err)
		}
		return result(value)
	case "jobs.get", "jobs.cancel":
		value, err := s.readOrCancelJob(r)
		if err != nil {
			return fail(err)
		}
		response.Result = value
		return response
	default:
		return fail(errors.New("不支持的 AI 操作"))
	}
}

func (s *aiService) accountGateFor(provider string) chan struct{} {
	if provider == "" || provider == "legacy" {
		return s.accountGate
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountGates == nil {
		s.accountGates = map[string]chan struct{}{}
	}
	if s.accountGates[provider] == nil {
		s.accountGates[provider] = make(chan struct{}, 1)
	}
	return s.accountGates[provider]
}

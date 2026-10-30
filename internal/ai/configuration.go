package ai

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// MaxProviderModels accommodates upstream catalogs above 2500 entries while
// retaining a bounded configuration and worker response.
const MaxProviderModels = 4096

// Provider identifies a configured service or an isolated signed-in account.
// API keys are write-only. User-defined request headers remain editable here.
type Provider struct {
	LimitRestOff bool `json:"limitRestOff,omitempty"`
	Searches     bool `json:"searches,omitempty"`
	RequestLimits
	ModelMetadata   map[string]ModelMetadata `json:"modelMetadata,omitempty"`
	AdditionalTools bool                     `json:"additionalTools,omitempty"`
	Headers         map[string]string        `json:"headers,omitempty"` // User request headers; never diagnostic or prompt data.
	Keys            []KeySlot                `json:"keys,omitempty"`
	KeyPolicy       *KeyPolicy               `json:"keyPolicy,omitempty"`
	AvailableModels []string                 `json:"availableModels,omitempty"`
	Delisted        []string                 `json:"delisted,omitempty"` // Paused picks; never served until listed again.
	ModelNames      map[string]string        `json:"modelNames,omitempty"`
	ModelFree       map[string]bool          `json:"modelFree,omitempty"` // Vendor metadata, independent of the routing ID.
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	AuthMode        string                   `json:"authMode"`
	Protocol        string                   `json:"protocol,omitempty"`
	BaseURL         string                   `json:"baseUrl,omitempty"`
	Agent           string                   `json:"agent,omitempty"`
	AgentBackend    string                   `json:"agentBackend,omitempty"` // Empty preserves an existing built-in account.
	Preset          string                   `json:"preset,omitempty"`
	PresetRegion    string                   `json:"presetRegion,omitempty"`
	Models          []string                 `json:"models"`
	HasKey          bool                     `json:"hasKey"`
	Icon            string                   `json:"icon,omitempty"`   // Derived from endpoint/account, not a user-supplied URL.
	Avatar          *string                  `json:"avatar,omitempty"` // Explicit picture/built-in choice; nil preserves older clients.
}

// KeySlot exposes identity and availability, never the secret or a secret suffix.
type KeySlot struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	HasKey  bool   `json:"hasKey"`
}

type KeyPolicy struct {
	Strategy        string `json:"strategy"`
	Failover        bool   `json:"failover"`
	MaxAttempts     int    `json:"maxAttempts"`
	CooldownSeconds int    `json:"cooldownSeconds"`
}

func keyPolicy(provider Provider) KeyPolicy {
	if provider.KeyPolicy != nil {
		return *provider.KeyPolicy
	}
	return KeyPolicy{Strategy: "round-robin", Failover: true, MaxAttempts: 3, CooldownSeconds: 60}
}

type Selection struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}
type Configuration struct {
	RoutingEnabled  bool                 `json:"routingEnabled,omitempty"`
	MagpieSettings  *MagpieSettings      `json:"magpieSettings,omitempty"`
	Revision        uint64               `json:"revision"`
	Providers       []Provider           `json:"providers"`
	Tasks           map[string]Selection `json:"tasks"`
	Engine          string               `json:"engine"`
	OCREngine       string               `json:"ocrEngine"`
	Target          string               `json:"target"`
	Prompt          string               `json:"prompt"`
	AutoRetranslate *bool                `json:"autoRetranslate,omitempty"`
	ShowReasoning   *bool                `json:"showReasoning,omitempty"`
}

// Missing in older configurations means enabled, without changing inference.
func (c Configuration) ReasoningVisible() bool { return c.ShowReasoning == nil || *c.ShowReasoning }

// Retranslation is enabled for older configurations that have no preference.
func (c Configuration) RetranslateOnModelChange() bool {
	return c.AutoRetranslate == nil || *c.AutoRetranslate
}

var providerIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func cloneConfiguration(c Configuration) Configuration {
	c.MagpieSettings = c.MagpieSettings.clone()
	automatic := c.RetranslateOnModelChange()
	c.AutoRetranslate = &automatic
	visible := c.ReasoningVisible()
	c.ShowReasoning = &visible
	c.Providers = slices.Clone(c.Providers)
	if c.Providers == nil {
		c.Providers = []Provider{}
	}
	for i := range c.Providers {
		c.Providers[i].Keys = slices.Clone(c.Providers[i].Keys)
		if c.Providers[i].AuthMode == "api-key" {
			policy := keyPolicy(c.Providers[i])
			c.Providers[i].KeyPolicy = &policy
		}
		c.Providers[i].Icon = providerIcon(c.Providers[i])
		if c.Providers[i].Avatar != nil {
			avatar := *c.Providers[i].Avatar
			c.Providers[i].Avatar = &avatar
		}
		c.Providers[i].Models = slices.Clone(c.Providers[i].Models)
		c.Providers[i].Delisted = slices.Clone(c.Providers[i].Delisted)
		c.Providers[i].ModelNames = maps.Clone(c.Providers[i].ModelNames)
		c.Providers[i].Headers = maps.Clone(c.Providers[i].Headers)
		c.Providers[i].ModelFree = maps.Clone(c.Providers[i].ModelFree)
		c.Providers[i].ModelMetadata = maps.Clone(c.Providers[i].ModelMetadata)
		for id, metadata := range c.Providers[i].ModelMetadata {
			c.Providers[i].ModelMetadata[id] = metadata.clone()
		}
		c.Providers[i].AvailableModels = c.Providers[i].AllModels()
		if c.Providers[i].Models == nil {
			c.Providers[i].Models = []string{}
		}
	}
	c.Tasks = maps.Clone(c.Tasks)
	if c.Tasks == nil {
		c.Tasks = map[string]Selection{}
	}
	return c
}

// AllModels includes discovered, manually entered and enabled legacy models.
// Models remains the enabled subset used by every invocation/model picker.
func (p Provider) AllModels() []string {
	all := slices.Clone(p.AvailableModels)
	if all == nil {
		all = []string{}
	}
	for _, model := range p.Models {
		if !slices.Contains(all, model) {
			all = append(all, model)
		}
	}
	return all
}

// Display names never replace protocol IDs. Some account services deliberately
// use opaque routing IDs such as qmodel_38max alongside a readable model name.
func (p Provider) ModelName(id string) string {
	if name := p.ModelNames[id]; strings.TrimSpace(name) != "" {
		return name
	}
	return id
}

// Match Magpie's explicit Free flag and its namedFree word-boundary rule.
// Missing prices and a zero default rate do not establish that a model is free.
var namedFreeModel = regexp.MustCompile(`(?i)(^|[^a-z])free($|[^a-z])|[（(]免费[）)]`)

func (p Provider) IsFreeModel(id string) bool {
	return p.ModelFree[id] || namedFreeModel.MatchString(id) || namedFreeModel.MatchString(p.ModelName(id))
}

func (p Provider) ModelIcon(id string) string {
	icon := ModelIcon(id)
	if icon == "model" {
		return ModelIcon(p.ModelName(id))
	}
	return icon
}
func legacyConfiguration(p Preferences) Configuration {
	c := Configuration{Revision: p.Revision, Providers: []Provider{}, Tasks: map[string]Selection{}, Engine: p.Engine, OCREngine: p.OCREngine, Target: p.Target, Prompt: p.Prompt}
	if c.OCREngine == "" {
		c.OCREngine = "local"
	}
	if p.Model != "" || p.BaseURL != "" || p.Agent != "" || p.HasKey {
		mode := p.AuthMode
		if mode == "" {
			mode = "api-key"
		}
		models := []string{}
		if p.Model != "" {
			models = append(models, p.Model)
		}
		c.Providers = append(c.Providers, Provider{ID: "legacy", Name: "现有服务", AuthMode: mode, Protocol: p.Protocol, BaseURL: p.BaseURL, Agent: p.Agent, AgentBackend: p.AgentBackend, Models: models, HasKey: p.HasKey})
		if p.Model != "" {
			c.Tasks["default"] = Selection{Provider: "legacy", Model: p.Model}
		}
	}
	return cloneConfiguration(c)
}
func (c Configuration) Provider(id string) (Provider, bool) {
	for _, p := range c.Providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}
func (c Configuration) Selected(task string) Selection {
	selection, exists := c.Tasks[task]
	// An explicitly cleared binding must not route to a different provider.
	if !exists {
		selection = c.Tasks["default"]
	}
	return selection
}
func (c Configuration) ValidateSelection(selection Selection) error {
	p, ok := c.Provider(selection.Provider)
	if !ok {
		return errors.New("请选择已配置的供应商或账号")
	}
	if !slices.Contains(p.Models, selection.Model) || selection.Model == "" {
		return errors.New("请选择该供应商中已勾选启用的模型")
	}
	return nil
}
func validateConfiguration(c Configuration) error {
	if err := c.MagpieSettings.validate(); err != nil {
		return err
	}
	if c.Engine != "google" && c.Engine != "llm" || c.OCREngine != "local" && c.OCREngine != "ai" || !ValidTranslationTarget(c.Target) || len(c.Prompt) > 16000 {
		return errors.New("默认任务设置无效")
	}
	if len(c.Providers) > 32 {
		return errors.New("最多配置 32 个服务")
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	for _, p := range c.Providers {
		if err := p.RequestLimits.validate(); err != nil {
			return err
		}
		if err := validateProviderHeaders(p.Headers); err != nil {
			return err
		}
		if p.Avatar != nil && !validProviderAvatar(*p.Avatar) {
			return errors.New("供应商头像无效，请重新选择图片或内置图标")
		}
		if p.AuthMode == "account" && len(p.Headers) > 0 {
			return errors.New("账号供应商使用自身的授权请求头")
		}
		if p.Preset != "" && (p.AuthMode != "api-key" || !providerIDPattern.MatchString(p.Preset)) || p.PresetRegion != "" && (p.Preset == "" || !providerIDPattern.MatchString(p.PresetRegion)) {
			return errors.New("服务预设或地区无效")
		}
		if len(p.Keys) > 16 {
			return errors.New("每个供应商最多配置 16 个密钥")
		}
		keyIDs := map[string]bool{}
		for _, slot := range p.Keys {
			if !providerIDPattern.MatchString(slot.ID) || keyIDs[slot.ID] {
				return errors.New("密钥标识无效或重复")
			}
			keyIDs[slot.ID] = true
		}
		if p.AuthMode == "account" && (len(p.Keys) > 0 || p.KeyPolicy != nil) {
			return errors.New("账号供应商不使用 API 密钥策略")
		}
		policy := keyPolicy(p)
		if (policy.Strategy != "round-robin" && policy.Strategy != "ordered") || policy.MaxAttempts < 1 || policy.MaxAttempts > 16 || policy.CooldownSeconds < 1 || policy.CooldownSeconds > 3600 {
			return errors.New("密钥策略、尝试次数或冷却时间无效")
		}
		if !providerIDPattern.MatchString(p.ID) || ids[p.ID] {
			return errors.New("供应商标识无效或重复，请重新添加此服务")
		}
		if strings.TrimSpace(p.Name) == "" || len(p.Name) > 240 {
			return errors.New("有供应商未填写有效名称，请选择未命名服务，填写名称或移除空白项")
		}
		ids[p.ID] = true
		name := strings.ToLower(strings.TrimSpace(p.Name))
		if names[name] {
			return errors.New("请为每个服务使用不同名称")
		}
		names[name] = true
		if len(p.Models) > MaxProviderModels {
			return errors.New("每个服务最多配置 2048 个模型")
		}
		seen := map[string]bool{}
		for _, model := range p.Models {
			if model == "" || strings.TrimSpace(model) != model || len(model) > 256 || seen[model] {
				return fmt.Errorf("%s 的模型名称为空、重复或过长", p.Name)
			}
			seen[model] = true
		}
		if len(p.Delisted) > MaxProviderModels {
			return errors.New("暂时不可用的模型选择过多")
		}
		paused := map[string]bool{}
		for _, model := range p.Delisted {
			if model == "" || strings.TrimSpace(model) != model || len(model) > 256 || paused[model] || seen[model] {
				return errors.New("暂时不可用的模型选择无效")
			}
			paused[model] = true
		}
		if len(p.AvailableModels) > MaxProviderModels {
			return errors.New("供应商模型列表过大")
		}
		known := map[string]bool{}
		for _, model := range p.Models {
			known[model] = true
		}
		availableSeen := map[string]bool{}
		for _, model := range p.AvailableModels {
			if model == "" || strings.TrimSpace(model) != model || len(model) > 256 || availableSeen[model] {
				return errors.New("供应商模型列表无效")
			}
			known[model] = true
			availableSeen[model] = true
		}
		if len(p.ModelNames) > MaxProviderModels {
			return errors.New("供应商模型名称列表过大")
		}
		for id, name := range p.ModelNames {
			if !known[id] || strings.TrimSpace(name) == "" || len(name) > 256 {
				return errors.New("供应商模型显示名称无效")
			}
		}
		if len(p.ModelFree) > MaxProviderModels {
			return errors.New("供应商模型费用标识过多")
		}
		for id := range p.ModelFree {
			if !known[id] {
				return errors.New("供应商费用标识引用了未知模型")
			}
		}
		if len(p.ModelMetadata) > MaxProviderModels {
			return errors.New("供应商模型设置过多")
		}
		for id, metadata := range p.ModelMetadata {
			if !known[id] {
				return errors.New("模型设置引用了未知模型")
			}
			if err := validateModelMetadata(metadata); err != nil {
				return fmt.Errorf("%s：%w", p.ModelName(id), err)
			}
		}
		// An account can be saved before sign-in/model discovery. Its eventual
		// task binding still requires a real model in ValidateSelection.
		probe := Preferences{Engine: "llm", OCREngine: "local", Target: c.Target, Protocol: p.Protocol, AuthMode: p.AuthMode, BaseURL: p.BaseURL, Agent: p.Agent, AgentBackend: p.AgentBackend, Model: "validation"}
		if p.AuthMode == "account" {
			probe.Protocol = "chat"
		} else if p.AuthMode != "api-key" {
			return errors.New("请选择有效的服务类型")
		}
		if err := validate(probe); err != nil {
			return fmt.Errorf("%s：%w", p.Name, err)
		}
	}
	for task, selection := range c.Tasks {
		if task != "default" && task != "ocr" && task != "translate" && task != "chat" {
			return errors.New("未知的 AI 任务")
		}
		if selection.Provider == "" && selection.Model == "" {
			continue
		}
		// A model may be disabled while an existing feature still refers to it.
		// Save the provider preference; invocation requires an enabled model.
		provider, exists := c.Provider(selection.Provider)
		if !exists || !slices.Contains(provider.AllModels(), selection.Model) && !slices.Contains(provider.Delisted, selection.Model) {
			return fmt.Errorf("%s：请选择存在的供应商和模型", task)
		}
	}
	return nil
}

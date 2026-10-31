package aiservices

import (
	"encoding/json"

	"winone/desktopmygo/core"
	"winone/desktopmygo/widgets"
)

// The configuration is the Host's capture.ai.configuration document. The
// page declares the fields it shows or edits; core.Preserved keeps the
// others (the capture engines, prompts and anything newer) as read, as
// React kept them by spreading the object it read.

// keySlot is one API key of a provider. Keys never leave the Host:
// hasKey tells whether one is saved, and new keys travel separately as
// keyUpdates when the configuration is saved.
type keySlot struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	HasKey  bool   `json:"hasKey"`
}

// keyPolicy decides how several keys share requests.
type keyPolicy struct {
	Strategy        string  `json:"strategy"`
	Failover        bool    `json:"failover"`
	MaxAttempts     float64 `json:"maxAttempts"`
	CooldownSeconds float64 `json:"cooldownSeconds"`
}

var defaultPolicy = keyPolicy{Strategy: "round-robin", Failover: true, MaxAttempts: 3, CooldownSeconds: 60}

// priceParts are a model price's fields in dollars per million tokens, as
// Magpie's Names & levels keeps them.
type priceParts struct {
	Input        *float64 `json:"input,omitempty"`
	Output       *float64 `json:"output,omitempty"`
	CacheRead    *float64 `json:"cache_read,omitempty"`
	CacheWrite   *float64 `json:"cache_write,omitempty"`
	CacheWrite1h *float64 `json:"cache_write_1h,omitempty"`
}

// priceTier is the price above a context size.
type priceTier struct {
	Above float64 `json:"above"`
	priceParts
}

type modelPrice struct {
	priceParts
	Tiers []core.Preserved[priceTier] `json:"tiers,omitempty"`
}

// modelMetadata is what a model's settings change: optional overrides of
// the vendor's report, absent while the user keeps the vendor's value.
// Pointers keep "absent" apart from zero and empty lists, as the React
// document kept undefined apart from 0 and [].
type modelMetadata struct {
	Searches            *bool                        `json:"searches,omitempty"`
	ContextWindow       *float64                     `json:"contextWindow,omitempty"`
	MaxTokens           *float64                     `json:"maxTokens,omitempty"`
	CompactionThreshold *float64                     `json:"compactionThreshold,omitempty"`
	CompactionPercent   *float64                     `json:"compactionPercent,omitempty"`
	ListedContextWindow *float64                     `json:"listedContextWindow,omitempty"`
	MaxContextWindow    *float64                     `json:"maxContextWindow,omitempty"`
	Rate                *float64                     `json:"rate,omitempty"`
	RateWas             *float64                     `json:"rateWas,omitempty"`
	Reasoning           *bool                        `json:"reasoning,omitempty"`
	Vision              *bool                        `json:"vision,omitempty"`
	Tools               *bool                        `json:"tools,omitempty"`
	ReasoningEffort     *string                      `json:"reasoningEffort,omitempty"`
	Tier                *string                      `json:"tier,omitempty"`
	Same                *string                      `json:"same,omitempty"`
	UpstreamName        *string                      `json:"upstreamName,omitempty"`
	API                 *string                      `json:"api,omitempty"`
	Levels              *[]string                    `json:"levels,omitempty"`
	Efforts             *[]string                    `json:"efforts,omitempty"`
	Tiers               *[]string                    `json:"tiers,omitempty"`
	Price               *core.Preserved[modelPrice] `json:"price,omitempty"`
}

type metadata = core.Preserved[modelMetadata]

// provider is one AI service: a custom API connection ("api-key") or an
// agent account ("account").
type provider struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	AuthMode        string              `json:"authMode"`
	Protocol        string              `json:"protocol,omitempty"`
	BaseURL         string              `json:"baseUrl,omitempty"`
	Preset          string              `json:"preset,omitempty"`
	PresetRegion    string              `json:"presetRegion,omitempty"`
	Agent           string              `json:"agent,omitempty"`
	AgentBackend    string              `json:"agentBackend,omitempty"`
	Models          []string            `json:"models"`
	Delisted        *[]string           `json:"delisted,omitempty"`
	AvailableModels *[]string           `json:"availableModels,omitempty"`
	ModelNames      map[string]string   `json:"modelNames,omitempty"`
	ModelFree       map[string]bool     `json:"modelFree,omitempty"`
	ModelMetadata   map[string]metadata `json:"modelMetadata,omitempty"`
	MaxRPM          float64             `json:"maxRPM,omitempty"`
	MaxConcurrency  float64             `json:"maxConcurrency,omitempty"`
	QueueLimit      float64             `json:"queueLimit,omitempty"`
	QueueWait       float64             `json:"queueWait,omitempty"`
	HasKey          bool                `json:"hasKey"`
	Icon            string              `json:"icon,omitempty"`
	// Avatar is nil when the document has no choice (older clients),
	// "" when the user reset the picture to the brand's: the Host keeps
	// the old picture for nil only.
	Avatar          *string             `json:"avatar,omitempty"`
	Keys            *[]keySlot          `json:"keys,omitempty"`
	KeyPolicy       *keyPolicy          `json:"keyPolicy,omitempty"`
	Headers         map[string]string   `json:"headers,omitempty"`
	AdditionalTools bool                `json:"additionalTools,omitempty"`
	LimitRestOff    bool                `json:"limitRestOff,omitempty"`
}

type providerDoc = core.Preserved[provider]

// mark is the provider's picture: the user's own, or the brand's.
func (p *provider) mark() string {
	if p.Avatar != nil && *p.Avatar != "" {
		return *p.Avatar
	}
	return p.Icon
}

// available lists the models the provider offers, enabled first ones
// included, without repeats.
func (p *provider) available() []string {
	var all []string
	if p.AvailableModels != nil {
		all = append(all, *p.AvailableModels...)
	}
	all = append(all, p.Models...)
	return unique(all)
}

// slots returns the provider's keys. A configuration from before several
// keys shows its one key as the stable primary slot; showing it does not
// change the draft.
func (p *provider) slots() []keySlot {
	if p.Keys != nil {
		return *p.Keys
	}
	return []keySlot{{ID: "primary", Enabled: true, HasKey: p.HasKey}}
}

func (p *provider) policy() keyPolicy {
	if p.KeyPolicy != nil {
		return *p.KeyPolicy
	}
	return defaultPolicy
}

// redactRule is a custom masking rule: a kind, matched by prefix or regex.
type redactRule struct {
	Kind   string `json:"kind"`
	Prefix string `json:"prefix,omitempty"`
	Regex  string `json:"regex,omitempty"`
}

// searchAPI is a web search service. key is a new key, write-only; the
// Host answers hasKey once it protected the saved one.
type searchAPI struct {
	Vendor string `json:"vendor"`
	URL    string `json:"url,omitempty"`
	Key    string `json:"key,omitempty"`
	HasKey bool   `json:"hasKey,omitempty"`
	Off    bool   `json:"off,omitempty"`
	Pick   string `json:"pick,omitempty"`
}

// builtinChoice overrides a built-in plugin's preset.
type builtinChoice struct {
	Enabled *bool          `json:"enabled,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
}

// bundledChoice turns a bundled plugin on or off, with its options.
type bundledChoice struct {
	Enabled bool           `json:"enabled"`
	Options map[string]any `json:"options,omitempty"`
}

// magpieSettings is the part of Magpie's settings WinOne edits, named as
// in Magpie's Settings JSON. Numbers are pointers: a missing number takes
// a default that zero does not mean.
type magpieSettings struct {
	RequestArchiveLocal   bool                                    `json:"requestArchiveLocal,omitempty"`
	RequestArchiveLocalMB *float64                                `json:"requestArchiveLocalMB,omitempty"`
	RequestArchiveMaxMB   *float64                                `json:"requestArchiveMaxMB,omitempty"`
	Proxy                 string                                  `json:"proxy,omitempty"`
	ProxyCustom           string                                  `json:"proxyCustom,omitempty"`
	UsageAliases          map[string]string                       `json:"usageAliases,omitempty"`
	ClassifyTimeout       *float64                                `json:"classifyTimeout,omitempty"`
	ClaudeRunsMost        *float64                                `json:"claudeRunsMost,omitempty"`
	ClaudeLeanPrompt      bool                                    `json:"claudeLeanPrompt,omitempty"`
	FetchPrivate          []string                                `json:"fetchPrivate,omitempty"`
	BuiltinPreset         string                                  `json:"builtinPreset,omitempty"`
	BuiltinPlugins        map[string]core.Preserved[builtinChoice] `json:"builtinPlugins,omitempty"`
	BundledPlugins        map[string]core.Preserved[bundledChoice] `json:"bundledPlugins,omitempty"`
	QuotaLeft             bool                                    `json:"quotaLeft,omitempty"`
	QuotaReads            string                                  `json:"quotaReads,omitempty"`
	Currency              string                                  `json:"currency,omitempty"`
	ChineseUnits          bool                                    `json:"chineseUnits,omitempty"`
	UsageAlert            *float64                                `json:"usageAlert,omitempty"`
	BalanceAlert          *float64                                `json:"balanceAlert,omitempty"`
	ResetReminder         *float64                                `json:"resetReminder,omitempty"`
	TrayUsages            []string                                `json:"trayUsages,omitempty"`
	TrayUsageEvery        *float64                                `json:"trayUsageEvery,omitempty"`
	CodexWarmup           string                                  `json:"codexWarmup,omitempty"`
	ClaudeWarmup          string                                  `json:"claudeWarmup,omitempty"`
	CodexWarmAt           string                                  `json:"codexWarmAt,omitempty"`
	ClaudeWarmAt          string                                  `json:"claudeWarmAt,omitempty"`
	CodexWarmAts          *[]string                               `json:"codexWarmAts,omitempty"`
	ClaudeWarmAts         *[]string                               `json:"claudeWarmAts,omitempty"`
	CodexWarmAtOf         map[string]string                       `json:"codexWarmAtOf,omitempty"`
	CodexAutoReset        []string                                `json:"codexAutoReset,omitempty"`
	CodexNoCredits        []string                                `json:"codexNoCredits,omitempty"`
	WorkbuddyCheckin      bool                                    `json:"workbuddyCheckin,omitempty"`
	TraeCheckin           bool                                    `json:"traeCheckin,omitempty"`
	MinimaxCheckin        bool                                    `json:"minimaxCheckin,omitempty"`
	QoderCheckin          bool                                    `json:"qoderCheckin,omitempty"`
	PluginCheckins        map[string]bool                         `json:"pluginCheckins,omitempty"`
	Vision                string                                  `json:"vision,omitempty"`
	ImageGen              string                                  `json:"imageGen,omitempty"`
	ImageTimeout          *float64                                `json:"imageTimeout,omitempty"`
	Searcher              string                                  `json:"searcher,omitempty"`
	SearchFirst           string                                  `json:"searchFirst,omitempty"`
	SearchAPIs            []core.Preserved[searchAPI]             `json:"searchAPIs,omitempty"`
	MemberModel           bool                                    `json:"memberModel,omitempty"`
	NoLoopGuard           bool                                    `json:"noLoopGuard,omitempty"`
	PlainNames            bool                                    `json:"plainNames,omitempty"`
	PlainOwnNames         bool                                    `json:"plainOwnNames,omitempty"`
	CodexAgentsV1         bool                                    `json:"codexAgentsV1,omitempty"`
	FullContext           bool                                    `json:"fullContext,omitempty"`
	CompactAt             *float64                                `json:"compactAt,omitempty"`
	CodexTitles           string                                  `json:"codexTitles,omitempty"`
	CodexAutoReview       string                                  `json:"codexAutoReview,omitempty"`
	Redact                bool                                    `json:"redact,omitempty"`
	RedactShape           bool                                    `json:"redactShape,omitempty"`
	RedactPersonal        bool                                    `json:"redactPersonal,omitempty"`
	RedactKinds           map[string]bool                         `json:"redactKinds,omitempty"`
	RedactWords           []string                                `json:"redactWords,omitempty"`
	RedactWordsAnyCase    bool                                    `json:"redactWordsAnyCase,omitempty"`
	RedactRules           []core.Preserved[redactRule]            `json:"redactRules,omitempty"`
	RedactActions         map[string]string                       `json:"redactActions,omitempty"`
	RedactAllow           []string                                `json:"redactAllow,omitempty"`
	HideAccounts          bool                                    `json:"hideAccounts,omitempty"`
	NoStats               bool                                    `json:"noStats,omitempty"`
	NoUsageStats          bool                                    `json:"noUsageStats,omitempty"`
	NoAccountDiscovery    bool                                    `json:"noAccountDiscovery,omitempty"`
}

type magpieDoc = core.Preserved[magpieSettings]

// configuration is the whole document. The capture engines, target
// language and prompt belong to the Capture page and stay as read.
type configuration struct {
	RoutingEnabled  bool                             `json:"routingEnabled,omitempty"`
	Revision        float64                          `json:"revision"`
	Providers       []providerDoc                    `json:"providers"`
	Tasks           map[string]widgets.AISelection   `json:"tasks"`
	ShowReasoning   *bool                            `json:"showReasoning,omitempty"`
	AutoRetranslate *bool                            `json:"autoRetranslate,omitempty"`
	MagpieSettings  *magpieDoc                       `json:"magpieSettings,omitempty"`
}

type document = core.Preserved[configuration]

// normalize gives lists and maps the Host may omit their empty values, so
// the document saves them as lists and maps, not null.
func normalize(d *document) {
	c := &d.Value
	if c.Providers == nil {
		c.Providers = []providerDoc{}
	}
	if c.Tasks == nil {
		c.Tasks = map[string]widgets.AISelection{}
	}
	for i := range c.Providers {
		if c.Providers[i].Value.Models == nil {
			c.Providers[i].Value.Models = []string{}
		}
	}
}

// clone copies a document deeply, so a draft never shares a list or map
// with what was saved.
func clone(d document) document {
	data, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	var out document
	if err := json.Unmarshal(data, &out); err != nil {
		panic(err)
	}
	normalize(&out)
	return out
}

// equal compares documents as JSON, as React compared them.
func equal(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(x) == string(y)
}

// settings returns the draft's Magpie settings, creating them for an edit.
func (c *configuration) settings() *magpieSettings {
	if c.MagpieSettings == nil {
		c.MagpieSettings = &magpieDoc{}
	}
	return &c.MagpieSettings.Value
}

// magpie reads the Magpie settings, the zero settings when there are none.
func (c *configuration) magpie() magpieSettings {
	if c.MagpieSettings == nil {
		return magpieSettings{}
	}
	return c.MagpieSettings.Value
}

func (c *configuration) find(id string) *provider {
	for i := range c.Providers {
		if c.Providers[i].Value.ID == id {
			return &c.Providers[i].Value
		}
	}
	return nil
}

// picker is a provider as the model pickers and icons show it.
func (p *provider) picker() widgets.AIProvider {
	out := widgets.AIProvider{ID: p.ID, Name: p.Name, Icon: p.Icon, Avatar: value(p.Avatar), Models: p.Models,
		ModelNames: p.ModelNames, ModelFree: p.ModelFree}
	if p.AvailableModels != nil {
		out.AvailableModels = *p.AvailableModels
	}
	if len(p.ModelMetadata) > 0 {
		out.ModelMetadata = make(map[string]widgets.AIModelMetadata, len(p.ModelMetadata))
		for id, m := range p.ModelMetadata {
			v := m.Value
			out.ModelMetadata[id] = widgets.AIModelMetadata{Rate: value(v.Rate), RateWas: value(v.RateWas),
				ContextWindow: v.ContextWindow, ListedContextWindow: v.ListedContextWindow}
		}
	}
	return out
}

// unique keeps the first of each value, in order. Catalogs reach 4096
// models, so it remembers what it kept instead of searching the list.
func unique(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

func value[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func ptr[T any](v T) *T { return &v }

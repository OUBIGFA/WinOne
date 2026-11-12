package aiservices

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"winone/desktopmygo/core"
	"winone/desktopmygo/widgets"
	"winone/internal/ai"
)

// The configuration is the Host's capture.ai.configuration document. The
// page declares the fields it shows or edits; core.Preserved keeps the
// others (the capture engines, prompts and anything newer) as read, as
// React kept them by spreading the object it read.

// keySlot is one API key of a provider. Keys never leave the Host:
// hasKey tells whether one is saved, and new keys travel separately as
// keyUpdates when the configuration is saved.
type keySlot struct {
	ai.KeyOptions
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
	Alias               *string                     `json:"alias,omitempty"`
	Searches            *bool                       `json:"searches,omitempty"`
	ContextWindow       *float64                    `json:"contextWindow,omitempty"`
	MaxTokens           *float64                    `json:"maxTokens,omitempty"`
	CompactionThreshold *float64                    `json:"compactionThreshold,omitempty"`
	CompactionPercent   *float64                    `json:"compactionPercent,omitempty"`
	ListedContextWindow *float64                    `json:"listedContextWindow,omitempty"`
	MaxContextWindow    *float64                    `json:"maxContextWindow,omitempty"`
	Rate                *float64                    `json:"rate,omitempty"`
	RateWas             *float64                    `json:"rateWas,omitempty"`
	Reasoning           *bool                       `json:"reasoning,omitempty"`
	Vision              *bool                       `json:"vision,omitempty"`
	Tools               *bool                       `json:"tools,omitempty"`
	ReasoningEffort     *string                     `json:"reasoningEffort,omitempty"`
	Tier                *string                     `json:"tier,omitempty"`
	Same                *string                     `json:"same,omitempty"`
	UpstreamName        *string                     `json:"upstreamName,omitempty"`
	API                 *string                     `json:"api,omitempty"`
	Headers             map[string]string           `json:"headers,omitempty"`
	Levels              *[]string                   `json:"levels,omitempty"`
	Efforts             *[]string                   `json:"efforts,omitempty"`
	Tiers               *[]string                   `json:"tiers,omitempty"`
	Price               *core.Preserved[modelPrice] `json:"price,omitempty"`
}

type metadata = core.Preserved[modelMetadata]

// provider is one AI service: a custom API connection ("api-key") or an
// agent account ("account").
type provider struct {
	CatalogModels     []string                    `json:"catalogModels"`
	CatalogNames      map[string]string           `json:"catalogNames,omitempty"`
	ManualModels      []string                    `json:"manualModels,omitempty"`
	CatalogMetadata   map[string]ai.ModelMetadata `json:"catalogMetadata,omitempty"`
	ModelRefreshAt    time.Time                   `json:"modelRefreshAt,omitempty"`
	ModelRefreshError string                      `json:"modelRefreshError,omitempty"`
	ai.ProviderPreferences
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
	Avatar          *string           `json:"avatar,omitempty"`
	Keys            *[]keySlot        `json:"keys,omitempty"`
	KeyPolicy       *keyPolicy        `json:"keyPolicy,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	AdditionalTools bool              `json:"additionalTools,omitempty"`
	LimitRestOff    bool              `json:"limitRestOff,omitempty"`
}

type providerDoc = core.Preserved[provider]

// mark is the provider's picture: the user's own, or the brand's.
func (p *provider) mark() string {
	if p.Avatar != nil && *p.Avatar != "" {
		return *p.Avatar
	}
	return p.Icon
}

// available uses a stable model-ID order for both discovery drafts and saved
// configurations. Vendor response order and enabled picks cannot reorder it.
func (p *provider) available() []string {
	var all []string
	if p.AvailableModels != nil && !p.ListOff {
		all = append(all, *p.AvailableModels...)
	}
	all = append(all, p.Models...)
	all = unique(all)
	slices.SortFunc(all, func(a, b string) int {
		if order := strings.Compare(strings.ToLower(a), strings.ToLower(b)); order != 0 {
			return order
		}
		return strings.Compare(a, b)
	})
	return all
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

// Account execution mode belongs to account-provider setup. Other former
// global advanced options have no editable or executable surface here.
type magpieSettings struct {
	ClaudePrintMode bool `json:"claudePrintMode,omitempty"`
}

type magpieDoc = core.Preserved[magpieSettings]

// configuration is the whole document. The capture engines, target
// language and prompt belong to the Capture page and stay as read.
type configuration struct {
	Revision        float64                        `json:"revision"`
	Providers       []providerDoc                  `json:"providers"`
	Tasks           map[string]widgets.AISelection `json:"tasks"`
	ShowReasoning   *bool                          `json:"showReasoning,omitempty"`
	AutoRetranslate *bool                          `json:"autoRetranslate,omitempty"`
	MagpieSettings  *magpieDoc                     `json:"magpieSettings,omitempty"`
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
	out := widgets.AIProvider{ID: p.ID, Name: p.Name, Icon: p.Icon, Avatar: value(p.Avatar), Models: p.Models, Off: p.Off,
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

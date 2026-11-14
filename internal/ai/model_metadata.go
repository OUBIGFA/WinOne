package ai

import (
	"encoding/json"
	"errors"
	"maps"
	"math"
	"slices"
)

// ModelMetadata keeps provider capabilities separate from the routing ID.
// Token sizes retain their original units. CompactionPercent is a separate
// user override, so legacy token thresholds cannot be misread as percentages.
type ModelMetadata struct {
	Alias        string            `json:"alias,omitempty"`
	Searches     bool              `json:"searches,omitempty"`
	Same         string            `json:"same,omitempty"`
	UpstreamName string            `json:"upstreamName,omitempty"`
	API          string            `json:"api,omitempty"`
	Kind         string            `json:"kind,omitempty"`
	SystemPrompt string            `json:"systemPrompt,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	// Levels are the reasoning levels the vendor reports for the model, kept
	// apart from Efforts (the ones the user offers) so narrowing the offer
	// never loses a level from the choices. Both use the vendor's own words.
	Levels  []string    `json:"levels,omitempty"`
	Efforts []string    `json:"efforts,omitempty"`
	Tiers   []string    `json:"tiers,omitempty"`
	Price   *ModelPrice `json:"price,omitempty"`
	// Rates are vendor-listed credit multipliers, not USD token prices.
	Rate                float64 `json:"rate,omitempty"`
	RateWas             float64 `json:"rateWas,omitempty"`
	ContextWindow       int     `json:"contextWindow,omitempty"`
	MaxTokens           int     `json:"maxTokens,omitempty"`
	CompactionThreshold int     `json:"compactionThreshold,omitempty"`
	CompactionPercent   float64 `json:"compactionPercent,omitempty"`
	ListedContextWindow int     `json:"listedContextWindow,omitempty"`
	MaxContextWindow    int     `json:"maxContextWindow,omitempty"`
	Reasoning           *bool   `json:"reasoning,omitempty"`
	Vision              *bool   `json:"vision,omitempty"`
	Tools               *bool   `json:"tools,omitempty"`
	ReasoningEffort     string  `json:"reasoningEffort,omitempty"`
	Tier                string  `json:"tier,omitempty"`
}

func (m ModelMetadata) clone() ModelMetadata {
	m.Headers = maps.Clone(m.Headers)
	m.Levels = slices.Clone(m.Levels)
	m.Efforts = slices.Clone(m.Efforts)
	m.Tiers = slices.Clone(m.Tiers)
	if m.Price != nil {
		price := *m.Price
		price.Tiers = slices.Clone(price.Tiers)
		m.Price = &price
	}
	if m.Reasoning != nil {
		v := *m.Reasoning
		m.Reasoning = &v
	}
	if m.Vision != nil {
		v := *m.Vision
		m.Vision = &v
	}
	if m.Tools != nil {
		v := *m.Tools
		m.Tools = &v
	}
	return m
}

// ModelMetadataFor returns an independent snapshot for Pi and result requests.
func (p Provider) ModelMetadataFor(id string) ModelMetadata { return p.ModelMetadata[id].clone() }

// ModelDescriptionFor is for labels only. Never send this merged report as
// request settings: absent overrides must continue to use Magpie's catalog.
func (p Provider) ModelDescriptionFor(id string) ModelMetadata {
	shown := p.CatalogMetadata[id].clone()
	if edits, ok := p.ModelMetadata[id]; ok {
		raw, _ := json.Marshal(edits)
		_ = json.Unmarshal(raw, &shown)
	}
	return shown
}

// ValidateModelMetadata validates a draft before model tests or IPC execution.
func ValidateModelMetadata(m ModelMetadata) error { return validateModelMetadata(m) }

func validateModelMetadata(m ModelMetadata) error {
	if len(m.Headers) > 32 {
		return errors.New("每个模型最多设置 32 个请求头")
	}
	if err := validateProviderHeaders(m.Headers); err != nil {
		return err
	}
	if err := m.validateExtended(); err != nil {
		return err
	}
	for _, rate := range []float64{m.Rate, m.RateWas} {
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
			return errors.New("模型额度倍率必须是有效的非负数")
		}
	}
	for _, size := range []int{m.ContextWindow, m.MaxTokens, m.CompactionThreshold, m.ListedContextWindow, m.MaxContextWindow} {
		if size < 0 {
			return errors.New("模型 Token 数不能为负数")
		}
	}
	if math.IsNaN(m.CompactionPercent) || math.IsInf(m.CompactionPercent, 0) || m.CompactionPercent < 0 || m.CompactionPercent > 100 {
		return errors.New("压缩阈值百分比必须在 0–100 之间")
	}
	// Context and output are independent upstream values. Magpie's
	// OutputWithin bounds the published pair; its gateway owns request limits.
	// The default level and tier are the vendor's words too; Magpie fits the
	// requested level to the model's own list when it sends the request.
	if m.ReasoningEffort != "" && !validVendorLevel(m.ReasoningEffort) {
		return errors.New("模型思考档位无效")
	}
	if m.Tier != "" && !validVendorLevel(m.Tier) {
		return errors.New("模型服务档位无效")
	}
	return nil
}

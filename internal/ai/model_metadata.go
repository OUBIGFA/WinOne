package ai

import (
	"errors"
	"math"
	"slices"
)

// ModelMetadata keeps provider capabilities separate from the routing ID.
// Token sizes retain their original units. CompactionPercent is a separate
// user override, so legacy token thresholds cannot be misread as percentages.
type ModelMetadata struct {
	Searches bool `json:"searches,omitempty"`
	Same         string      `json:"same,omitempty"`
	UpstreamName string      `json:"upstreamName,omitempty"`
	API          string      `json:"api,omitempty"`
	Efforts      []string    `json:"efforts,omitempty"`
	Tiers        []string    `json:"tiers,omitempty"`
	Price        *ModelPrice `json:"price,omitempty"`
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

// ValidateModelMetadata validates a draft before model tests or IPC execution.
func ValidateModelMetadata(m ModelMetadata) error { return validateModelMetadata(m) }

func validateModelMetadata(m ModelMetadata) error {
	if err := m.validateExtended(); err != nil {
		return err
	}
	for _, rate := range []float64{m.Rate, m.RateWas} {
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
			return errors.New("模型额度倍率必须是有效的非负数")
		}
	}
	for _, size := range []int{m.ContextWindow, m.MaxTokens, m.CompactionThreshold, m.ListedContextWindow, m.MaxContextWindow} {
		if size < 0 || size > 100_000_000 {
			return errors.New("模型 Token 数必须在 0–100000000 之间")
		}
	}
	if math.IsNaN(m.CompactionPercent) || math.IsInf(m.CompactionPercent, 0) || m.CompactionPercent < 0 || m.CompactionPercent > 100 {
		return errors.New("压缩阈值百分比必须在 0–100 之间")
	}
	if m.ContextWindow > 0 && m.MaxTokens > m.ContextWindow {
		return errors.New("最大输出不能超过上下文窗口")
	}
	if !slices.Contains([]string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}, m.ReasoningEffort) {
		return errors.New("模型思考档位无效")
	}
	if !slices.Contains([]string{"", "auto", "default", "flex", "priority", "ultrafast"}, m.Tier) {
		return errors.New("模型服务档位无效")
	}
	return nil
}

// RequestLimits follow Magpie: limits are per key/account, zero means no bound,
// and queueWait is seconds. WinOne must retain them across one-shot workers.
type RequestLimits struct {
	MaxRPM         int `json:"maxRPM,omitempty"`
	MaxConcurrency int `json:"maxConcurrency,omitempty"`
	QueueLimit     int `json:"queueLimit,omitempty"`
	QueueWait      int `json:"queueWait,omitempty"`
}

func (l RequestLimits) validate() error {
	if l.MaxRPM < 0 || l.MaxRPM > 10000 || l.MaxConcurrency < 0 || l.MaxConcurrency > 1000 || l.QueueLimit < 0 || l.QueueLimit > 10000 || l.QueueWait < 0 || l.QueueWait > 3600 {
		return errors.New("请求限制无效：RPM 和排队上限为 0–10000，并发上限为 0–1000，等待为 0–3600 秒")
	}
	return nil
}

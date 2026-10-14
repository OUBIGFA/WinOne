package ai

import (
	"errors"
	"math"
	"slices"
	"strings"
)

// ModelPrice uses the upstream USD-per-million-token units. A nil price means
// catalog pricing; an explicit zero price means free, including after reload.
type ModelPrice struct {
	Input        float64          `json:"input"`
	Output       float64          `json:"output"`
	CacheRead    float64          `json:"cache_read"`
	CacheWrite   float64          `json:"cache_write"`
	CacheWrite1h float64          `json:"cache_write_1h,omitempty"`
	Tiers        []ModelPriceTier `json:"tiers,omitempty"`
}
type ModelPriceTier struct {
	Above        int     `json:"above"`
	Input        float64 `json:"input"`
	Output       float64 `json:"output"`
	CacheRead    float64 `json:"cache_read"`
	CacheWrite   float64 `json:"cache_write"`
	CacheWrite1h float64 `json:"cache_write_1h,omitempty"`
}

func (m ModelMetadata) validateExtended() error {
	if len(m.Same) > 256 || len(m.UpstreamName) > 256 || strings.ContainsAny(m.Same+m.UpstreamName, "\x00\r\n") {
		return errors.New("模型映射名称无效")
	}
	if !slices.Contains([]string{"", "chat", "responses", "anthropic", "gemini"}, m.API) {
		return errors.New("模型协议无效")
	}
	for _, effort := range m.Efforts {
		if !slices.Contains([]string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}, effort) {
			return errors.New("可用思考档位无效")
		}
	}
	if m.Price == nil {
		return nil
	}
	values := []float64{m.Price.Input, m.Price.Output, m.Price.CacheRead, m.Price.CacheWrite, m.Price.CacheWrite1h}
	previous := 0
	for _, tier := range m.Price.Tiers {
		if tier.Above <= previous {
			return errors.New("分档价格阈值须递增且大于 0")
		}
		previous = tier.Above
		values = append(values, tier.Input, tier.Output, tier.CacheRead, tier.CacheWrite, tier.CacheWrite1h)
	}
	for _, price := range values {
		if math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
			return errors.New("模型价格必须为有限的非负数")
		}
	}
	return nil
}

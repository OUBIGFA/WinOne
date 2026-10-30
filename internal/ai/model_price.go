package ai

import (
	"errors"
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
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

// Reasoning levels and service tiers are the vendor's own words. Magpie keeps
// whatever a provider or plugin lists (light/balanced/deep on Our Free Model,
// persistent on Codex) and checks a kept level against that model's list at
// the worker, where the catalog is (provider.SetModelEfforts). The Host has no
// catalog, so it only rejects what cannot be a level: empty, control text, a
// colon (Magpie's "model:effort" member syntax) or an unbounded value. It must
// not substitute a fixed ladder, which turned valid plugin levels into
// "可用思考档位无效".
func validVendorLevel(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.RuneCountInString(value) <= 64 &&
		!strings.ContainsRune(value, ':') && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validVendorLevels(values []string) bool {
	if len(values) > 64 {
		return false
	}
	for _, value := range values {
		if !validVendorLevel(value) {
			return false
		}
	}
	return true
}

func (m ModelMetadata) validateExtended() error {
	if len(m.Same) > 256 || len(m.UpstreamName) > 256 || strings.ContainsAny(m.Same+m.UpstreamName, "\x00\r\n") {
		return errors.New("模型映射名称无效")
	}
	if !slices.Contains([]string{"", "chat", "responses", "anthropic", "gemini"}, m.API) {
		return errors.New("模型协议无效")
	}
	if !validVendorLevels(m.Efforts) || !validVendorLevels(m.Levels) {
		return errors.New("可用思考档位无效")
	}
	if !validVendorLevels(m.Tiers) {
		return errors.New("可用服务档位无效")
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

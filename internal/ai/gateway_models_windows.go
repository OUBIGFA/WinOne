//go:build windows

package ai

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

func gatewayModelSet(models []Selection) (map[Selection]struct{}, error) {
	if models == nil {
		return nil, nil
	}
	if len(models) > 16384 {
		return nil, errors.New("网关模型数量过多")
	}
	selected := make(map[Selection]struct{}, len(models))
	for _, model := range models {
		if !providerIDPattern.MatchString(model.Provider) || model.Model == "" || len(model.Model) > 256 || !utf8.ValidString(model.Model) || strings.TrimSpace(model.Model) != model.Model || strings.ContainsFunc(model.Model, unicode.IsControl) {
			return nil, errors.New("网关模型选择无效，请重新选择")
		}
		if _, exists := selected[model]; exists {
			return nil, errors.New("网关模型选择重复")
		}
		selected[model] = struct{}{}
	}
	return selected, nil
}

// Capture one immutable policy for the complete HTTP request. A concurrent
// settings save cannot mix old and new choices within the same model list.
func (g *Gateway) ModelFilter() func(Selection) bool {
	g.mu.Lock()
	selected := g.models
	g.mu.Unlock()
	return func(model Selection) bool {
		if selected == nil {
			return true
		}
		_, ok := selected[model]
		return ok
	}
}

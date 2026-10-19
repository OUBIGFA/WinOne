package ai

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"slices"
)

var builtinPluginIDs = []string{"budget", "gateway", "groups", "library", "middleware", "otel", "redact", "rotation", "routing", "search", "sessions", "tools", "usage", "warmups"}
var pluginIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`)

func (s *MagpieSettings) validatePlugins() error {
	if !slices.Contains([]string{"", "all", "essentials"}, s.BuiltinPreset) {
		return errors.New("内置插件预设无效")
	}
	for id, choice := range s.BuiltinPlugins {
		if !slices.Contains(builtinPluginIDs, id) {
			return errors.New("存在当前版本未提供的内置插件")
		}
		// Magpie 674b6ca declares one schema: gateway.callsPage. Reject values the
		// runtime would otherwise ignore; update this contract with upstream schema.
		for key, value := range choice.Config {
			encoded, err := json.Marshal(value)
			if err != nil {
				return errors.New("插件设置值无效")
			}
			var number float64
			if id != "gateway" || key != "callsPage" || json.Unmarshal(encoded, &number) != nil || math.Trunc(number) != number || !slices.Contains([]float64{10, 20, 50, 100}, number) {
				return errors.New("插件设置不符合当前版本的参数定义")
			}
		}
	}
	if len(s.BundledPlugins) > 64 || len(s.PluginOptions) > 64 {
		return errors.New("插件设置过多")
	}
	for id, choice := range s.BundledPlugins {
		data, err := json.Marshal(choice.Options)
		if !pluginIDPattern.MatchString(id) || err != nil || len(data) > 32<<10 {
			return errors.New("内置扩展插件设置无效或过大")
		}
	}
	for provider, options := range s.PluginOptions {
		if !pluginIDPattern.MatchString(provider) || len(options) > 16 {
			return errors.New("账号插件选项无效")
		}
		for key := range options {
			if len(key) > 40 || !pluginIDPattern.MatchString(key) {
				return errors.New("账号插件选项名称无效")
			}
		}
	}
	return nil
}

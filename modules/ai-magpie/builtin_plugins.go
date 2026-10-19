package main

import (
	"encoding/json"
	"fmt"
	"log"
	"slices"

	"github.com/yetone/magpie/internal/gwbudget"
	"github.com/yetone/magpie/internal/gwmw"
	"github.com/yetone/magpie/internal/gwtool"
	"github.com/yetone/magpie/internal/pluginrt"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/usage"
)

type builtinPluginChoice struct {
	Enabled *bool          `json:"enabled,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
}

// Validate with the upstream schema before publishing any setting. Compose's
// forgiving treatment of a hand-edited YAML file is not a successful UI save.
func applyBuiltinPlugins(raw json.RawMessage) error {
	var selected struct {
		Preset  string                         `json:"builtinPreset"`
		Plugins map[string]builtinPluginChoice `json:"builtinPlugins"`
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &selected); err != nil {
			return err
		}
	}
	if selected.Preset == "" {
		selected.Preset = pluginrt.PresetAll
	}
	if selected.Preset != pluginrt.PresetAll && selected.Preset != pluginrt.PresetEssentials {
		return fmt.Errorf("invalid built-in plugin preset")
	}
	registered := pluginrt.Registered()
	file := pluginrt.File{Preset: selected.Preset}
	for id, choice := range selected.Plugins {
		index := slices.IndexFunc(registered, func(m pluginrt.Manifest) bool { return m.ID == id })
		if index < 0 {
			return fmt.Errorf("unknown built-in plugin %q", id)
		}
		if len(choice.Config) > 0 {
			schema := registered[index].Schema()
			if schema == nil {
				return fmt.Errorf("plugin %s has no settings", id)
			}
			checked, err := schema.Validate(choice.Config)
			if err != nil {
				return fmt.Errorf("%s: %w", id, err)
			}
			choice.Config = checked
		}
		file.Plugins = append(file.Plugins, pluginrt.Row{ID: id, Enabled: choice.Enabled, Config: choice.Config})
	}
	pluginrt.WinOneUse(file)
	return nil
}

// Mirrors gui/builtins.go without exposing the Magpie application shell. The
// UI consumes every manifest, schema field and lifecycle state, not a shortlist.
func builtinPluginCatalog() ([]map[string]any, error) {
	composed, err := pluginrt.Refresh()
	if err != nil {
		return nil, err
	}
	statuses := map[string]pluginrt.Status{}
	for _, status := range pluginrt.Running().Statuses() {
		statuses[status.ID] = status
	}
	result := []map[string]any{}
	for _, manifest := range pluginrt.Registered() {
		index := slices.IndexFunc(composed, func(c pluginrt.Composed) bool { return c.ID == manifest.ID })
		if index < 0 {
			return nil, fmt.Errorf("plugin %s was not composed", manifest.ID)
		}
		c, status := composed[index], statuses[manifest.ID]
		why := manifest.One
		if c.InUse > 1 {
			why = manifest.Many
		}
		result = append(result, map[string]any{"id": manifest.ID, "name": manifest.Name, "sub": manifest.Sub, "essential": manifest.Essential, "on": status.State == pluginrt.Active, "chosen": c.Chosen, "source": c.Source, "state": status.State, "why": status.Why, "inUse": c.InUse, "use": why, "schema": manifest.Schema(), "config": c.Config, "requires": manifest.Requires, "provides": manifest.Provides, "parts": status.Parts})
	}
	return result, nil
}

// gateway.Handler does not run ListenAndServe's plugin lifecycle. WinOne owns
// the listener, so reproduce those exact service gates for its embedded calls.
// This makes an off switch affect execution, not just the page's appearance.
func serveBuiltinPlugins() func() {
	refresh := func() {
		if _, err := pluginrt.Refresh(); err != nil {
			log.Printf("plugins: %v", err)
		}
	}
	stops := []func(){usage.ServeObservers(), provider.GateGroups(refresh), gwtool.Gate(), gwmw.Refresh(refresh), gwbudget.Refresh(refresh)}
	return func() {
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
	}
}

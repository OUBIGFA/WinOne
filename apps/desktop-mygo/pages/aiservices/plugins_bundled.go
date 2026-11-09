package aiservices

import (
	"encoding/json"
	"slices"
	"winone/desktopmygo/core"
)

// Bundled code is installed with WinOne, while its enable/options choice is in
// the Host configuration. Present it in Installed and use that same choice;
// never write packaged source paths into the third-party installation registry.
func (x *pluginsPanel) effectivePlugins() []managedPlugin {
	var rows []managedPlugin
	if x.state != nil {
		rows = slices.Clone(x.state.Plugins)
	}
	if x.p.advanced == nil || x.p.advanced.catalog == nil || x.p.draft == nil {
		return rows
	}
	for _, plugin := range x.p.advanced.catalog.BundledPlugins {
		if slices.ContainsFunc(rows, func(p managedPlugin) bool { return p.Spec == plugin.Package || p.Package == plugin.Package }) {
			continue
		}
		on := len(plugin.Providers) > 0
		choice, chosen := x.p.draft.Value.settings().BundledPlugins[plugin.Directory]
		if choice.Value.Removed {
			continue
		}
		if chosen {
			on = choice.Value.Enabled
		}
		row := managedPlugin{Spec: plugin.Package, Package: plugin.Package, BundleDirectory: plugin.Directory, OwnName: plugin.Name, Version: plugin.Version, Off: !on, Providers: plugin.Providers, Options: choice.Value.Options, OptionsExample: map[string]any{}, IsMiddleware: plugin.Kind == "middleware", IsGateway: plugin.Kind == "hooks"}
		row.Raw, _ = json.Marshal(row)
		rows = append(rows, row)
	}
	return rows
}

func (x *pluginsPanel) bundleAction(action string, body any) bool {
	bytes, _ := json.Marshal(body)
	var in struct {
		Spec    string
		Off     bool
		Options map[string]any
	}
	_ = json.Unmarshal(bytes, &in)
	if in.Spec == "" {
		return false
	}
	var found *managedPlugin
	for _, row := range x.effectivePlugins() {
		if row.Spec == in.Spec && row.BundleDirectory != "" {
			copy := row
			found = &copy
			break
		}
	}
	// Packaged files stay immutable, but removing the plugin removes its
	// installed registration. Reinstall restores that explicit saved choice.
	if found == nil && action == "add" && x.p.advanced != nil && x.p.advanced.catalog != nil {
		for _, plugin := range x.p.advanced.catalog.BundledPlugins {
			if plugin.Package == in.Spec {
				found = &managedPlugin{Spec: in.Spec, BundleDirectory: plugin.Directory, Off: true}
				break
			}
		}
	}
	if found == nil {
		return false
	}
	if action != "off" && action != "options" && action != "remove" && action != "add" {
		return false
	}
	if x.p.dirty() {
		x.api.err = upstreamT("Save your changes first")
		return true
	}
	settings := x.p.draft.Value.settings()
	if settings.BundledPlugins == nil {
		settings.BundledPlugins = map[string]core.Preserved[bundledChoice]{}
	}
	choice, exists := settings.BundledPlugins[found.BundleDirectory]
	if !exists {
		choice.Value.Enabled = !found.Off
	}
	switch action {
	case "add":
		choice.Value.Enabled = true
		choice.Value.Removed = false
	case "off":
		choice.Value.Enabled = !in.Off
		choice.Value.Removed = false
	case "remove":
		choice.Value.Enabled = false
		choice.Value.Removed = true
		choice.Value.Options = nil
	case "options":
		choice.Value.Options = in.Options
	}
	settings.BundledPlugins[found.BundleDirectory] = choice
	x.discard()
	x.remove = ""
	x.p.save()
	return true
}

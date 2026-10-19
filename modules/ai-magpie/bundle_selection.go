package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/yetone/magpie/internal/plugin"
)

type bundledPluginChoice struct {
	Enabled bool           `json:"enabled"`
	Options map[string]any `json:"options,omitempty"`
}

var selectedBundles map[string]bundledPluginChoice

func loadBundledSelection(raw json.RawMessage) error {
	var data struct {
		Plugins map[string]bundledPluginChoice `json:"bundledPlugins"`
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
	}
	for id := range data.Plugins {
		found := false
		for _, p := range accountPlugins() {
			if p.Directory == id {
				found = true
				break
			}
		}
		if !found {
			return errors.New("unknown bundled plugin")
		}
	}
	selectedBundles = data.Plugins
	return nil
}

func checkBundle(ctx context.Context, root string, p accountPlugin) error {
	for _, resource := range p.Files {
		if filepath.IsAbs(resource.Path) || filepath.Clean(resource.Path) == ".." {
			return errors.New("invalid bundled plugin resource")
		}
		if err := checkPluginResource(ctx, filepath.Join(root, "plugins", p.Directory, filepath.FromSlash(resource.Path)), resource.SHA256, resource.Size); err != nil {
			return err
		}
	}
	return nil
}

// Register only the explicitly enabled, verified bundled extensions. Provider
// plugins remain lazy and are loaded when their own account is selected. No
// npm install, upstream auto-migration or external app discovery runs here.
func prepareBundledExtensions(ctx context.Context) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	root := filepath.Dir(executable)
	for _, p := range accountPlugins() {
		choice, chosen := selectedBundles[p.Directory]
		spec := filepath.Join(root, "plugins", p.Directory)
		var existing *plugin.Entry
		for _, entry := range plugin.Load().Plugins {
			if entry.Spec == spec {
				copy := entry
				existing = &copy
				break
			}
		}
		if !chosen || !choice.Enabled {
			if existing != nil && chosen && !existing.Off {
				if err := plugin.SetOff(spec, true); err != nil {
					return err
				}
			}
			continue
		}
		if err := checkBundle(ctx, root, p); err != nil {
			return err
		}
		bun := filepath.Join(root, "runtime", "bun.exe")
		if err := checkPluginResource(ctx, bun, bundledPluginResources.Bun.SHA256, -1); err != nil {
			return err
		}
		if err := os.Setenv("MAGPIE_BUN", bun); err != nil {
			return err
		}
		if plugin.WinOneDirectory() != "" {
			plugin.WinOneUseBundle(plugin.Entry{Spec: spec, Options: choice.Options})
			continue
		}
		if existing == nil {
			if _, err := plugin.Add(ctx, spec); err != nil {
				return err
			}
		} else if existing.Off {
			if err := plugin.SetOff(spec, false); err != nil {
				return err
			}
		}
		if choice.Options != nil {
			if err := plugin.SetOptions(spec, choice.Options); err != nil {
				return err
			}
		}
	}
	return nil
}

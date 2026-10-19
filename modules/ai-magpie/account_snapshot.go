package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/yetone/magpie/internal/plugin"
)

// Ordinary account-page reads follow magpie's pluginSubs/Cached path. Only a
// new profile, another installation or a changed bundle needs plugin discovery;
// sign-in and model calls continue to verify and start the real plugin.
func pluginAccountSnapshot(ctx context.Context, agent string) (plugin.Provider, error) {
	if err := ctx.Err(); err != nil {
		return plugin.Provider{}, err
	}
	descriptor, ok := pluginForAgent(agent)
	if !ok {
		return preparePlugin(ctx, agent)
	}
	if choice, chosen := selectedBundles[descriptor.Directory]; chosen && !choice.Enabled {
		return plugin.Provider{}, errors.New("account plugin is disabled; enable it in AI services > Advanced settings")
	}
	executable, err := os.Executable()
	if err != nil {
		return plugin.Provider{}, err
	}
	root := filepath.Dir(executable)
	data, err := json.Marshal(struct {
		Plugin  accountPlugin
		Runtime string
	}{descriptor, bundledPluginResources.Bun.SHA256})
	if err != nil {
		return plugin.Provider{}, err
	}
	digest := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(digest[:])
	spec := filepath.Join(root, "plugins", descriptor.Directory)
	if plugin.WinOneDirectory() != "" {
		entry := plugin.Entry{Spec: spec}
		if choice, ok := selectedBundles[descriptor.Directory]; ok {
			entry.Options = choice.Options
		}
		plugin.WinOneUseBundle(entry)
	}
	if snapshot, found, err := plugin.WinOneReadProvider(spec, agent, fingerprint); err != nil || found {
		return snapshot, err
	}
	snapshot, err := preparePluginAt(ctx, root, agent)
	if err != nil {
		return plugin.Provider{}, err
	}
	if err = plugin.WinOneRememberProvider(snapshot, fingerprint); err != nil {
		return plugin.Provider{}, err
	}
	return snapshot, nil
}

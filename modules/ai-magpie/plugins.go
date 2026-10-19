package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

func checkPluginResource(ctx context.Context, filename, digest string, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stat, err := os.Lstat(filename)
	if err != nil || !stat.Mode().IsRegular() || size >= 0 && stat.Size() != size {
		return errors.New("AI plugin resource is missing or damaged; repair WinOne")
	}
	file, err := os.Open(filename)
	if err != nil {
		return errors.New("AI plugin resource cannot be read")
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, 128<<20)); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != digest {
		return errors.New("AI plugin resource checksum does not match; repair WinOne")
	}
	return ctx.Err()
}

// Each WinOne provider already runs in its own private magpie profile. It
// registers exactly one packaged plugin; neither catalog reads nor provider
// calls may install arbitrary npm packages or select another application's home.
func preparePluginAt(ctx context.Context, root, agent string) (plugin.Provider, error) {
	descriptor, found := pluginForAgent(agent)
	if !found {
		return plugin.Provider{}, errors.New("unsupported account plugin")
	}
	if choice, exists := selectedBundles[descriptor.Directory]; exists && !choice.Enabled {
		return plugin.Provider{}, errors.New("account plugin is disabled; enable it in AI services > Plugins")
	}
	directory := filepath.Join(root, "plugins", descriptor.Directory)
	for _, resource := range descriptor.Files {
		if filepath.IsAbs(resource.Path) || strings.Contains(resource.Path, "..") {
			return plugin.Provider{}, errors.New("invalid bundled plugin path")
		}
		if err := checkPluginResource(ctx, filepath.Join(directory, filepath.FromSlash(resource.Path)), resource.SHA256, resource.Size); err != nil {
			return plugin.Provider{}, err
		}
	}
	bun := filepath.Join(root, "runtime", "bun.exe")
	if err := checkPluginResource(ctx, bun, bundledPluginResources.Bun.SHA256, -1); err != nil {
		return plugin.Provider{}, err
	}
	if err := os.Setenv("MAGPIE_BUN", bun); err != nil {
		return plugin.Provider{}, err
	}
	entries := plugin.Load().Plugins
	if plugin.WinOneDirectory() != "" {
		// User-installed extensions are deliberately loaded beside this one
		// verified account backend; its registration is not an installation.
		entry := plugin.Entry{Spec: directory}
		if choice, ok := selectedBundles[descriptor.Directory]; ok {
			entry.Options = choice.Options
		}
		plugin.WinOneUseBundle(entry)
		entries = nil
	}
	registered := false
	if plugin.WinOneDirectory() != "" {
		registered = true
	}
	for _, entry := range entries {
		previous := filepath.ToSlash(filepath.Clean(entry.Spec))
		if entry.Spec == directory {
			registered = true
			continue
		}
		// Each explicitly enabled bundled extension can coexist with this
		// account. Unknown profile code never becomes an implicit fallback.
		known := false
		for _, p := range accountPlugins() {
			if filepath.IsAbs(entry.Spec) && strings.HasSuffix(strings.ToLower(previous), "/shared/ai/plugins/"+p.Directory) || entry.Spec == filepath.Join(root, "plugins", p.Directory) {
				known = true
				break
			}
		}
		if !known {
			return plugin.Provider{}, errors.New("account plugin profile contains an unverified plugin")
		}
		if strings.HasSuffix(strings.ToLower(previous), "/shared/ai/plugins/"+descriptor.Directory) {
			if err := plugin.WinOneRebindPath(entry.Spec, directory); err != nil {
				return plugin.Provider{}, err
			}
			registered = true
		}
	}
	if !registered {
		if _, err := plugin.Add(ctx, directory); err != nil {
			return plugin.Provider{}, errors.New("account plugin could not be prepared")
		}
	}
	providers, err := plugin.Providers(ctx)
	if err != nil {
		return plugin.Provider{}, errors.New("account plugin could not start")
	}
	provider.Changed()
	for _, candidate := range providers {
		if candidate.ID == agent {
			return candidate, nil
		}
	}
	return plugin.Provider{}, errors.New("account plugin does not expose the selected provider")
}

func preparePlugin(ctx context.Context, agent string) (plugin.Provider, error) {
	if _, bundled := pluginForAgent(agent); !bundled {
		if plugin.WinOneDirectory() == "" {
			return plugin.Provider{}, errors.New("unsupported account plugin")
		}
		if err := prepareThirdPartyRuntime("accounts.execute"); err != nil {
			return plugin.Provider{}, err
		}
		providers, err := plugin.Providers(ctx)
		if err != nil {
			return plugin.Provider{}, errors.New("installed account plugin could not start")
		}
		provider.Changed()
		for _, candidate := range providers {
			if candidate.ID != agent {
				continue
			}
			for _, entry := range plugin.WinOneManaged(plugin.Load()).Plugins {
				if !entry.Off && entry.Spec == candidate.Spec {
					return candidate, nil
				}
			}
		}
		return plugin.Provider{}, errors.New("installed account plugin is missing or disabled")
	}
	executable, err := os.Executable()
	if err != nil {
		return plugin.Provider{}, err
	}
	return preparePluginAt(ctx, filepath.Dir(executable), agent)
}

// Install operations reuse WinOne's verified Bun. Missing or damaged runtime
// resources must fail explicitly instead of downloading a second executable.
func prepareThirdPartyRuntime(operation string) error {
	// A cached public summary reads current private auth without executing
	// plugin code. Discovery on a cache miss validates Bun in preparePlugin.
	if operation == "accounts.list" {
		return nil
	}
	if plugin.WinOneDirectory() == "" || operation != "routing.serve" && len(plugin.Load().Plugins) == 0 {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	bun := filepath.Join(filepath.Dir(executable), "runtime", "bun.exe")
	if err := checkPluginResource(context.Background(), bun, bundledPluginResources.Bun.SHA256, -1); err != nil {
		return err
	}
	return os.Setenv("MAGPIE_BUN", bun)
}

func findAccount(ctx context.Context, agent string) (*provider.Provider, error) {
	if settings.Load().NoAccountDiscovery {
		return nil, errors.New("WinOne subscription accounts are disabled in privacy settings")
	}
	provider.WinOneProviders = nil
	_, bundled := pluginForAgent(agent)
	if bundled || !knownAgent(agent) && plugin.WinOneDirectory() != "" {
		if _, err := preparePlugin(ctx, agent); err != nil {
			return nil, err
		}
		agent = provider.PluginID(agent)
	} else if !knownAgent(agent) {
		return nil, errors.New("unsupported account connection")
	}
	account, err := provider.Find(agent)
	if err != nil || account == nil || account.Account == nil {
		return nil, errors.New("sign in to the selected agent first")
	}
	return account, nil
}

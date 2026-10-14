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
	if len(entries) > 1 {
		return plugin.Provider{}, errors.New("account plugin profile does not match its provider")
	}
	if len(entries) == 1 && entries[0].Spec != directory {
		previous := filepath.ToSlash(filepath.Clean(entries[0].Spec))
		// Registered and portable copies can share a profile. Only replace a
		// WinOne bundle location for this exact plugin; never execute that path.
		if !filepath.IsAbs(entries[0].Spec) || !strings.HasSuffix(strings.ToLower(previous), "/shared/ai/plugins/"+descriptor.Directory) {
			return plugin.Provider{}, errors.New("account plugin profile does not match its provider")
		}
		if err := plugin.WinOneRebindPath(entries[0].Spec, directory); err != nil {
			return plugin.Provider{}, err
		}
	}
	if len(entries) == 0 {
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
	executable, err := os.Executable()
	if err != nil {
		return plugin.Provider{}, err
	}
	return preparePluginAt(ctx, filepath.Dir(executable), agent)
}

func findAccount(ctx context.Context, agent, backend string) (*provider.Provider, error) {
	if settings.Load().NoAccountDiscovery {
		return nil, errors.New("WinOne subscription accounts are disabled in privacy settings")
	}
	provider.WinOneProviders = nil
	if backend == "plugin" {
		if _, err := preparePlugin(ctx, agent); err != nil {
			return nil, err
		}
		agent = provider.PluginID(agent)
	} else if backend != "" && backend != "builtin" || !knownAgent(agent) {
		return nil, errors.New("unsupported account connection")
	}
	account, err := provider.Find(agent)
	if err != nil || account == nil || account.Account == nil {
		return nil, errors.New("sign in to the selected agent first")
	}
	return account, nil
}

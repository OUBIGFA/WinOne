package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/provider"
)

func moveAntigravityAccounts(ctx context.Context, in request) error {
	if in.Agent != "antigravity" {
		return errors.New("unsupported account migration")
	}
	// Check the private source before starting any adapter or native client.
	if len(provider.AgyMoveUsers()) == 0 {
		return errors.New("no Antigravity account is left to move")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	root := filepath.Dir(executable)
	var entries []plugin.Entry
	version := ""
	for _, id := range []string{"antigravity", "agy-cli"} {
		descriptor, ok := pluginForAgent(id)
		if !ok {
			return errors.New("account migration adapter is not bundled")
		}
		if _, err := preparePluginAt(ctx, root, id); err != nil {
			return err
		}
		entries = append(entries, plugin.Entry{Spec: filepath.Join(root, "plugins", descriptor.Directory)})
		if id == "agy-cli" {
			version = descriptor.Version
		}
	}
	plugin.WinOneUseMigrationBundles(entries)
	provider.Changed()
	// Require a successful installed-client check. The upstream fallback for
	// an unreadable check must not turn a WinOne migration into an installer.
	installed, err := plugin.CLIInstalled(ctx, "agy-cli")
	if err != nil {
		return errors.New("Antigravity CLI installation could not be checked")
	}
	if !installed {
		return errors.New("Antigravity CLI (agy) is required; install the official client first")
	}
	results, err := provider.WinOneMoveToAgy(ctx, version)
	if err != nil {
		return err
	}
	for i := range results {
		if results[i].Error != "" {
			results[i].Error = "Account could not be moved"
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"results": results})
}

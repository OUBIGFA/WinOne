package plugin

import (
	"github.com/yetone/magpie/internal/settings"
	"os"
	"path/filepath"
	"testing"
)

func clearAccountRegistry(t *testing.T) {
	t.Helper()
	winOneAccount.Lock()
	winOneAccount.entry = nil
	winOneAccount.migration = nil
	winOneAccount.Unlock()
	t.Cleanup(func() {
		winOneAccount.Lock()
		winOneAccount.entry = nil
		winOneAccount.migration = nil
		winOneAccount.Unlock()
		UseCached(nil)
	})
}

func TestWinOneAccountRegistryKeepsOnlyTwoVerifiedMigrationBundles(t *testing.T) {
	clearAccountRegistry(t)
	WinOneUseMigrationBundles([]Entry{{Spec: "old-verified"}, {Spec: "new-verified"}})
	entries := Load().Plugins
	if len(entries) != 2 || entries[0].Spec != "old-verified" || entries[1].Spec != "new-verified" {
		t.Fatal("migration lost its verified source/target registry")
	}
	WinOneUseBundle(Entry{Spec: "new-verified"})
	if entries = Load().Plugins; len(entries) != 1 || entries[0].Spec != "new-verified" {
		t.Fatal("ordinary account read retained the migration's extra bundle")
	}
}

func TestWinOneAccountRegistryIgnoresDiskExtensionsAndKeepsPrivateAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	clearAccountRegistry(t)
	if err := os.MkdirAll(settings.Dir(), 0700); err != nil {
		t.Fatal(err)
	}
	old := []byte(`{"plugins":[{"spec":"unreviewed-package"}]}`)
	if err := os.WriteFile(ListPath(), old, 0600); err != nil {
		t.Fatal(err)
	}
	if len(Load().Plugins) != 0 {
		t.Fatal("old extensions became executable")
	}
	WinOneUseBundle(Entry{Spec: "verified-account", Options: map[string]any{"site": "one"}})
	got := Load()
	if len(got.Plugins) != 1 || got.Plugins[0].Spec != "verified-account" {
		t.Fatal("verified account absent")
	}
	got.Plugins[0].Options["site"] = "mutated"
	if Load().Plugins[0].Options["site"] != "one" {
		t.Fatal("reader mutated account snapshot")
	}
	WinOneUseBundle(Entry{Spec: "replacement-account"})
	if list := Load(); len(list.Plugins) != 1 || list.Plugins[0].Spec != "replacement-account" {
		t.Fatal("account snapshot accumulated adapters")
	}
	data, err := os.ReadFile(ListPath())
	if err != nil || string(data) != string(old) {
		t.Fatal("loading changed the legacy registry", err)
	}
	if AuthPath() != filepath.Join(settings.Dir(), "plugin-auth.json") {
		t.Fatal("account authentication escaped its private profile")
	}
}

func TestWinOneCachedProvidersIncludeTheBundle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	clearAccountRegistry(t)
	WinOneUseBundle(Entry{Spec: "verified-account"})
	UseCached([]Provider{{ID: "bundled", Spec: "verified-account"}})
	got := Cached()
	if len(got) != 1 || got[0].ID != "bundled" {
		t.Fatalf("verified account missing: %+v", got)
	}
}

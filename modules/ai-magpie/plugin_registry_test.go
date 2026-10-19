package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/settings"
)

// Sharing plugin code must never share two providers' login credentials.
func TestWinOneExtensionStoreKeepsPrivateAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	privateAuth := filepath.Join(settings.Dir(), "plugin-auth.json")
	shared := t.TempDir()
	t.Setenv("WINONE_AI_PLUGINS_HOME", shared)
	if got := Dir(); got != filepath.Join(shared, "plugins") {
		t.Fatalf("plugin directory %q is not shared", got)
	}
	if got := ListPath(); got != filepath.Join(shared, "plugins.json") {
		t.Fatalf("plugin registry %q is not shared", got)
	}
	if got := AuthPath(); got != privateAuth {
		t.Fatalf("private auth escaped to %q", got)
	}
	if err := os.WriteFile(ListPath(), []byte("{\"plugins\":[{\"spec\":\"third-party-example\"}]}"), 0600); err != nil {
		t.Fatal(err)
	}
	l := Load()
	if len(l.Plugins) != 1 || l.Plugins[0].Spec != "third-party-example" {
		t.Fatalf("shared plugin not loaded: %#v", l)
	}
	if err := SetOff("third-party-example", true); err != nil {
		t.Fatal(err)
	}
	if !Load().Plugins[0].Off {
		t.Fatal("switch was not persisted")
	}
	if _, err := os.Stat(privateAuth); !os.IsNotExist(err) {
		t.Fatalf("plugin management touched private auth: %v", err)
	}
}

func TestWinOneExtensionBundleIsEphemeralAndOptionsAreIndependent(t *testing.T) {
	t.Setenv("WINONE_AI_PLUGINS_HOME", t.TempDir())
	if err := os.WriteFile(ListPath(), []byte("{\"plugins\":[{\"spec\":\"third-party-example\",\"options\":{\"value\":1}}]}"), 0600); err != nil {
		t.Fatal(err)
	}
	winOneBundles.Lock()
	winOneBundles.entries = nil
	winOneBundles.Unlock()
	t.Cleanup(func() { winOneBundles.Lock(); winOneBundles.entries = nil; winOneBundles.Unlock() })
	WinOneUseBundle(Entry{Spec: "verified-bundled-account", Options: map[string]any{"token": "private-option"}})
	effective := Load()
	if len(effective.Plugins) != 2 {
		t.Fatalf("request lost its account backend: %#v", effective)
	}
	effective.Plugins[1].Options["token"] = "mutated"
	if Load().Plugins[1].Options["token"] != "private-option" {
		t.Fatal("request mutated shared account options")
	}
	if err := SetOff("third-party-example", true); err != nil {
		t.Fatal(err)
	}
	managed := WinOneManaged(Load())
	if len(managed.Plugins) != 1 || !managed.Plugins[0].Off {
		t.Fatalf("manager exposes integrated backend: %#v", managed)
	}
	winOneBundles.Lock()
	winOneBundles.entries = nil
	winOneBundles.Unlock()
	if len(Load().Plugins) != 1 {
		t.Fatal("integrated account backend was persisted in third-party registry")
	}
}

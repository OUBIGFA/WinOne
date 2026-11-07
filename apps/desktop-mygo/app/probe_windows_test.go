//go:build windows

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"winone/internal/catalog"
	internalhost "winone/internal/host"
)

func TestProbeOptionsKeepScratchOutsideTheInstallation(t *testing.T) {
	root := t.TempDir()
	installation := filepath.Join(root, "installed")
	scratch := installation + "-probe"
	for _, dir := range []string{installation, scratch, filepath.Join(installation, "nested")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(installation, "WinOne.exe")
	args := func(dir string) []string { return []string{"--check-ui", "nonce", "--probe-root", dir} }
	if got, err := probeOptions(args(scratch), exe); err != nil || got != scratch {
		t.Fatalf("valid scratch: %q, %v", got, err)
	}
	file := filepath.Join(scratch, "not-a-directory")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{installation, filepath.Join(installation, "nested"), filepath.Join(root, "missing"), "relative", file} {
		if _, err := probeOptions(args(dir), exe); err == nil {
			t.Errorf("accepted unsafe scratch %q", dir)
		}
	}
	for _, invalid := range [][]string{nil, {"--check-ui"}, {"--check-ui", "", "--probe-root", scratch}, {"--check-ui", strings.Repeat("n", 129), "--probe-root", scratch}, {"--check-ui", "n\x00", "--probe-root", scratch}, {"--check-ui", "n", "--data-dir", scratch}, {"--check-ui", "n", "--probe-root", scratch, "--open-settings"}} {
		if _, err := probeOptions(invalid, exe); err == nil {
			t.Errorf("accepted arguments %q", invalid)
		}
	}
}

// The old stub reported through fail's modal message box. Installer failures
// must instead return an error promptly, with no normal launch or UI loop.
func TestInvalidProbeArgumentsReturnWithoutAWindow(t *testing.T) {
	if code := Main([]string{"--check-ui"}); code != 1 {
		t.Fatalf("invalid probe exit code = %d", code)
	}
}

func TestNativeProbeHostOnlyReadsTheBuiltinCatalog(t *testing.T) {
	h := newNativeProbeHost()
	for _, method := range []string{"snapshot", "modules.snapshot", "interconnect.pending", "directory-links.pending", "chat.ghosts.pending", "chat.ghosts.pages"} {
		r := h.Handle(internalhost.Request{ID: "read", Method: method})
		if r.Error != nil || r.ID != "read" || !json.Valid(r.Result) {
			t.Fatalf("%s: %+v", method, r)
		}
	}
	if !h.seenSnapshot.Load() || !h.seenModules.Load() || len(h.snapshot.Modules) != len(catalog.Builtins()) {
		t.Fatal("read-only startup did not see the complete builtin catalog")
	}
	for _, method := range []string{"module.enable", "settings.save", "capture.ai.accounts.quota", "chat.ghosts.install", "chat.ghosts.answer", "chat.ghosts.screen", "chat.ghosts.page-session", "chat.ghosts.page-focus", "chat.ghosts.page-gesture", "chat.ghosts.connection-submit"} {
		if r := h.Handle(internalhost.Request{ID: "write", Method: method}); r.Error == nil || r.Error.Code != "probe" {
			t.Fatalf("probe allowed %s: %+v", method, r)
		}
	}
	select {
	case <-h.failure:
	default:
		t.Fatal("unexpected operations were not reported to the native probe")
	}
}

func TestProbeUsesTheProductionNativeWindowOptions(t *testing.T) {
	o := settingsWindowOptions(ui.View(func(c *ui.Context) {}))
	if o.Content == nil || o.Title != "WinOne" || o.Width != 1280 || o.Height != 840 || o.MinWidth != 800 || o.MinHeight != 560 || o.TitleBarStyle != mygo.TitleBarHidden || !o.HideWindowButtons || !o.Hidden {
		t.Fatalf("probe and production frame diverged: %+v", o)
	}
}

// Copied into the pinned plugin package to verify parity with its account page.
package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWinOneAccountSnapshotMatchesCachedAndInvalidatesChangedBundle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	spec := filepath.Join(t.TempDir(), "cline")
	if err := save(List{Plugins: []Entry{{Spec: spec}}}); err != nil {
		t.Fatal(err)
	}
	p := Provider{ID: "cline", Spec: spec, Name: "Cline", Methods: []Method{{Type: "api", Label: "API key"}}}
	if err := WinOneRememberProvider(p, "bundle-v1"); err != nil {
		t.Fatal(err)
	}
	// Suppress background networking in the upstream reference only. Both
	// implementations still read the same real on-disk authentication changes.
	UseCached([]Provider{p})
	defer UseCached(nil)
	for _, auth := range []string{`{"cline":{"type":"api","key":"not-a-real-secret","email":"one@example.invalid"}}`, `{"cline":{"type":"oauth","email":"two@example.invalid"},"cline#second":{"type":"api","key":"another-fixture"}}`, `{}`} {
		if err := os.WriteFile(AuthPath(), []byte(auth), 0600); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		reference := Cached()
		elapsed := time.Since(start)
		current, found, err := WinOneReadProvider(spec, "cline", "bundle-v1")
		if err != nil || !found || len(reference) != 1 || !reflect.DeepEqual(current.Accounts, reference[0].Accounts) || current.SignedIn != reference[0].SignedIn || !reflect.DeepEqual(current.Methods, reference[0].Methods) {
			t.Fatal("snapshot differs from magpie account page", err)
		}
		t.Logf("magpie Cached account metadata: %s", elapsed)
	}
	for _, value := range []struct{ spec, id, fingerprint string }{{spec, "cline", "bundle-v2"}, {spec, "other", "bundle-v1"}, {spec + "-moved", "cline", "bundle-v1"}} {
		if _, found, err := WinOneReadProvider(value.spec, value.id, value.fingerprint); err != nil || found {
			t.Fatal("stale metadata crossed bundle/provider identity", err)
		}
	}
	data, err := os.ReadFile(winOneProviderSnapshotPath())
	if err != nil || strings.Contains(string(data), "not-a-real-secret") || strings.Contains(string(data), "example.invalid") || !json.Valid(data) {
		t.Fatal("discovery snapshot contains account state", err)
	}
	if err = save(List{Plugins: []Entry{{Spec: spec, Options: map[string]any{"site": "changed"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := WinOneReadProvider(spec, "cline", "bundle-v1"); err != nil || found {
		t.Fatal("changed plugin options reused old methods", err)
	}
	if err = save(List{Plugins: []Entry{{Spec: spec, Off: true}}}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := WinOneReadProvider(spec, "cline", "bundle-v1"); err != nil || found {
		t.Fatal("disabled plugin returned as available", err)
	}
}

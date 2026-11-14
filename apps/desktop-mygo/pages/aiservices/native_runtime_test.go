package aiservices

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/testhost"
	"winone/internal/ai"
)

// This test owns the real Magpie process, profile and vendor fixture. Native
// controls issue production HTTP requests, not a copied routing implementation.
func nativeAIWithRuntime(t *testing.T) (*page, *testhost.Harness, *ui.Tester, *ai.GatewayProcess) {
	t.Helper()
	exe := os.Getenv("WINONE_GATEWAY_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated Magpie runtime")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Native routed answer"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(upstream.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	settings := &ai.MagpieSettings{NoStats: true, NoUsageStats: true}
	translator := ai.Translator{Executable: exe, DataDirectory: filepath.Join(t.TempDir(), "runtime")}
	router, err := translator.StartGateway(ctx, settings, []ai.RuntimeProvider{{Provider: ai.Provider{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: upstream.URL, Models: []string{"model"}}, Key: "fixture-key"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.Close)
	f := newFakeAI(t)
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		if method == "capture.ai.gateway.open" {
			return map[string]string{"url": router.AdminURL()}, nil
		}
		// Marketplace icons are stored by the real worker in its own profile;
		// keep the native UI's normal chunked Host read on that same profile.
		if method == "capture.ai.icons.read" {
			var input struct {
				Icon   string
				Offset int
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, err
			}
			return translator.ReadAvatar(input.Icon, input.Offset)
		}
		return f.handle(method, params)
	})
	p := newPage(h.Env)
	t.Cleanup(p.Close)
	// Production pages use the shell's shared scroller. Keep that boundary
	// here so policies added above recording never make the bottom unreachable.
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Size(1100, 1600).Children(func() { p.View(c) })
	}, 1100, 1600)
	settle(h, tt)
	return p, h, tt, router
}

func kitID(v int64) string { b, _ := json.Marshal(v); return string(b) }

func TestNativeAPIRefusesRedirectAndRetainsMutationFailure(t *testing.T) {
	var leaked bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true; w.Write([]byte(`{}`)) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer server.Close()
	_, h, p, tt := setup(t)
	p.gateway.runtimeURL = server.URL + "/session/private/"
	var api nativeAPI
	called := false
	api.request(p, "POST", "/api/caller-keys/add-key", map[string]string{"id": "private"}, func(json.RawMessage) { called = true })
	settle(h, tt)
	if leaked || called || api.err == "" {
		t.Fatalf("redirect or failed mutation treated as success: leaked=%v called=%v err=%s", leaked, called, api.err)
	}
}

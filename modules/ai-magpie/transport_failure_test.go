package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// A provider transport has no retry budget. The outer router must receive a
// refusal promptly so it can choose the next credential or group member.
func TestTransportFailureReturnsToOuterRouter(t *testing.T) {
	oldSettings, oldProviders, oldTransport := settings.WinOneSettings.Load(), provider.WinOneProviders, gateway.WinOneTransportOnly
	t.Cleanup(func() {
		settings.WinOneSettings.Store(oldSettings)
		provider.WinOneProviders, gateway.WinOneTransportOnly = oldProviders, oldTransport
	})
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(t.TempDir(), "usage.jsonl"))
	t.Setenv("WINONE_AI_TRANSPORT_ONLY", "1")
	for _, status := range []int{429, 529} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(status)
				w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
			}))
			defer vendor.Close()
			in := request{Operation: "gateway", Protocol: "chat", BaseURL: vendor.URL, Key: "fixture", Model: "model", ProviderID: "fixture", GatewayPath: "/v1/chat/completions", GatewayBody: json.RawMessage(`{"model":"model","messages":[{"role":"user","content":"hello"}]}`)}
			if err := applyRuntimeSettings(in); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			start := time.Now()
			if err := serveGateway(ctx, in, &output); err != nil {
				t.Fatal(err)
			}
			if time.Since(start) > 3*time.Second || !bytes.Contains(output.Bytes(), []byte(fmt.Sprintf(`"status":%d`, status))) {
				t.Fatalf("transport held refusal: %s", output.String())
			}
		})
	}
}

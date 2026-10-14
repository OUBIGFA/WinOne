package chat

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// The private endpoint must not become an unauthenticated alternate gateway.
func TestPrivateBridgeRequiresItsEphemeralKeyAndRejectsBrowserOrigins(t *testing.T) {
	var calls atomic.Int32
	bridge, err := NewPrivateBridge(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); io.WriteString(w, "ok") }))
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	for _, tc := range []struct {
		key, origin, host string
		status            int
	}{
		{"", "", "", 401}, {"wrong", "", "", 401}, {bridge.Key, "https://example.com", "", 403},
		{bridge.Key, "", "example.com:8080", 403}, {bridge.Key, "", "", 200},
	} {
		req, _ := http.NewRequest("POST", bridge.BaseURL+"/chat/completions", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+tc.key)
		req.Header.Set("Origin", tc.origin)
		if tc.host != "" {
			req.Host = tc.host
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.status {
			t.Fatalf("expected %d, got %d", tc.status, res.StatusCode)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("unauthorized request reached AI", calls.Load())
	}
	address := bridge.BaseURL
	bridge.Close()
	if response, err := http.Get(address + "/models"); err == nil {
		response.Body.Close()
		t.Fatal("closed bridge still accepts requests")
	}
}

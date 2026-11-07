package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yetone/magpie/internal/provider"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNativeVideoTransportPreservesExplicitModelKind(t *testing.T) {
	called := false
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "fixture-video", "status": "completed"})
	}))
	defer vendor.Close()
	in := request{Operation: "gateway", Protocol: "chat", BaseURL: vendor.URL + "/v1", Key: "fixture-key", Model: "video", ProviderID: "fixture", GatewayMethod: "POST", GatewayPath: "/v1/videos", GatewayBody: json.RawMessage(`{"model":"video","prompt":"A sample","seconds":4}`), MagpieSettings: json.RawMessage(`{"noStats":true,"noUsageStats":true,"modelKinds":{"fixture/video":"video"}}`)}
	provider.WinOneProviders = nil
	t.Setenv("WINONE_AI_TRANSPORT_ONLY", "1")
	if err := applyRuntimeSettings(in); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := serveGateway(context.Background(), in, &output); err != nil {
		t.Fatalf("direct native video transport: %v", err)
	}
	decoder := json.NewDecoder(&output)
	var data bytes.Buffer
	status := 0
	for decoder.More() {
		var event struct {
			Type   string
			Status int
			Data   []byte
		}
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "headers" {
			status = event.Status
		}
		if event.Type == "data" {
			data.Write(event.Data)
		}
	}
	if !called || status/100 != 2 || !strings.Contains(data.String(), "fixture-video") {
		t.Fatalf("native result missing, called=%v status=%d body=%s", called, status, data.String())
	}
}

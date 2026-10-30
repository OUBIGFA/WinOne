//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"winone/internal/ai"
	"winone/internal/host"
)

func TestPackagedMediaMCPUsesGatewayPolicyAndKeepsImageFiles(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var slow atomic.Bool
	released := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/chat/completions" {
			io.WriteString(w, `{"choices":[{"message":{"content":"image description"},"finish_reason":"stop"}]}`)
			return
		}
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("unexpected vendor path %s", r.URL.Path)
		}
		if slow.Load() {
			select {
			case <-released:
			case <-r.Context().Done():
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"created": 1, "data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(imageBytes.Bytes())}}})
	}))
	defer vendor.Close()
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	s := newAIService(ctx, root, filepath.Join(root, "WinOne.Host.exe"))
	s.translator.Executable = executable
	s.gatewaySlots = make(chan struct{}, 4)
	s.initGateway()
	defer s.gateway.Close()
	defer s.closeRouting()
	store, err := s.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	config := store.Configuration()
	config.MagpieSettings = &ai.MagpieSettings{NoStats: true, NoUsageStats: true, ImageGen: "fixture/gpt-image-1", Vision: "fixture/gpt-4o"}
	config.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Models: []string{"gpt-image-1", "gpt-4o"}}}
	key := "fixture-key"
	if _, err := store.SaveConfiguration(config, map[string]*string{"fixture": &key}); err != nil {
		t.Fatal(err)
	}
	port, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	number := port.Addr().(*net.TCPAddr).Port
	port.Close()
	state, err := s.gateway.Save(ai.GatewaySettings{Enabled: true, Port: number}, false)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := s.gateway.Key()
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(state.BaseURL, "/v1")
	request := func(method, path string, body any) (int, []byte) {
		t.Helper()
		var input io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			input = bytes.NewReader(raw)
		}
		req, _ := http.NewRequest(method, base+path, input)
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("X-WinOne-Caller-Key", strings.Repeat("a", 64))
		req.Header.Set("X-WinOne-Media-Models", `["fixture/gpt-image-1","fixture/gpt-4o"]`)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, data
	}
	for _, path := range []string{"/mcp/magpie/image", "/mcp/magpie/vision"} {
		status, data := request("POST", path, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
		if status != 200 {
			t.Fatalf("MCP not served: %d %s", status, data)
		}
		want := []string{"analyze_image"}
		if strings.HasSuffix(path, "image") {
			want = []string{"generate_image", "generate_video", "generation_result"}
		}
		for _, name := range want {
			if !bytes.Contains(data, []byte(name)) {
				t.Fatalf("MCP missing %s", name)
			}
		}
	}
	payload := map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "generate_image", "arguments": map[string]string{"prompt": "fixture image"}}}
	status, data := request("POST", "/mcp/magpie/image", payload)
	if status != 200 {
		t.Fatalf("generation %d %s", status, data)
	}
	var result struct {
		Result struct {
			IsError bool
			Content []struct{ Type, Text, Data string }
		}
	}
	if json.Unmarshal(data, &result) != nil || result.Result.IsError {
		t.Fatalf("generation failed: %s", data)
	}
	var link string
	for _, content := range result.Result.Content {
		if content.Type == "text" {
			link = regexp.MustCompile(`http://[^\s]+/mcp/magpie/image/files/[a-zA-Z0-9.-]+`).FindString(content.Text)
		}
	}
	if !strings.HasPrefix(link, base+"/mcp/magpie/image/files/") {
		t.Fatalf("download uses private worker address or missing: %s", data)
	}
	status, data = request("GET", strings.TrimPrefix(link, base), nil)
	if status != 200 || !bytes.Equal(data, imageBytes.Bytes()) {
		t.Fatalf("image lost between calls: %d", status)
	}
	vision := map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "analyze_image", "arguments": map[string]string{"image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes.Bytes())}}}
	_, data = request("POST", "/mcp/magpie/vision", vision)
	if !bytes.Contains(data, []byte("image description")) {
		t.Fatalf("vision MCP did not reach configured model: %s", data)
	}
	// The first call ends after upstream's 20-second window; its job must
	// remain available to a later generation_result call in the same service.
	slow.Store(true)
	_, data = request("POST", "/mcp/magpie/image", payload)
	job := regexp.MustCompile(`img_[a-f0-9]+`).Find(data)
	if len(job) == 0 {
		t.Fatalf("slow generation did not return its job ID: %s", data)
	}
	release()
	poll := map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": "generation_result", "arguments": map[string]string{"id": string(job)}}}
	_, data = request("POST", "/mcp/magpie/image", poll)
	if !bytes.Contains(data, []byte(base64.StdEncoding.EncodeToString(imageBytes.Bytes()))) {
		t.Fatalf("generation result lost completed image: %s", data)
	}
	previous := calls.Load()
	allowed := s.gateway.Status().GatewaySettings
	allowed.Models = []ai.Selection{{Provider: "fixture", Model: "gpt-4o"}}
	if _, err := s.gateway.Save(allowed, false); err != nil {
		t.Fatal(err)
	}
	_, data = request("POST", "/mcp/magpie/image", payload)
	if json.Unmarshal(data, &result) != nil || !result.Result.IsError || calls.Load() != previous {
		t.Fatalf("MCP bypassed model policy: %s", data)
	}
	allowed = s.gateway.Status().GatewaySettings
	allowed.Models = []ai.Selection{{Provider: "fixture", Model: "gpt-image-1"}}
	if _, err := s.gateway.Save(allowed, false); err != nil {
		t.Fatal(err)
	}
	_, data = request("POST", "/mcp/magpie/vision", vision)
	if !bytes.Contains(data, []byte(`"isError":true`)) || calls.Load() != previous {
		t.Fatalf("vision cache bypassed model policy: %s", data)
	}
	allowed = s.gateway.Status().GatewaySettings
	if _, err := s.gateway.Save(allowed, true); err != nil {
		t.Fatal(err)
	}
	secret, _ = s.gateway.Key()
	status, _ = request("GET", strings.TrimPrefix(link, base), nil)
	if status == 200 {
		t.Fatal("a different gateway key read the old image")
	}
	catalog := s.handle(host.Request{ID: "catalog", Method: "magpie.catalog", Params: json.RawMessage(`{}`)})
	var metadata struct{ Gateway string }
	if catalog.Error != nil || json.Unmarshal(catalog.Result, &metadata) != nil || metadata.Gateway != base {
		t.Fatal("MCP catalog did not advertise the Host listener", catalog.Error)
	}
	disabled := s.gateway.Status().GatewaySettings
	disabled.Enabled = false
	raw, _ := json.Marshal(map[string]any{"settings": disabled})
	stopped := s.handle(host.Request{ID: "disable", Method: "gateway.save", Params: raw})
	if stopped.Error != nil || s.routing.process != nil {
		t.Fatal("disabling the gateway retained its media worker", stopped.Error)
	}
}

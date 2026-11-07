//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"winone/internal/ai"
)

// Host operations use the real packaged Magpie gateway, catalog and keys.
// The deterministic provider is an HTTP fixture, not simulated media receipts.
func TestGhostMediaModelsEditVideoAndEmbeddingsUseTheRealGateway(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("packaged AI runtime required")
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	video := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 1, 'i', 's', 'o', 'm', 'm', 'p', '4', '2'}
	var edited bool
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var input map[string]any
		_ = json.NewDecoder(r.Body).Decode(&input)
		switch r.URL.Path {
		case "/v1/models":
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"id": "image", "kind": "image"}, map[string]string{"id": "video", "kind": "video"}, map[string]string{"id": "embedding", "kind": "embedding"}}})
		case "/v1/images/edits":
			edited = true
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(pngBytes.Bytes()), "mime_type": "image/png"}}})
		case "/v1/videos":
			json.NewEncoder(w).Encode(map[string]string{"id": "fixture-video", "status": "completed", "model": "video"})
		case "/v1/videos/fixture-video":
			json.NewEncoder(w).Encode(map[string]string{"id": "fixture-video", "status": "completed", "model": "video"})
		case "/v1/videos/fixture-video/content":
			w.Header().Set("Content-Type", "video/mp4")
			w.Write(video)
		case "/v1/embeddings":
			if input["dimensions"] != float64(2) {
				t.Errorf("dimensions were lost: %#v", input)
			}
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"index": 1, "embedding": []float64{3, 4}}, map[string]any{"index": 0, "embedding": []float64{1, 2}}}})
		default:
			t.Errorf("unexpected media endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer vendor.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := t.TempDir()
	service := newAIService(ctx, root, filepath.Join(root, "WinOne.Host.exe"))
	service.translator.Executable = executable
	defer service.closeRouting()
	store, err := service.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	config := store.Configuration()
	config.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Models: []string{"gpt-image-1", "video", "embedding"}}}
	config.MagpieSettings = &ai.MagpieSettings{NoStats: true, NoUsageStats: true, ImageGen: "fixture/gpt-image-1", ModelKinds: map[string]string{"fixture/video": "video"}}
	key := "fixture-media-key"
	if _, err = store.SaveConfiguration(config, map[string]*string{"fixture": &key}); err != nil {
		t.Fatal(err)
	}
	g := newChatGhosts(&chatControl{ctx: ctx, ai: service, directory: root})
	catalog, err := g.mediaCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	listed, _ := json.Marshal(catalog)
	if !strings.Contains(string(listed), "fixture/gpt-image-1") || !strings.Contains(string(listed), "fixture/video") {
		t.Fatalf("wrong actual media catalog: %s", listed)
	}
	reference := filepath.Join(root, "reference.png")
	if err = os.WriteFile(reference, pngBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"prompt": "Edit", "model": "fixture/gpt-image-1", "imagePaths": []string{reference}})
	result, err := g.pluginMedia(ctx, "image-generate", raw)
	if err != nil {
		t.Fatal(err)
	}
	info := result.(map[string]string)
	data, err := os.ReadFile(info["path"])
	if err != nil || !bytes.Equal(data, pngBytes.Bytes()) || !edited {
		t.Fatalf("real image edit: %v %v", edited, err)
	}
	// Confirm the inner transport independently of the outer router's helper
	// projection, using the exact same private settings and method envelope.
	transport := service.translator.ForProvider("fixture")
	transport.TransportOnly = true
	probe := httptest.NewRecorder()
	preferences := ai.Preferences{ProviderID: "fixture", Protocol: "chat", BaseURL: vendor.URL + "/v1", Model: "video", MagpieSettings: config.MagpieSettings}
	partial, probeErr := transport.ChatRequest(ctx, preferences, key, "/v1/videos", json.RawMessage(`{"model":"video","prompt":"Video","seconds":4}`), http.Header{}, probe)
	if probeErr != nil {
		t.Fatalf("inner video transport partial=%v status=%d error=%v body=%s", partial, probe.Code, probeErr, probe.Body.String())
	}
	raw, _ = json.Marshal(map[string]any{"prompt": "Video", "model": "fixture/video", "ratio": "16:9", "duration": 4})
	result, err = g.pluginMedia(ctx, "video-generate", raw)
	if err != nil {
		t.Fatal(err)
	}
	info = result.(map[string]string)
	data, err = os.ReadFile(info["path"])
	if err != nil || !bytes.Equal(data, video) {
		t.Fatalf("real video content did not survive: %v", err)
	}
	raw, _ = json.Marshal(map[string]any{"model": "fixture/embedding", "texts": []string{"one", "two"}, "dimensions": 2, "inputType": "query"})
	result, err = g.pluginMedia(ctx, "embed-text", raw)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(result)
	if !bytes.Contains(data, []byte(`[[1,2],[3,4]]`)) {
		t.Fatalf("unordered or missing actual embeddings: %s", data)
	}
}

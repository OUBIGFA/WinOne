//go:build windows

package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuiltInTranslationNetwork(t *testing.T) {
	if os.Getenv("WINONE_TRANSLATION_NETWORK_TEST") != "1" {
		t.Skip("explicit network test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := translateGoogle(ctx, &http.Client{Timeout: 20 * time.Second}, googleEndpoint, "Hello world", "zh-CN")
	if err != nil || result == "" || result == "Hello world" {
		t.Fatalf("built-in translation: %q %v", result, err)
	}
	t.Logf("Built-in translation returned %q", result)
}

func TestPackagedMagpieProcess(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	for _, protocol := range []string{"chat", "responses", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			paths := map[string]string{"chat": "/v1/chat/completions", "responses": "/v1/responses", "anthropic": "/v1/messages"}
			bodies := map[string]string{
				"chat":      `{"id":"fixture","object":"chat.completion","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"测试译文"}}]}`,
				"responses": `{"id":"resp_fixture","status":"completed","model":"fixture-model","output":[{"type":"message","id":"msg_fixture","role":"assistant","status":"completed","content":[{"type":"output_text","text":"测试译文"}]}]}`,
				"anthropic": `{"id":"msg_fixture","type":"message","role":"assistant","content":[{"type":"text","text":"测试译文"}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":2}}`,
			}
			const encodedImage = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1cAAAAASUVORK5CYII="
			var expectImage atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != paths[protocol] {
					t.Errorf("unexpected API path %s", r.URL.Path)
				}
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if expectImage.Load() {
					encoded, _ := json.Marshal(request)
					if !strings.Contains(string(encoded), encodedImage) {
						t.Error("protocol adaptation dropped the image")
					}
				}
				if request["stream"] == true && protocol != "chat" {
					w.Header().Set("Content-Type", "text/event-stream")
					if protocol == "responses" {
						fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"测试译文\"}\n\n")
						fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":%s}\n\n", bodies[protocol])
					} else {
						fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_fixture\",\"role\":\"assistant\",\"model\":\"fixture-model\",\"content\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":0}}}\n\n")
						fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"测试译文\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
					}
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(bodies[protocol]))
				}
			}))
			defer upstream.Close()
			p := DefaultPreferences()
			p.Engine = "llm"
			p.Protocol = protocol
			p.BaseURL = upstream.URL + "/v1"
			p.Model = "fixture-model"
			result, err := (Translator{Executable: executable, DataDirectory: t.TempDir()}).Translate(context.Background(), p, "test-key", "Only test data")
			if err != nil || result != "测试译文" {
				t.Fatalf("magpie process result %q: %v", result, err)
			}
			expectImage.Store(true)
			image, _ := base64.StdEncoding.DecodeString(encodedImage)
			result, err = (Translator{Executable: executable, DataDirectory: t.TempDir()}).Generate(context.Background(), p, "test-key", Generation{Text: "Read image", ImagePNG: image})
			if err != nil || result != "测试译文" {
				t.Fatalf("magpie image result %q: %v", result, err)
			}
		})
	}
}

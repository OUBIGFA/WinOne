//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPackagedStreamDeliversBeforeLastChunkAndCancels(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("explicit packaged magpie test")
	}
	for _, stop := range []bool{false, true} {
		t.Run(fmt.Sprint(stop), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			seen := make(chan struct{})
			disconnected := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				_ = json.NewDecoder(r.Body).Decode(&request)
				if request["stream"] != true {
					t.Error("stream not requested")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"思考内容\",\"content\":\"首段\"}}]}\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-seen:
				case <-r.Context().Done():
					close(disconnected)
					return
				case <-ctx.Done():
					return
				}
				if stop {
					<-r.Context().Done()
					close(disconnected)
					return
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"后段\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			p := DefaultPreferences()
			p.Engine = "llm"
			p.BaseURL = server.URL + "/v1"
			p.Model = "stream-test"
			var text, reasoning strings.Builder
			result, err := (Translator{Executable: exe, DataDirectory: t.TempDir()}).GenerateStream(ctx, p, "fixture-key", Generation{Text: "Fixture"}, func(d Delta) error {
				text.WriteString(d.Text)
				reasoning.WriteString(d.Reasoning)
				if d.Text == "首段" {
					close(seen)
					if stop {
						cancel()
					}
				}
				return nil
			})
			if reasoning.String() != "思考内容" {
				t.Fatal("reasoning lost")
			}
			if stop {
				if err != context.Canceled {
					t.Fatalf("cancel returned %v", err)
				}
				select {
				case <-disconnected:
				case <-time.After(time.Second):
					t.Fatal("vendor request survived cancellation")
				}
			} else if err != nil || result != "首段后段" || text.String() != result {
				t.Fatalf("stream %q: %v", result, err)
			}
		})
	}
}

func TestPackagedStreamNormalizesResponsesAndAnthropicReasoning(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("explicit packaged magpie test")
	}
	for _, protocol := range []string{"responses", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			seen := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				send := func(kind string, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data) }
				if protocol == "responses" {
					send("response.reasoning_summary_text.delta", `{"type":"response.reasoning_summary_text.delta","output_index":0,"summary_index":0,"delta":"思考内容"}`)
					send("response.output_text.delta", `{"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"首段"}`)
				} else {
					send("message_start", `{"type":"message_start","message":{"id":"fixture","role":"assistant","model":"test","content":[],"usage":{"input_tokens":3,"output_tokens":0}}}`)
					send("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`)
					send("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"思考内容"}}`)
					send("content_block_stop", `{"type":"content_block_stop","index":0}`)
					send("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`)
					send("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"首段"}}`)
				}
				w.(http.Flusher).Flush()
				select {
				case <-seen:
				case <-r.Context().Done():
					return
				case <-ctx.Done():
					return
				}
				if protocol == "responses" {
					send("response.output_text.delta", `{"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"后段"}`)
					send("response.completed", `{"type":"response.completed","response":{"id":"fixture","status":"completed","output":[]}}`)
				} else {
					send("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"后段"}}`)
					send("content_block_stop", `{"type":"content_block_stop","index":1}`)
					send("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`)
					send("message_stop", `{"type":"message_stop"}`)
				}
			}))
			defer server.Close()
			p := DefaultPreferences()
			p.Protocol = protocol
			p.BaseURL = server.URL + "/v1"
			p.Model = "test"
			var reasoning strings.Builder
			result, err := (Translator{Executable: exe, DataDirectory: t.TempDir()}).GenerateStream(ctx, p, "fixture", Generation{Text: "fixture"}, func(d Delta) error {
				reasoning.WriteString(d.Reasoning)
				if d.Text == "首段" {
					close(seen)
				}
				return nil
			})
			if err != nil || result != "首段后段" || reasoning.String() != "思考内容" {
				t.Fatalf("%s stream text=%q reasoning=%q error=%v", protocol, result, reasoning.String(), err)
			}
		})
	}
}

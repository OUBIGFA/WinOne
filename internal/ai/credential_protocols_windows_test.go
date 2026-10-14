//go:build windows

package ai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPackagedKeyFailoverAcrossAPIProtocols(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("explicit packaged magpie test")
	}
	for _, protocol := range []string{"responses", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			var mu sync.Mutex
			var keys []string
			vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				key := r.Header.Get("x-api-key")
				if key == "" {
					key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				}
				mu.Lock()
				keys = append(keys, key)
				mu.Unlock()
				if key == "first" {
					w.WriteHeader(401)
					fmt.Fprint(w, `{"error":{"message":"fixture rejected"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				send := func(kind, data string) { fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data) }
				if protocol == "responses" {
					send("response.output_text.delta", `{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"成功"}`)
					send("response.completed", `{"type":"response.completed","response":{"id":"fixture","status":"completed","output":[]}}`)
				} else {
					send("message_start", `{"type":"message_start","message":{"id":"fixture","role":"assistant","model":"m","content":[],"usage":{"input_tokens":3,"output_tokens":0}}}`)
					send("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
					send("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"成功"}}`)
					send("content_block_stop", `{"type":"content_block_stop","index":0}`)
					send("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`)
					send("message_stop", `{"type":"message_stop"}`)
				}
			}))
			defer vendor.Close()
			store, err := Open(filepath.Join(t.TempDir(), "ai.json"))
			if err != nil {
				t.Fatal(err)
			}
			c := store.Configuration()
			c.Providers = []Provider{{ID: "one", Name: "One", AuthMode: "api-key", Protocol: protocol, BaseURL: vendor.URL + "/v1", Models: []string{"m"}, Keys: []KeySlot{{ID: "a", Enabled: true}, {ID: "b", Enabled: true}}}}
			c.Tasks["chat"] = Selection{Provider: "one", Model: "m"}
			first, second := "first", "second"
			if _, err = store.SaveConfigurationWithKeys(c, nil, CredentialUpdates{"one": {"a": &first, "b": &second}}); err != nil {
				t.Fatal(err)
			}
			p, plan, err := store.ForTaskKeys("chat", nil)
			if err != nil {
				t.Fatal(err)
			}
			runtime := Translator{Executable: exe, DataDirectory: t.TempDir()}
			var result string
			err = store.WithKeys(ctx, plan, func(ctx context.Context, key string) (bool, error) {
				partial := false
				var err error
				result, err = runtime.GenerateStream(ctx, p, key, Generation{Text: "fixture"}, func(Delta) error { partial = true; return nil })
				return partial, err
			})
			if err != nil || result != "成功" {
				t.Fatalf("result=%q error=%v", result, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if !reflect.DeepEqual(keys, []string{"first", "second"}) {
				t.Fatalf("credential sequence=%v", keys)
			}
		})
	}
}

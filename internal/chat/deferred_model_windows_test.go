//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"winone/internal/ai"
)

func TestDeferredModelPreservesActiveTurnAndRunsQueuedInputOnTheAcceptedRoute(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit bundled Pi runtime")
	}
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	row, err := store.NewConversation("", "", "Deferred model", ai.Selection{Provider: "fixture", Model: "old"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	var mu sync.Mutex
	models := []string{}
	release := make(chan struct{})
	var once sync.Once
	old := map[string]any{"id": "fixture/old", "name": "Old", "provider": "fixture"}
	next := map[string]any{"id": "fixture/new", "name": "New", "provider": "fixture"}
	available := []map[string]any{old, next}
	service := NewService(ctx, store, ServiceOptions{Bun: bun, Worker: worker, Configure: func(value Conversation) (map[string]any, func(), error) {
		bridge, err := NewPrivateBridge(ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request struct{ Model string }
			_ = json.NewDecoder(r.Body).Decode(&request)
			mu.Lock()
			models = append(models, request.Model)
			first := len(models) == 1
			mu.Unlock()
			if first {
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Done\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}))
		if err != nil {
			return nil, nil, err
		}
		model := old
		if value.Model.Model == "new" {
			model = next
		}
		return map[string]any{"baseUrl": bridge.BaseURL, "key": bridge.Key, "agentDir": filepath.Join(root, "agent"), "model": model, "models": available, "runtimeSettings": map[string]any{"codeMode": false, "autoRetry": false, "subagents": false}}, bridge.Close, nil
	}})
	defer func() {
		once.Do(func() { close(release) })
		end, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = service.Stop(end)
	}()
	if _, err = service.Open(ctx, row.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Prompt(ctx, row.ID, "old-input", "Original input"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(models)
		mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	changed, err := service.DeferModelConfiguration(ctx, row.ID, map[string]any{"model": next, "models": available, "thinkingLevel": "", "fastMode": false}, func(value *Conversation) error { value.Model.Model = "new"; return nil })
	if err != nil || !changed.ModelDeferred {
		t.Fatal(changed, err)
	}
	if _, err = service.Prompt(ctx, row.ID, "new-input", "Queued input"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(models) != 1 || models[0] != "fixture/old" {
		t.Fatalf("active model changed: %v", models)
	}
	mu.Unlock()
	once.Do(func() { close(release) })
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(models)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	got := append([]string{}, models...)
	mu.Unlock()
	if len(got) != 2 || got[0] != "fixture/old" || got[1] != "fixture/new" {
		t.Fatalf("deferred routes: %v", got)
	}
	changed, err = store.Conversation(row.ID)
	if err != nil || changed.ModelDeferred {
		t.Fatalf("deferred state not settled: %+v %v", changed, err)
	}
}

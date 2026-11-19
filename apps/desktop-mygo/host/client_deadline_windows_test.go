package host

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	internalhost "winone/internal/host"
	"winone/internal/transport"
)

type completionHandler func(internalhost.Request) internalhost.Response

func (f completionHandler) Handle(r internalhost.Request) internalhost.Response { return f(r) }

// The real local pipe must keep the synchronous account operation alive
// past the old 15-second first-response cap, and submit it only once.
func TestLongAccountCompletionsKeepTheirSingleResponse(t *testing.T) {
	for index, method := range []string{"capture.ai.accounts.move-agy", "capture.ai.accounts.warm-now", "capture.ai.accounts.warm-all"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			pipe := fmt.Sprintf(`\\.\pipe\WinOne-completion-%d-%d`, time.Now().UnixNano(), index)
			listener, err := transport.Listen(pipe)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			defer listener.Close()
			var calls atomic.Int32
			go transport.Serve(ctx, listener, completionHandler(func(r internalhost.Request) internalhost.Response {
				calls.Add(1)
				select {
				case <-time.After(16 * time.Second):
				case <-ctx.Done():
				}
				return internalhost.Response{ID: r.ID, Result: json.RawMessage(`{"completed":true}`)}
			}))
			client, err := NewClient(pipe)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Cancel()
			defer func() {
				if client.server != nil {
					windows.CloseHandle(client.server.handle)
				}
			}()
			raw, err := client.CallCompleting(method, map[string]string{"provider": "private", "agent": "codex"})
			if err != nil || string(raw) != `{"completed":true}` || calls.Load() != 1 {
				t.Fatalf("long completion lost or repeated: %s %v calls=%d", raw, err, calls.Load())
			}
		})
	}
}

// Warm-all has no made-up whole-operation deadline. Closing its window's
// client must still cancel a connected request promptly.
func TestWarmAllCompletionStopsWhenItsClientIsCancelled(t *testing.T) {
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-warm-cancel-%d`, time.Now().UnixNano())
	listener, err := transport.Listen(pipe)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	defer listener.Close()
	entered := make(chan struct{})
	go transport.Serve(ctx, listener, completionHandler(func(r internalhost.Request) internalhost.Response {
		close(entered)
		<-ctx.Done()
		return internalhost.Response{ID: r.ID, Result: json.RawMessage(`{}`)}
	}))
	client, err := NewClient(pipe)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cancel()
	defer func() {
		if client.server != nil {
			windows.CloseHandle(client.server.handle)
		}
	}()
	finished := make(chan error, 1)
	go func() { _, err := client.CallCompleting("capture.ai.accounts.warm-all", nil); finished <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("warm-all was not admitted")
	}
	client.Cancel()
	select {
	case err := <-finished:
		if Code(err) != "cancelled" {
			t.Fatal("window cancellation was lost", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled warm-all kept its pipe open")
	}
}

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

// Real local pipes must retain the backend's response after the old 15-second
// UI cap. Import also exceeds the old 30-second total, without resubmission.
func TestSlowAICompletionsKeepTheirSingleResponse(t *testing.T) {
	for index, test := range []struct {
		method string
		delay  time.Duration
		failed bool
	}{
		{"capture.ai.models.fetch", 16 * time.Second, false},
		{"capture.ai.models.certificate", 16 * time.Second, true},
		{"capture.ai.proxies.test", 16 * time.Second, false},
		{"capture.ai.icons.store", 16 * time.Second, false},
		{"capture.ai.icons.fetch", 16 * time.Second, true},
		{"capture.ai.icons.website", 16 * time.Second, false},
		{"capture.ai.plans.get", 16 * time.Second, false},
		{"capture.ai.plans.signout", 16 * time.Second, false},
		{"capture.ai.accounts.export", 16 * time.Second, false},
		{"capture.ai.accounts.import", 31 * time.Second, false},
		{"capture.ai.accounts.move-agy", 16 * time.Second, false},
		{"capture.ai.accounts.warm-now", 16 * time.Second, false},
		{"capture.ai.accounts.warm-all", 16 * time.Second, false},
	} {
		t.Run(test.method, func(t *testing.T) {
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
				case <-time.After(test.delay):
				case <-ctx.Done():
				}
				if test.failed {
					return internalhost.Response{ID: r.ID, Error: &internalhost.RPCError{Code: "ai", Message: "fixture operation failed"}}
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
			raw, err := client.CallCompleting(test.method, map[string]string{"provider": "private", "agent": "codex"})
			if test.failed {
				if Code(err) != "ai" || err.Error() != "fixture operation failed (ai)" {
					t.Errorf("lost backend failure: %v", err)
				}
			} else if err != nil || string(raw) != `{"completed":true}` {
				t.Fatalf("long completion lost or repeated: %s %v calls=%d", raw, err, calls.Load())
			}
			if calls.Load() != 1 {
				t.Errorf("operation submitted %d times; want one", calls.Load())
			}
		})
	}
}

func TestClientDeadlineIsReportedAsTimeout(t *testing.T) {
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-client-timeout-%d`, time.Now().UnixNano())
	listener, err := transport.Listen(pipe)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go transport.Serve(ctx, listener, completionHandler(func(r internalhost.Request) internalhost.Response {
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
	_, err = client.request("deadline", "snapshot", nil, time.Now().Add(200*time.Millisecond))
	if Code(err) != "timeout" {
		t.Fatalf("deadline shown as a connection failure: %v", err)
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

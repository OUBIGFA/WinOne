//go:build windows

package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/protocol"
)

// Completed work must reach the caller even when it outlasts the old
// connection-wide deadline. Use real pipes and keep the caller's budget intact.
func TestPipeReturnsSlowHandlerResponses(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		delay  time.Duration
		failed bool
	}{
		{"ordinary", "snapshot", 6 * time.Second, false},
		{"model-test", "capture.ai.models.test", 16 * time.Second, false},
		{"account-error", "capture.ai.accounts.list", 16 * time.Second, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			name := fmt.Sprintf(`\\.\pipe\WinOne-slow-%s-%d`, test.name, time.Now().UnixNano())
			listener, err := Listen(name)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			var calls atomic.Int32
			served := make(chan error, 1)
			go func() {
				served <- Serve(ctx, listener, pipeTestHandler(func(request host.Request) host.Response {
					if request.Method == test.method {
						calls.Add(1)
						time.Sleep(test.delay)
						if test.failed {
							return host.Response{ID: request.ID, Error: &host.RPCError{Code: "ai", Message: "fixture account unavailable"}}
						}
					}
					return host.Response{ID: request.ID, Result: json.RawMessage(`{"done":true}`)}
				}))
			}()
			t.Cleanup(func() {
				cancel()
				if err := <-served; err != nil {
					t.Error(err)
				}
			})
			response, err := CallWithTimeout(ctx, name, host.Request{Version: 1, ID: "slow", Method: test.method}, 30*time.Second)
			if err != nil {
				t.Errorf("completed handler response was lost: %v", err)
			} else if test.failed {
				if response.Error == nil || response.Error.Code != "ai" || response.Error.Message != "fixture account unavailable" {
					t.Errorf("operation error was lost: %+v", response)
				}
			} else if response.Error != nil || string(response.Result) != `{"done":true}` {
				t.Errorf("operation result was lost: %+v", response)
			}
			// EOF from the old implementation did not mean the Host had exited.
			if response, err = Call(ctx, name, host.Request{Version: 1, ID: "next", Method: "health"}); err != nil || string(response.Result) != `{"done":true}` {
				t.Errorf("Host did not answer the next request: %+v, %v", response, err)
			}
			if calls.Load() != 1 {
				t.Errorf("slow operation ran %d times; want exactly once", calls.Load())
			}
		})
	}
}

// Splitting read/write budgets must retain both protections: an idle sender
// cannot enter the handler, and an unread reply cannot keep a connection alive.
func TestPipeBoundsIdleSendersAndUnreadReplies(t *testing.T) {
	for _, sendRequest := range []bool{false, true} {
		t.Run(fmt.Sprintf("request-sent-%t", sendRequest), func(t *testing.T) {
			t.Parallel()
			server, client := net.Pipe()
			defer client.Close()
			defer server.Close()
			var calls atomic.Int32
			done := make(chan struct{})
			go func() {
				defer close(done)
				serveOne(server, pipeTestHandler(func(request host.Request) host.Response {
					calls.Add(1)
					return host.Response{ID: request.ID, Result: json.RawMessage(`{"done":true}`)}
				}))
			}()
			if sendRequest {
				if err := client.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
					t.Fatal(err)
				}
				if err := protocol.WriteFrame(client, []byte(`{"version":1,"id":"unread","method":"snapshot"}`)); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("inactive peer kept the connection alive")
			}
			wantCalls := int32(0)
			if sendRequest {
				wantCalls = 1
			}
			if calls.Load() != wantCalls {
				t.Errorf("handler ran %d times; want %d", calls.Load(), wantCalls)
			}
			if _, err := protocol.ReadFrame(client); !errors.Is(err, io.EOF) {
				t.Errorf("inactive connection did not close: %v", err)
			}
		})
	}
}

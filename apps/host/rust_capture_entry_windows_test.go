//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/residency"
	"winone/internal/settings"
	"winone/internal/transport"
)

// The real entry executable waits for a controlled capture job. No screen is
// acquired here; native canvas recovery is covered by the capture probes.
func TestRustCaptureEntryDoesNotCreateSettingsWindow(t *testing.T) {
	bundle := os.Getenv("WINONE_EXIT_BUNDLE")
	if bundle == "" {
		t.Skip("explicit packaged Rust capture entry test")
	}
	for _, state := range []string{"completed", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			directory := t.TempDir()
			store, err := settings.Open(filepath.Join(directory, "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			service := host.New(store)
			service.SetLocaleResolver(func(settings.Settings) string { return "zh-CN" })
			pipe := fmt.Sprintf(`\\.\pipe\WinOne-capture-entry-%d`, time.Now().UnixNano())
			listener, err := transport.Listen(pipe)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			entered := make(chan struct{}, 1)
			var released atomic.Bool
			var starts atomic.Int32
			service.RegisterModule("capture", func() host.Module { return host.Module{ID: "capture", State: "ready"} }, func(request host.Request) host.Response {
				var result any
				switch request.Method {
				case "draft.resume":
					var params struct {
						ExpectedID string `json:"expectedId"`
						Language   string `json:"language"`
					}
					if json.Unmarshal(request.Params, &params) != nil || params.ExpectedID != strings.Repeat("a", 32) || params.Language != "zh-CN" {
						return host.Response{ID: request.ID, Error: &host.RPCError{Code: "invalid", Message: "unexpected draft request"}}
					}
					starts.Add(1)
					select {
					case entered <- struct{}{}:
					default:
					}
					result = map[string]any{"id": "capture-job", "state": "running"}
				case "jobs.get":
					current := "running"
					if released.Load() {
						current = state
					}
					result = map[string]any{"id": "capture-job", "state": current, "result": map[string]bool{"copied": true}}
				default:
					return host.Response{ID: request.ID, Error: &host.RPCError{Code: "invalid", Message: "unexpected method"}}
				}
				data, _ := json.Marshal(result)
				return host.Response{ID: request.ID, Result: data}
			})
			done := make(chan error, 1)
			go func() { done <- transport.Serve(ctx, listener, service) }()
			defer func() { cancel(); listener.Close(); <-done }()
			command := exec.CommandContext(ctx, filepath.Join(bundle, "WinOne.exe"), "--pipe", pipe, "--data-dir", directory, "--standard-session", "--capture-draft", strings.Repeat("a", 32))
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if command.ProcessState == nil {
					command.Process.Kill()
					command.Wait()
				}
			}()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("capture entry did not reach the worker")
			}
			if open, err := residency.WindowOpen(pipe); err != nil || open {
				t.Fatal("capture entry created a settings window", open, err)
			}
			released.Store(true)
			if err := command.Wait(); err != nil {
				t.Fatal("capture completion did not exit normally", err)
			}
			if starts.Load() != 1 {
				t.Fatal("capture was resubmitted")
			}
		})
	}
}

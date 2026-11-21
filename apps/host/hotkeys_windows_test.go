//go:build windows

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/hotkeys"
	"winone/internal/settings"
)

func TestShortcutCaptureReportsDelayedFailure(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	service.RegisterModule("capture", func() host.Module { return host.Module{} }, func(req host.Request) host.Response {
		if req.Method != "jobs.get" {
			t.Fatalf("unexpected %s", req.Method)
		}
		return host.Response{ID: req.ID, Result: json.RawMessage(`{"id":"job","state":"failed","error":"复制失败"}`)}
	})
	_, err = waitCaptureJob(context.Background(), service, host.Response{Result: json.RawMessage(`{"id":"job","state":"running"}`)}, nil)
	if err == nil || !strings.Contains(err.Error(), "复制失败") {
		t.Fatalf("late failure lost: %v", err)
	}
}

func TestShortcutHandsOffOnlyAValidCompletedDraft(t *testing.T) {
	id := strings.Repeat("a", 32)
	opened := ""
	launch := func(value string) error { opened = value; return nil }
	response := host.Response{Result: json.RawMessage(`{"id":"job","state":"completed","result":{"draftId":"` + id + `"}}`)}
	if _, err := waitCaptureJob(context.Background(), nil, response, launch); err != nil || opened != id {
		t.Fatal(opened, err)
	}
	opened = ""
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := waitCaptureJob(cancelled, nil, response, launch); err == nil || opened != "" {
		t.Fatal("closing host launched a new editor")
	}
	if _, err := waitCaptureJob(context.Background(), nil, response, func(string) error { return context.DeadlineExceeded }); err != context.DeadlineExceeded {
		t.Fatal("window launch error was hidden", err)
	}
	opened = ""
	for _, payload := range []string{`{"id":"job","state":"cancelled"}`, `{"id":"job","state":"completed","result":{"id":"ordinary-image"}}`} {
		if _, err := waitCaptureJob(context.Background(), nil, host.Response{Result: json.RawMessage(payload)}, launch); err != nil || opened != "" {
			t.Fatal(opened, err)
		}
	}
	for _, payload := range []string{`{"id":"job","state":"completed","result":{"draftId":"../bad"}}`, `{"id":"job","state":"unknown"}`} {
		if _, err := waitCaptureJob(context.Background(), nil, host.Response{Result: json.RawMessage(payload)}, launch); err == nil {
			t.Fatal("invalid completion was accepted")
		}
	}
}

func TestShortcutResumesAnExistingDraftWithoutStartingCapture(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	id := strings.Repeat("b", 32)
	service.RegisterModule("capture", func() host.Module { return host.Module{} }, func(request host.Request) host.Response {
		if request.Method != "draft.get" {
			t.Fatalf("unexpected capture command: %s", request.Method)
		}
		return host.Response{ID: request.ID, Result: json.RawMessage(`{"draft":{"id":"` + id + `"}}`)}
	})
	opened := ""
	resumed, err := resumeCaptureDraft(context.Background(), service, func(value string) error { opened = value; return nil })
	if err != nil || !resumed || opened != id {
		t.Fatal(resumed, opened, err)
	}
}

// A finished capture must be observed when it ends, not on the next 250 ms
// poll. The worker holds jobs.get until the job changes (bounded by waitMs).
func TestShortcutCaptureObservesCompletionWithoutPollingDelay(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	calls := 0
	service.RegisterModule("capture", func() host.Module { return host.Module{} }, func(req host.Request) host.Response {
		var params struct {
			ID     string `json:"id"`
			WaitMs int    `json:"waitMs"`
		}
		if req.Method != "jobs.get" || json.Unmarshal(req.Params, &params) != nil || params.ID != "job" || params.WaitMs <= 0 {
			t.Fatalf("unexpected %s %s", req.Method, req.Params)
		}
		calls++
		if calls == 1 {
			return host.Response{ID: req.ID, Result: json.RawMessage(`{"id":"job","state":"running"}`)}
		}
		return host.Response{ID: req.ID, Result: json.RawMessage(`{"id":"job","state":"completed","result":{},"timings":{"shown":42.5,"closed":900}}`)}
	})
	started := time.Now()
	timings, err := waitCaptureJob(context.Background(), service, host.Response{Result: json.RawMessage(`{"id":"job","state":"running"}`)}, nil)
	if err != nil || calls != 2 || timings["shown"] != 42.5 || timings["closed"] != 900 {
		t.Fatal(calls, timings, err)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("completion observed after %v", elapsed)
	}
}

type fakeShortcutRegistrar struct{ events chan string }

func (f *fakeShortcutRegistrar) Update([]hotkeys.Binding) ([]hotkeys.Status, error) { return nil, nil }
func (f *fakeShortcutRegistrar) Events() <-chan string                              { return f.events }
func (f *fakeShortcutRegistrar) Close() error                                      { return nil }

// Presses during a capture that has no visible session to focus (silent OCR,
// closing) used to be dropped. The latest one now runs as soon as it ends.
func TestShortcutRunsTheLatestPressQueuedWhileBusy(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	release := make(chan struct{})
	started := make(chan string, 4)
	activations := make(chan struct{}, 4)
	service.RegisterModule("capture", func() host.Module { return host.Module{} }, func(req host.Request) host.Response {
		switch req.Method {
		case "draft.get":
			return host.Response{ID: req.ID, Result: json.RawMessage(`{"draft":null}`)}
		case "capture.unified":
			var params struct {
				Mode string `json:"mode"`
			}
			_ = json.Unmarshal(req.Params, &params)
			started <- params.Mode
			return host.Response{ID: req.ID, Result: json.RawMessage(`{"id":"` + params.Mode + `","state":"running"}`)}
		case "capture.activate":
			activations <- struct{}{}
			return host.Response{ID: req.ID, Result: json.RawMessage(`{"active":false}`)}
		case "jobs.get":
			var params struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(req.Params, &params)
			if params.ID == "ocr" {
				<-release
			}
			return host.Response{ID: req.ID, Result: json.RawMessage(`{"id":"` + params.ID + `","state":"completed","result":{}}`)}
		}
		t.Errorf("unexpected %s", req.Method)
		return host.Response{ID: req.ID}
	})
	// Unbuffered: each send returns once the dispatch loop has taken the press.
	registrar := &fakeShortcutRegistrar{events: make(chan string)}
	runtime := &shortcutRuntime{registrar: registrar}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runtime.run(ctx, service)
	wait := func(want string) {
		t.Helper()
		select {
		case mode := <-started:
			if mode != want {
				t.Fatalf("started %s, want %s", mode, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not start", want)
		}
	}
	registrar.events <- "ocr"
	wait("ocr")
	registrar.events <- "capture"
	registrar.events <- "translate"
	close(release)
	wait("translate")
	select {
	case mode := <-started:
		t.Fatalf("an older queued press also ran: %s", mode)
	case <-time.After(200 * time.Millisecond):
	}
}

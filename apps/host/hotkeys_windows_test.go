//go:build windows

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/host"
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
	err = waitCaptureJob(context.Background(), service, host.Response{Result: json.RawMessage(`{"id":"job","state":"running"}`)}, nil)
	if err == nil || !strings.Contains(err.Error(), "复制失败") {
		t.Fatalf("late failure lost: %v", err)
	}
}

func TestShortcutHandsOffOnlyAValidCompletedDraft(t *testing.T) {
	id := strings.Repeat("a", 32)
	opened := ""
	launch := func(value string) error { opened = value; return nil }
	response := host.Response{Result: json.RawMessage(`{"id":"job","state":"completed","result":{"draftId":"` + id + `"}}`)}
	if err := waitCaptureJob(context.Background(), nil, response, launch); err != nil || opened != id {
		t.Fatal(opened, err)
	}
	opened = ""
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitCaptureJob(cancelled, nil, response, launch); err == nil || opened != "" {
		t.Fatal("closing host launched a new editor")
	}
	if err := waitCaptureJob(context.Background(), nil, response, func(string) error { return context.DeadlineExceeded }); err != context.DeadlineExceeded {
		t.Fatal("window launch error was hidden", err)
	}
	opened = ""
	for _, payload := range []string{`{"id":"job","state":"cancelled"}`, `{"id":"job","state":"completed","result":{"id":"ordinary-image"}}`} {
		if err := waitCaptureJob(context.Background(), nil, host.Response{Result: json.RawMessage(payload)}, launch); err != nil || opened != "" {
			t.Fatal(opened, err)
		}
	}
	for _, payload := range []string{`{"id":"job","state":"completed","result":{"draftId":"../bad"}}`, `{"id":"job","state":"unknown"}`} {
		if err := waitCaptureJob(context.Background(), nil, host.Response{Result: json.RawMessage(payload)}, launch); err == nil {
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

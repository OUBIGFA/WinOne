//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/transport"
)

type captureLanguageRecorder struct{ requests chan host.Request }

func (r captureLanguageRecorder) Handle(request host.Request) host.Response {
	if request.Method == "capture.draft.get" {
		return host.Response{ID: request.ID, Result: json.RawMessage(`{"draft":{"id":"image","revision":2,"settings":{"askWhereToSave":false}}}`)}
	}
	r.requests <- request
	return host.Response{ID: request.ID, Result: json.RawMessage(`{"state":"completed"}`)}
}

func TestPinRequestsUseDesktopLanguage(t *testing.T) {
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-pin-language-%d`, time.Now().UnixNano())
	listener, err := transport.Listen(pipe)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	recorder := captureLanguageRecorder{requests: make(chan host.Request, 4)}
	go func() { done <- transport.Serve(ctx, listener, recorder) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	for _, locale := range []string{"en-US", "zh-CN"} {
		app := &App{pipe: pipe, locale: locale}
		for _, method := range []string{"pin.last", "draft.pin", "draft.complete"} {
			if _, err := app.Capture(method, `{"expectedId":"image","revision":2,"language":"stale"}`); err != nil {
				t.Fatal(err)
			}
			request := <-recorder.requests
			var params struct {
				Language   string
				ExpectedID string
				Revision   int
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				t.Fatal(err)
			}
			if params.Language != locale || params.ExpectedID != "image" || params.Revision != 2 {
				t.Fatalf("incorrect pin parameters: %+v", params)
			}
		}
	}
}

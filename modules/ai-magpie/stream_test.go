package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamFailureReportsOnlyStatusAndRetryDelay(t *testing.T) {
	w := newStreamResponse(func(streamEvent) error { return nil })
	w.Header().Set("Retry-After", "120")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":"fixture-private-key and prompt"}`))
	err := w.complete()
	var failure *upstreamFailure
	if !errors.As(err, &failure) || failure.Status != 429 || failure.RetryAfterSeconds != 120 {
		t.Fatalf("metadata=%+v error=%v", failure, err)
	}
	encoded, _ := json.Marshal(failureEvent(err))
	if strings.Contains(string(encoded), "fixture-private-key") || strings.Contains(string(encoded), "prompt") {
		t.Fatal("vendor body leaked")
	}
}

func TestLimitFailureClassificationDoesNotExposeVendorBodies(t *testing.T) {
	for _, test := range []struct {
		status  int
		body    string
		limited bool
	}{
		{429, "private-vendor-data", true},
		{403, `{"error":{"message":"Your quota has been exhausted","type":"insufficient_quota"}}`, true},
		{402, `{"error":{"message":"Insufficient balance","type":"insufficient_balance"}}`, true},
		{503, `{"error":{"message":"server unavailable private-vendor-data"}}`, false},
	} {
		w := newStreamResponse(func(streamEvent) error { return nil })
		w.WriteHeader(test.status)
		_, _ = w.Write([]byte(test.body))
		raw, _ := json.Marshal(failureEvent(w.complete()))
		var event struct {
			Limited bool `json:"limited"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		if event.Limited != test.limited {
			t.Fatalf("status %d classification=%s", test.status, raw)
		}
		if strings.Contains(string(raw), "private-vendor-data") || strings.Contains(string(raw), "quota has") {
			t.Fatal("vendor body escaped worker")
		}
	}
}

func TestStreamingPublishesReasoningAndTextBeforeCompletion(t *testing.T) {
	seen := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request["stream"] != true {
			t.Error("stream was not requested")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"分析输入\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"首段🙂\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-seen:
		case <-ctx.Done():
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"后段\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	input := fmt.Sprintf(`{"protocol":"chat","baseUrl":%q,"model":"test","text":"Hello"}`, server.URL+"/v1")
	var text, reasoning string
	done := false
	_, err := runOutput(ctx, strings.NewReader(input), func(e streamEvent) error {
		switch e.Type {
		case "text":
			text += e.Delta
			if text == "首段🙂" {
				close(seen)
			}
		case "reasoning":
			reasoning += e.Delta
		case "done":
			done = true
		}
		return nil
	})
	if err != nil || text != "首段🙂后段" || reasoning != "分析输入" || !done {
		t.Fatalf("stream text=%q reasoning=%q done=%v err=%v", text, reasoning, done, err)
	}
}

func TestStreamRejectsTruncationErrorsAndOversizedContent(t *testing.T) {
	for _, data := range []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n",
		"data: {\"error\":{\"message\":\"secret-upstream-text\"}}\n\n",
		"data: " + strings.Repeat("x", (1<<20)+1),
	} {
		w := newStreamResponse(func(streamEvent) error { return nil })
		_, _ = w.Write([]byte(data))
		err := w.complete()
		if err == nil || strings.Contains(err.Error(), "secret-upstream-text") {
			t.Fatal("invalid stream accepted or leaked vendor response")
		}
	}
}

func TestStreamAcceptsUTF8SplitAcrossWrites(t *testing.T) {
	var text string
	w := newStreamResponse(func(e streamEvent) error {
		if e.Type == "text" {
			text += e.Delta
		}
		return nil
	})
	input := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"你好🙂\"},\"finish_reason\":\"stop\"}]}\r\n\r\ndata: [DONE]\r\n\r\n")
	for _, b := range input {
		if _, err := w.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.complete(); err != nil || text != "你好🙂" {
		t.Fatalf("split text %q: %v", text, err)
	}
}

func TestStreamAcceptsAccountingTrailersWithoutAcceptingLaterAnswers(t *testing.T) {
	base := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	for _, scenario := range []struct {
		name, trailer string
		accepted      bool
	}{
		{"cost", `{"choices":[],"cost":null}`, true},
		{"usage", `{"choices":[],"usage":{"total_tokens":12}}`, true},
		{"later text", `{"choices":[{"delta":{"content":"late"}}],"cost":null}`, false},
		{"error", `{"choices":[],"cost":null,"error":{"message":"failure"}}`, false},
		{"unknown frame", `{"choices":[],"content":"late"}`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var text string
			done := 0
			w := newStreamResponse(func(event streamEvent) error {
				if event.Type == "text" {
					text += event.Delta
				}
				if event.Type == "done" {
					done++
				}
				return nil
			})
			_, writeErr := w.Write([]byte(base + "data: " + scenario.trailer + "\n\n"))
			err := w.complete()
			if scenario.accepted && (writeErr != nil || err != nil || done != 1) {
				t.Fatalf("accounting trailer failed: write=%v complete=%v done=%d", writeErr, err, done)
			}
			if !scenario.accepted && (err == nil || done != 0) {
				t.Fatal("content or error after completion was accepted")
			}
			if text != "answer" {
				t.Fatalf("trailer changed the answer: %q", text)
			}
		})
	}
}

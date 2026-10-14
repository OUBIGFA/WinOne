package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// The embedded gateway normalizes all vendor protocols to Chat Completions SSE.
// Emit bounded NDJSON over the owned process pipe; never buffer a whole response
// or forward raw error bodies that can repeat credentials and source text.
type streamEvent struct {
	Type              string `json:"type"`
	Delta             string `json:"delta,omitempty"`
	Error             string `json:"error,omitempty"`
	Status            int    `json:"status,omitempty"`
	RetryAfterSeconds int    `json:"retryAfterSeconds,omitempty"`
}
type streamResponse struct {
	header        http.Header
	code          int
	pending       []byte
	data          []string
	emit          func(streamEvent) error
	err           error
	finish        string
	done, hasText bool
	total         int
	eventBytes    int
	cancel        func()
}

func newStreamResponse(emit func(streamEvent) error) *streamResponse {
	return &streamResponse{header: make(http.Header), code: http.StatusOK, emit: emit}
}
func (w *streamResponse) Header() http.Header  { return w.header }
func (w *streamResponse) WriteHeader(code int) { w.code = code }
func (w *streamResponse) Flush()               {}
func (w *streamResponse) Write(p []byte) (int, error) {
	defer func() {
		if w.err != nil && w.cancel != nil {
			w.cancel()
		}
	}()
	if w.err != nil {
		return 0, w.err
	}
	if w.code != http.StatusOK {
		return len(p), nil
	}
	if len(w.pending)+len(p) > 1<<20 {
		w.err = errors.New("AI stream event is too large")
		return 0, w.err
	}
	w.pending = append(w.pending, p...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			break
		}
		line := strings.TrimSuffix(string(w.pending[:end]), "\r")
		w.pending = w.pending[end+1:]
		if line == "" {
			w.err = w.event()
			w.data = nil
			w.eventBytes = 0
		} else if strings.HasPrefix(line, "data:") {
			w.eventBytes += len(line)
			if w.eventBytes > 1<<20 {
				w.err = errors.New("AI stream event is too large")
				return 0, w.err
			}
			w.data = append(w.data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		if w.err != nil {
			return 0, w.err
		}
	}
	return len(p), nil
}
func (w *streamResponse) event() error {
	if len(w.data) == 0 {
		return nil
	}
	data := strings.Join(w.data, "\n")
	if w.done {
		if accountingTrailer(data) {
			return nil
		}
		return errors.New("AI stream continued after completion")
	}
	if data == "[DONE]" {
		w.done = true
		return nil
	}
	var chunk struct {
		Error   json.RawMessage `json:"error"`
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Content   string            `json:"content"`
				Reasoning string            `json:"reasoning_content"`
				Tools     []json.RawMessage `json:"tool_calls"`
			} `json:"delta"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
	}
	if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Error) > 0 && string(chunk.Error) != "null" {
		return errors.New("AI stream failed or is invalid")
	}
	if len(chunk.Choices) > 1 {
		return errors.New("AI stream has multiple choices")
	}
	for _, choice := range chunk.Choices {
		if choice.Index != 0 || len(choice.Delta.Tools) > 0 {
			return errors.New("AI stream returned an unsupported action")
		}
		if w.finish != "" && (choice.Delta.Content != "" || choice.Delta.Reasoning != "") {
			return errors.New("AI stream text arrived after finish")
		}
		for _, event := range []streamEvent{{Type: "reasoning", Delta: choice.Delta.Reasoning}, {Type: "text", Delta: choice.Delta.Content}} {
			if event.Delta == "" {
				continue
			}
			w.total += len(event.Delta)
			if w.total > 1<<20 {
				return errors.New("AI response is too large")
			}
			if event.Type == "text" && strings.TrimSpace(event.Delta) != "" {
				w.hasText = true
			}
			if err := w.emit(event); err != nil {
				return err
			}
		}
		if choice.Finish != "" {
			w.finish = choice.Finish
		}
	}
	return nil
}

// OpenCode and other gateway streams can append usage/cost accounting after
// [DONE]. Such a frame has no choices and cannot add an answer or action.
// Keep rejecting errors, unknown frames and any new choice after completion.
func accountingTrailer(data string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(data), &fields) != nil {
		return false
	}
	if _, usage := fields["usage"]; !usage {
		if _, cost := fields["cost"]; !cost {
			return false
		}
	}
	var choices []json.RawMessage
	if json.Unmarshal(fields["choices"], &choices) != nil || choices == nil || len(choices) != 0 {
		return false
	}
	for key := range fields {
		switch key {
		case "choices", "usage", "cost", "id", "object", "created", "model", "system_fingerprint", "service_tier":
		default:
			return false
		}
	}
	return true
}
func (w *streamResponse) complete() error {
	if w.err != nil {
		return w.err
	}
	if w.code != http.StatusOK {
		return &upstreamFailure{Status: w.code, RetryAfterSeconds: retryDelay(w.header.Get("Retry-After"))}
	}
	if !w.done || len(w.pending) != 0 || len(w.data) != 0 || (w.finish != "stop" && w.finish != "end_turn") || !w.hasText {
		return errors.New("AI stream did not finish with a complete answer")
	}
	return w.emit(streamEvent{Type: "done"})
}

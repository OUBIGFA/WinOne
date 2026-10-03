//go:build windows

package main

import (
	"crypto/rand"
	"errors"
	"strconv"
	"sync"
	"winone/internal/capturehandoff"
)

type captureWindowStatus struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}
type captureWindows struct {
	mu            sync.Mutex
	start         func(string) (<-chan error, error)
	next          uint64
	prefix        string
	active, draft string
	records       map[string]captureWindowStatus
	order         []string
}

func newCaptureWindows(start func(string) (<-chan error, error)) *captureWindows {
	return &captureWindows{start: start, prefix: rand.Text(), records: map[string]captureWindowStatus{}}
}

// The Host owns the actual screenshot process. Repeated requests refer to the
// same lifetime, not to a short-lived single-instance notification process.
func (w *captureWindows) Open(draft string) (captureWindowStatus, error) {
	if err := capturehandoff.ValidateDraftID(draft); err != nil {
		return captureWindowStatus{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.active != "" {
		if w.draft != draft {
			return captureWindowStatus{}, errors.New("请先结束当前截图")
		}
		return w.records[w.active], nil
	}
	done, err := w.start(draft)
	if err != nil {
		return captureWindowStatus{}, err
	}
	w.next++
	id := w.prefix + "-" + strconv.FormatUint(w.next, 10)
	value := captureWindowStatus{ID: id, State: "running"}
	w.records[id] = value
	w.active = id
	w.draft = draft
	w.order = append(w.order, id)
	// Keep a bounded completion history so a caller can observe an exit even
	// when another screenshot starts before its next status request.
	if len(w.order) > 16 {
		delete(w.records, w.order[0])
		w.order = w.order[1:]
	}
	go func() {
		err := <-done
		w.mu.Lock()
		defer w.mu.Unlock()
		result := captureWindowStatus{ID: id, State: "completed"}
		if err != nil {
			result.State = "failed"
			result.Error = err.Error()
		}
		w.records[id] = result
		if w.active == id {
			w.active = ""
			w.draft = ""
		}
	}()
	return value, nil
}
func (w *captureWindows) Status(id string) (captureWindowStatus, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	result, ok := w.records[id]
	if !ok {
		return captureWindowStatus{}, errors.New("截图窗口状态已失效")
	}
	return result, nil
}

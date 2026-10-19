//go:build windows

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	"winone/internal/host"
)

func waitingAIJob(t *testing.T) (*aiService, *aiJob, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	job := &aiJob{ID: "owned", owner: 42, State: "running", cancel: cancel, settled: ctx.Done()}
	return &aiService{jobs: map[string]*aiJob{job.ID: job}}, job, cancel
}

func waitJobRequest(owner uint32, method string, wait int) host.Request {
	params, _ := json.Marshal(map[string]any{"id": "owned", "waitMs": wait})
	return host.Request{Version: 1, ID: "wait", PeerPID: owner, Method: method, Params: params}
}

func TestAIJobWaitWakesOnCompletionWithoutHoldingStateLock(t *testing.T) {
	service, job, finish := waitingAIJob(t)
	defer finish()
	done := make(chan host.Response, 1)
	go func() { done <- service.handle(waitJobRequest(42, "jobs.get", 500)) }()
	select {
	case <-done:
		t.Fatal("long wait returned the running state immediately")
	case <-time.After(20 * time.Millisecond):
	}
	service.mu.Lock()
	job.Text, job.State = "ready OCR", "completed"
	service.mu.Unlock()
	finish()
	select {
	case response := <-done:
		var result aiJob
		if response.Error != nil || json.Unmarshal(response.Result, &result) != nil || result.State != "completed" || result.Text != "ready OCR" {
			t.Fatalf("wrong settled result: %s %+v", response.Result, response.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("completion did not wake the waiting client")
	}
}

func TestAIJobWaitKeepsCancellationAndIdentityBoundaries(t *testing.T) {
	service, _, finish := waitingAIJob(t)
	defer finish()
	for _, request := range []host.Request{waitJobRequest(7, "jobs.get", 500), waitJobRequest(42, "jobs.get", -1), waitJobRequest(42, "jobs.get", 501), waitJobRequest(42, "jobs.cancel", 100)} {
		if response := service.handle(request); response.Error == nil {
			t.Fatal("invalid owner/wait accepted")
		}
	}
	done := make(chan host.Response, 1)
	go func() { done <- service.handle(waitJobRequest(42, "jobs.get", 500)) }()
	if response := service.handle(waitJobRequest(42, "jobs.cancel", 0)); response.Error != nil {
		t.Fatal(response.Error)
	}
	select {
	case response := <-done:
		if response.Error == nil {
			t.Fatal("cancelled job leaked a stale result")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not release the waiting client")
	}
}

func TestAIJobWaitIsBoundedAndDefaultGetRemainsImmediate(t *testing.T) {
	service, _, finish := waitingAIJob(t)
	defer finish()
	for _, wait := range []int{0, 10} {
		response := service.handle(waitJobRequest(42, "jobs.get", wait))
		var result aiJob
		if response.Error != nil || json.Unmarshal(response.Result, &result) != nil || result.State != "running" {
			t.Fatal("bounded/default read changed state")
		}
	}
}

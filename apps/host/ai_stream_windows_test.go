//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/host"
)

func TestAIJobsExposeOnlyNewStreamTextAndReasoning(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("explicit real adapter test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	release := make(chan struct{})
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"思考\",\"content\":\"首段🙂\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		case <-ctx.Done():
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"结束\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer vendor.Close()
	engine := newAIService(ctx, t.TempDir(), exe)
	engine.translator.Executable = exe
	store, err := ai.Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := store.Snapshot()
	p.BaseURL = vendor.URL + "/v1"
	p.Model = "fixture-model"
	if _, err = store.Save(p, nil); err != nil {
		t.Fatal(err)
	}
	engine.store = store
	call := func(method string, params any) host.Response {
		b, _ := json.Marshal(params)
		return engine.handle(host.Request{ID: "stream", Method: method, PeerPID: uint32(os.Getpid()), Params: b})
	}
	started := call("generate.start", map[string]string{"text": "fixture"})
	if started.Error != nil {
		t.Fatal(started.Error)
	}
	var job struct{ ID, State, Text, Reasoning string }
	_ = json.Unmarshal(started.Result, &job)
	defer call("jobs.cancel", map[string]string{"id": job.ID})
	deadline := time.Now().Add(3 * time.Second)
	for job.Text == "" && time.Now().Before(deadline) {
		r := call("jobs.get", map[string]string{"id": job.ID})
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		_ = json.Unmarshal(r.Result, &job)
		time.Sleep(10 * time.Millisecond)
	}
	if job.State != "running" || job.Text != "首段🙂" || job.Reasoning != "思考" {
		t.Fatalf("no live output: %+v", job)
	}
	offset := map[string]any{"id": job.ID, "textOffset": len(job.Text), "reasoningOffset": len(job.Reasoning)}
	latest := call("jobs.get", offset)
	if latest.Error != nil {
		t.Fatal(latest.Error)
	}
	var delta struct{ Text, Reasoning string }
	_ = json.Unmarshal(latest.Result, &delta)
	if delta.Text != "" || delta.Reasoning != "" {
		t.Fatal("stream replayed already consumed text")
	}
	bad := call("jobs.get", map[string]any{"id": job.ID, "textOffset": 1})
	if bad.Error == nil {
		t.Fatal("offset split a UTF-8 character")
	}
	close(release)
	for job.State == "running" && ctx.Err() == nil {
		r := call("jobs.get", offset)
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		_ = json.Unmarshal(r.Result, &job)
		time.Sleep(10 * time.Millisecond)
	}
	if job.State != "completed" || job.Text != "结束" || strings.Contains(job.Text, "首段") {
		t.Fatalf("final delta: %+v", job)
	}
}

func TestCompletedLargeStreamFitsFramesWithoutEarlyCompletion(t *testing.T) {
	text := strings.Repeat("\x01🙂", 30000)
	job := &aiJob{ID: "large", State: "completed", Text: text, owner: 1}
	engine := &aiService{jobs: map[string]*aiJob{"large": job}}
	var received strings.Builder
	for index := 0; index < 20; index++ {
		params, _ := json.Marshal(map[string]any{"id": "large", "textOffset": received.Len(), "reasoningOffset": 0})
		response := engine.handle(host.Request{ID: "read", Method: "jobs.get", PeerPID: 1, Params: params})
		if response.Error != nil || len(response.Result) > 1<<20 {
			t.Fatalf("large frame failed: %v", response.Error)
		}
		var delta struct{ Text, State string }
		if err := json.Unmarshal(response.Result, &delta); err != nil {
			t.Fatal(err)
		}
		received.WriteString(delta.Text)
		if delta.State == "completed" {
			if received.String() != text {
				t.Fatal("terminal state arrived before the final text")
			}
			return
		}
	}
	t.Fatal("large output did not drain")
}

//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeSelectionPipeline(t *testing.T) {
	probe, bundle, output := os.Getenv("WINONE_SELECTION_PIPELINE_PROBE"), os.Getenv("WINONE_SELECTION_PIPELINE_BUNDLE"), os.Getenv("WINONE_SELECTION_PIPELINE_OUTPUT")
	if probe == "" || bundle == "" || output == "" {
		t.Skip("set explicit selection pipeline fixture, bundle and new output directory")
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect || r.URL.IsAbs() {
			t.Error("selection translation tried an external endpoint instead of its configured AI model")
			http.Error(w, "external fixture requests are blocked", http.StatusBadGateway)
			return
		}
		var request struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid text-only AI request")
		}
		expectedKey := "Bearer fixture-key"
		if request.Model == "custom-model" {
			expectedKey = "Bearer custom-key"
		}
		if !request.Stream || r.Header.Get("Authorization") != expectedKey {
			t.Error("shared AI configuration was not used")
		}
		selectedSource := false
		for _, message := range request.Messages {
			if strings.Contains(message.Content, "Selected content") {
				selectedSource = true
			}
		}
		if !selectedSource {
			t.Error("selected text did not reach the task or its conversation")
		}
		round := calls.Add(1)
		answer := "后续对话结果"
		switch round {
		case 1:
			answer = "这是划词翻译结果"
			if request.Model != "fixture-translate" {
				t.Error("translation task assignment lost")
			}
		case 2:
			if request.Model != "fixture-translate" || len(request.Messages) < 4 {
				t.Error("conversation lost route or history")
			}
		case 3:
			answer = "自定义功能结果\n\n```go\nfmt.Println(\"answer\")\n```"
			if request.Model != "custom-model" || !strings.Contains(r.URL.Path, "/custom/") || len(request.Messages) != 2 || !strings.Contains(request.Messages[0].Content, "Provide the custom fixture answer.") {
				t.Error("custom instruction was not sent to shared AI service")
			}
		case 4:
			if request.Model != "custom-model" || len(request.Messages) < 4 {
				t.Error("custom action conversation lost its model or history")
			}
		default:
			t.Error("unexpected generation request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		event, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": answer}, "finish_reason": "stop"}}})
		_, _ = w.Write(append(append([]byte("data: "), event...), []byte("\n\ndata: [DONE]\n\n")...))
		w.(http.Flusher).Flush()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, probe, "--pipeline", output)
	command.Env = append(os.Environ(), "WINONE_SELECTION_PIPELINE_BUNDLE="+bundle, "WINONE_SELECTION_PIPELINE_URL="+server.URL+"/v1")
	// A routing regression
	// must fail locally rather than sending selected text to that public service.
	command.Env = append(command.Env, "HTTP_PROXY="+server.URL, "HTTPS_PROXY="+server.URL, "NO_PROXY=127.0.0.1,localhost,::1")
	log, err := command.CombinedOutput()
	if writeErr := os.WriteFile(filepath.Join(output, "probe.log"), log, 0600); writeErr != nil {
		t.Error(writeErr)
	}
	if err != nil {
		t.Fatalf("selection pipeline: %v (generation requests=%d)\n%s", err, calls.Load(), log)
	}
	if calls.Load() != 4 {
		t.Fatalf("expected two initial tasks and two conversation calls, got %d", calls.Load())
	}
	t.Log("Global selection reached the shared native toolbar, translation/custom action, conversation, copy and module shutdown")
}

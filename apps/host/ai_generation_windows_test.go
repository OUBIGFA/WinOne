//go:build windows

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"winone/internal/ai"
	"winone/internal/host"
	"winone/internal/settings"
)

// The real magpie executable and a loopback vendor prove that unrelated module
// callers receive the same saved model/key without supplying provider wiring.
func TestSharedAIModelForTextAndImage(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_AI_TEST_EXE to the built magpie adapter")
	}
	root := t.TempDir()
	target := filepath.Join(root, "shared", "ai", "WinOne.AI.exe")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	program, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(target, program, 0700); err != nil {
		t.Fatal(err)
	}
	var textCalls, imageCalls atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "shared-model" || r.Header.Get("Authorization") != "Bearer fixture-shared-key" {
			t.Error("saved provider or credentials were not used")
		}
		if len(body.Messages) == 2 && strings.Contains(string(body.Messages[1].Content), "data:image/png;base64,") {
			imageCalls.Add(1)
		} else {
			textCalls.Add(1)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Shared result\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer vendor.Close()
	store, err := ai.Open(filepath.Join(root, "ai-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := store.Snapshot()
	p.BaseURL = vendor.URL + "/v1"
	p.Model = "shared-model"
	p.OCREngine = "ai"
	secret := "fixture-shared-key"
	if _, err = store.Save(p, &secret); err != nil {
		t.Fatal(err)
	}
	preferences, err := settings.Open(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(preferences)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err = registerAI(service, ctx, root, filepath.Join(root, "WinOne.Host.exe")); err != nil {
		t.Fatal(err)
	}
	peer := uint32(os.Getpid())
	call := func(method string, params any, owner uint32) host.Response {
		data, _ := json.Marshal(params)
		return service.Handle(host.Request{Version: 1, ID: "shared-ai", Method: method, Params: data, PeerPID: owner})
	}
	complete := func(params any) {
		t.Helper()
		started := call("ai.generate.start", params, peer)
		if started.Error != nil {
			t.Fatal(started.Error)
		}
		var job aiJob
		if err := json.Unmarshal(started.Result, &job); err != nil {
			t.Fatal(err)
		}
		defer call("ai.jobs.cancel", map[string]string{"id": job.ID}, peer)
		var selected struct {
			Model        string `json:"model"`
			ProviderName string `json:"providerName"`
		}
		if err := json.Unmarshal(started.Result, &selected); err != nil || selected.Model != "shared-model" || selected.ProviderName != "现有服务" {
			t.Error("job did not report its selected provider name and model")
		}
		if result := call("ai.jobs.get", map[string]string{"id": job.ID}, peer+1); result.Error == nil {
			t.Fatal("another module read the result")
		}
		deadline := time.Now().Add(15 * time.Second)
		for job.State == "running" && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			result := call("ai.jobs.get", map[string]string{"id": job.ID}, peer)
			if result.Error != nil {
				t.Fatal(result.Error)
			}
			if err := json.Unmarshal(result.Result, &job); err != nil {
				t.Fatal(err)
			}
		}
		if job.State != "completed" || job.Text != "Shared result" {
			t.Fatalf("shared job: %+v", job)
		}
	}
	complete(map[string]string{"text": "Synthetic text for a future module"})
	image, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1cAAAAASUVORK5CYII=")
	name := fmt.Sprintf(`Local\WinOne.AI.%d.%032x`, peer, time.Now().UnixNano())
	key, _ := windows.UTF16PtrFromString(name)
	mapping, err := windows.CreateFileMapping(windows.InvalidHandle, nil, windows.PAGE_READWRITE, 0, uint32(len(image)), key)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(mapping)
	view, err := windows.MapViewOfFile(mapping, windows.FILE_MAP_WRITE, 0, 0, uintptr(len(image)))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.UnmapViewOfFile(view)
	if err = windows.WriteProcessMemory(windows.CurrentProcess(), view, &image[0], uintptr(len(image)), nil); err != nil {
		t.Fatal(err)
	}
	complete(map[string]any{"purpose": "ocr", "image": ai.ImageReference{Mapping: name, Bytes: len(image)}})
	if textCalls.Load() != 1 || imageCalls.Load() != 1 {
		t.Fatalf("requests: text=%d, image=%d", textCalls.Load(), imageCalls.Load())
	}
	if state := call("ai.settings.get", nil, peer); state.Error != nil || !strings.Contains(string(state.Result), `"engine":"google"`) {
		t.Fatal("model calls changed translation preference")
	}
}

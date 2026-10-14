//go:build windows

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRelocatedCompiledWorkerUsesBundledPhotonAndPreservesOriginalImage(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" || !strings.HasSuffix(worker, ".js") {
		t.Skip("explicit compiled Chat fixture")
	}
	wasm := filepath.Join(filepath.Dir(worker), "photon_rs_bg.wasm")
	if _, err := os.Stat(wasm); err != nil {
		t.Fatal("compiled Chat bundle is missing Photon", err)
	}
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	os.Mkdir(runtimeDir, 0700)
	executable := filepath.Join(runtimeDir, "bun.exe")
	if err := os.Link(bun, executable); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(wasm, filepath.Join(runtimeDir, "photon_rs_bg.wasm")); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(worker)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _ := json.Marshal(strings.ToLower(filepath.ToSlash(runtimeDir)) + "/")
	// Simulate deployment away from the build machine: only the bundle's WASM
	// path may be read. Pi's own documented fallback must resolve that resource.
	guard := fmt.Sprintf("import {createRequire as __testRequire} from 'node:module'; const __testFS=__testRequire(import.meta.url)('fs'); const __testRead=__testFS.readFileSync; __testFS.readFileSync=function(file,...args){const value=String(file).split(String.fromCharCode(92)).join('/').toLowerCase();if(value.endsWith('photon_rs_bg.wasm')){if(!value.startsWith(%s))throw Object.assign(new Error('Build-machine image resource unavailable'),{code:'ENOENT'});console.error('BUNDLED_WASM_READ');}return __testRead.call(this,file,...args);};\n", allowed)
	isolatedWorker := filepath.Join(root, "worker.js")
	if err = os.WriteFile(isolatedWorker, append([]byte(guard), source...), 0600); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspace")
	sessions := filepath.Join(root, "sessions")
	os.Mkdir(workspace, 0700)
	attachment := filepath.Join(sessions, "attachments", "one", "large.png")
	os.MkdirAll(filepath.Dir(attachment), 0700)
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	original := append(encoded.Bytes(), make([]byte, 13<<20)...)
	if err = os.WriteFile(attachment, original, 0600); err != nil {
		t.Fatal(err)
	}
	received := make(chan int, 1)
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaximumChatRequestBytes))
		if err != nil {
			t.Error(err)
			http.Error(w, "too large", 413)
			return
		}
		if !bytes.Contains(body, []byte("data:image/")) {
			t.Error("image was silently omitted")
		}
		received <- len(body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Image accepted\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer vendor.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	events := make(chan json.RawMessage, 128)
	runner, err := StartRunner(ctx, executable, isolatedWorker, workspace, func(raw json.RawMessage) { events <- raw })
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closing, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runner.Close(closing)
	}()
	config := map[string]any{"cwd": workspace, "sessionDirectory": sessions, "agentDir": filepath.Join(root, "agent"), "baseUrl": vendor.URL + "/v1", "key": "fixture-session-token", "model": map[string]any{"id": "fixture", "name": "Fixture", "vision": true}}
	if _, err = runner.Call(ctx, "init", config); err != nil {
		t.Fatal(err)
	}
	if _, err = runner.CallOnce(ctx, "image-request", "prompt", map[string]any{"text": "Read this original image", "attachments": []string{attachment}}); err != nil {
		t.Fatal(err)
	}
	settled := false
	for !settled {
		select {
		case raw := <-events:
			var event struct{ Type, Status, Error string }
			json.Unmarshal(raw, &event)
			if event.Type == "settled" {
				if event.Status != "completed" {
					t.Fatal(event.Status, event.Error)
				}
				settled = true
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	select {
	case size := <-received:
		if size > MaximumChatRequestBytes {
			t.Fatal("oversized payload", size)
		}
	default:
		t.Fatal("image never reached the real HTTP request")
	}
	if _, err = runner.Call(ctx, "state", nil); err != nil {
		t.Fatal(err)
	}
	closing, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err = runner.Close(closing); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(runner.cmd.Stderr.(*runnerDiagnostics).String(), "BUNDLED_WASM_READ") {
		t.Fatal("test did not exercise the deployed WASM fallback")
	}
	retained, err := os.ReadFile(attachment)
	if err != nil || !bytes.Equal(retained, original) {
		t.Fatal("imported original image changed", err)
	}
}

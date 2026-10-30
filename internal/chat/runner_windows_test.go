//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativePiRunnerUsesItsToolsAndDoesNotRepeatAcceptedPrompt(t *testing.T) {
	bun, script := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || script == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	os.Mkdir(workspace, 0700)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		var delta map[string]any
		finish := "stop"
		if calls.Add(1) == 1 {
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "write-one", "type": "function", "function": map[string]any{"name": "write", "arguments": `{"path":"proof.txt","content":"saved by Pi"}`}}}}
			finish = "tool_calls"
		} else {
			delta = map[string]any{"content": "Done"}
		}
		for _, value := range []map[string]any{{"reasoning_content": "Fixture reasoning"}, delta, {}} {
			data, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "service/model", "choices": []any{map[string]any{"index": 0, "delta": value, "finish_reason": func() any {
				if len(value) == 0 {
					return finish
				}
				return nil
			}()}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	events := make(chan json.RawMessage, 128)
	runner, err := StartRunner(ctx, bun, script, workspace, func(value json.RawMessage) { events <- value })
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := runner.Close(closeCtx); err != nil {
			t.Error(err)
		}
	}()
	config := map[string]any{"cwd": workspace, "agentDir": filepath.Join(root, "agent"), "sessionDirectory": filepath.Join(root, "sessions"), "baseUrl": server.URL + "/v1", "key": "winone-fixture-session-token", "model": map[string]string{"id": "service/model", "name": "Fixture model"}, "permission": "ask", "language": "zh-CN", "title": "Fixture"}
	if _, err = runner.Call(ctx, "init", config); err != nil {
		t.Fatal(err)
	}
	params := map[string]string{"text": "Create proof.txt."}
	if _, err = runner.CallOnce(ctx, "owned-prompt", "prompt", params); err != nil {
		t.Fatal(err)
	}
	settled, saved := false, false
	for !settled {
		select {
		case raw := <-events:
			var event struct {
				Type    string `json:"type"`
				ID      string `json:"id"`
				Status  string `json:"status"`
				Error   string `json:"error"`
				Request struct {
					ID string `json:"id"`
				} `json:"request"`
			}
			if err = json.Unmarshal(raw, &event); err != nil {
				t.Fatal(err)
			}
			switch event.Type {
			case "approval":
				if _, err = runner.Call(ctx, "permission", map[string]any{"id": event.Request.ID, "allow": true}); err != nil {
					t.Fatal(err)
				}
			case "input_saved":
				saved = event.ID == "owned-prompt"
			case "settled":
				if event.Status != "completed" {
					t.Fatal(event.Status, event.Error)
				}
				settled = true
			}
		case <-ctx.Done():
			t.Fatal("Pi run did not settle", ctx.Err())
		}
	}
	if !saved {
		t.Fatal("user input was not acknowledged as persisted")
	}
	state, err := runner.Call(ctx, "state", nil)
	if err != nil {
		t.Fatal(err)
	}
	var savedState struct {
		File       string `json:"file"`
		Operations []struct {
			Kind     string `json:"kind"`
			Status   string `json:"status"`
			TurnID   string `json:"turnId"`
			Duration *int64 `json:"durationMs"`
		} `json:"operations"`
	}
	if err = json.Unmarshal(state, &savedState); err != nil {
		t.Fatal(err)
	}
	inspection, err := runner.Call(ctx, "inspect", map[string]string{"file": savedState.File, "directory": filepath.Dir(savedState.File), "cwd": workspace})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(inspection, &savedState); err != nil {
		t.Fatal(err)
	}
	turnRecorded, approvalRecorded := false, false
	for _, operation := range savedState.Operations {
		if operation.TurnID != "owned-prompt" || operation.Duration == nil || *operation.Duration < 0 {
			t.Fatalf("invalid persisted process operation: %+v", operation)
		}
		turnRecorded = turnRecorded || operation.Kind == "turn" && operation.Status == "completed"
		approvalRecorded = approvalRecorded || operation.Kind == "approval" && operation.Status == "allowed"
	}
	if !turnRecorded || !approvalRecorded {
		t.Fatal("reopened Pi history lost its run/approval process records")
	}
	history, err := os.ReadFile(savedState.File)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(history), `"customType":"winone.thinking"`) || !strings.Contains(string(state), "thinkingDurationMs") {
		t.Fatal("thinking timing did not persist with Pi history")
	}
	data, err := os.ReadFile(filepath.Join(workspace, "proof.txt"))
	if err != nil || string(data) != "saved by Pi" {
		t.Fatal(string(data), err)
	}
	if _, err = runner.CallOnce(ctx, "owned-prompt", "prompt", params); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("accepted prompt was executed again", calls.Load())
	}
	if _, err = runner.CallOnce(ctx, "owned-prompt", "prompt", map[string]string{"text": "different"}); err == nil {
		t.Fatal("request identifier reused with different data")
	}
}

func TestRunnerRetainsFinalReplyAndReapsDescendants(t *testing.T) {
	bun := os.Getenv("WINONE_CHAT_BUN_EXE")
	if bun == "" {
		t.Skip("explicit Bun runtime fixture")
	}
	root := t.TempDir()
	script := filepath.Join(root, "fixture.mjs")
	code := `import {spawn} from 'node:child_process';process.stdout.write(JSON.stringify({type:'ready',protocol:1})+'\n');let input='';process.stdin.on('data',chunk=>{input+=chunk;const end=input.indexOf('\n');if(end<0)return;const request=JSON.parse(input.slice(0,end));const child=spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{detached:true,windowsHide:true,stdio:['ignore','inherit','inherit']});child.unref();process.stdout.write(JSON.stringify({type:'response',id:request.id,ok:true,value:{text:request.params.text,pid:child.pid}})+'\n',()=>process.exit(0));});`
	os.WriteFile(script, []byte(code), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	runner, err := StartRunner(ctx, bun, script, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := runner.Call(ctx, "echo", map[string]string{"text": "a\u2028b\nc"})
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Text string `json:"text"`
		PID  uint32 `json:"pid"`
	}
	json.Unmarshal(data, &value)
	if value.Text != "a\u2028b\nc" {
		t.Fatal("Unicode JSONL framing was changed", value.Text)
	}
	select {
	case <-runner.Done():
	case <-ctx.Done():
		t.Fatal("inherited stdout kept the runner alive")
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, value.PID)
	if err == nil {
		defer windows.CloseHandle(process)
		state, err := windows.WaitForSingleObject(process, 1000)
		if err != nil || state != windows.WAIT_OBJECT_0 {
			t.Fatal("descendant remained after its worker exited", state, err)
		}
	} else if err != windows.ERROR_INVALID_PARAMETER {
		t.Fatal(err)
	}
}

// A proxy configured for the user's network must not intercept worker calls to
// loopback services: they carry private keys and a remote proxy cannot reach
// this machine's 127.0.0.1. The proxy here is a closed port, so a proxied
// request fails; the user's own NO_PROXY entries must survive the merge.
func TestRunnerLoopbackRequestsBypassInheritedProxy(t *testing.T) {
	bun := os.Getenv("WINONE_CHAT_BUN_EXE")
	if bun == "" {
		t.Skip("explicit Bun runtime fixture")
	}
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxy := "http://" + closed.Addr().String()
	closed.Close()
	t.Setenv("HTTP_PROXY", proxy)
	t.Setenv("HTTPS_PROXY", proxy)
	t.Setenv("NO_PROXY", "intranet.example")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "direct") }))
	defer server.Close()
	root := t.TempDir()
	script := filepath.Join(root, "fixture.mjs")
	code := `process.stdout.write(JSON.stringify({type:'ready',protocol:1})+'\n');let input='';process.stdin.on('data',async chunk=>{input+=chunk;const end=input.indexOf('\n');if(end<0)return;const request=JSON.parse(input.slice(0,end));input=input.slice(end+1);let value;try{const response=await fetch(request.params.url);value={body:await response.text(),bypass:process.env.NO_PROXY};}catch(error){value={error:String(error),bypass:process.env.NO_PROXY};}process.stdout.write(JSON.stringify({type:'response',id:request.id,ok:true,value})+'\n');});`
	os.WriteFile(script, []byte(code), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runner, err := StartRunner(ctx, bun, script, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		runner.Close(closeCtx)
	}()
	data, err := runner.Call(ctx, "fetch", map[string]string{"url": server.URL})
	if err != nil {
		t.Fatal(err)
	}
	var value struct{ Body, Bypass, Error string }
	json.Unmarshal(data, &value)
	if value.Body != "direct" {
		t.Fatalf("loopback request went through the proxy: %+v", value)
	}
	if !strings.HasPrefix(value.Bypass, "intranet.example,") || !strings.Contains(value.Bypass, "127.0.0.1") {
		t.Fatal("NO_PROXY was not merged", value.Bypass)
	}
}

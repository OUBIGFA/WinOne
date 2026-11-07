//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"winone/internal/chat"
)

// Real bridge -> real Runner JSON serialization -> Bun protocol. The fixture
// only reflects the payload: it must never normalize or repair base64 input.
func TestGhostToolBridgePreservesObjectParamsAndBindsTheHostConversation(t *testing.T) {
	bun := os.Getenv("WINONE_CHAT_BUN_EXE")
	if bun == "" {
		t.Skip("use build/test-onebot-host.ps1 with the bundled Bun runtime")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "echo.mjs")
	code := `let pending='';process.stdin.on('data',bytes=>{pending+=bytes;let n;while((n=pending.indexOf('\n'))>=0){const r=JSON.parse(pending.slice(0,n));pending=pending.slice(n+1);console.log(JSON.stringify({type:'response',id:r.id,ok:true,value:r.params}));}});process.stdin.on('end',()=>process.exit(0));console.log(JSON.stringify({type:'ready',protocol:1}));`
	if err := os.WriteFile(file, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	runner, err := chat.StartRunner(ctx, bun, file, dir, func(json.RawMessage) {})
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close(ctx)
	store, err := chat.Open(filepath.Join(dir, "profile", "chat"))
	if err != nil {
		t.Fatal(err)
	}
	g := &chatGhosts{runner: runner, control: &chatControl{store: store}}
	conv := chat.Conversation{ID: "owned-session", Directory: dir, Permission: "ask"}
	req := httptest.NewRequest("POST", "http://127.0.0.1/v1/ghost-tool", strings.NewReader(`{"method":"agent-call","params":{"ghostId":"fixture","tool":"echo","args":{"text":"hello"},"session":{"id":"forged"}}}`))
	res := httptest.NewRecorder()
	if !g.toolCall(res, req, conv) || res.Code != 200 {
		t.Fatalf("bridge: %d %s", res.Code, res.Body.String())
	}
	var value struct {
		GhostID, Tool string
		Args          map[string]any
		Session       struct{ ID, WorkingDir string }
	}
	if err := json.Unmarshal(res.Body.Bytes(), &value); err != nil {
		t.Fatalf("not a JSON object: %s: %v", res.Body.String(), err)
	}
	if value.GhostID != "fixture" || value.Tool != "echo" || value.Args["text"] != "hello" || value.Session.ID != "owned-session" || value.Session.WorkingDir != dir {
		t.Fatalf("bridge changed or lost fields: %+v", value)
	}
}

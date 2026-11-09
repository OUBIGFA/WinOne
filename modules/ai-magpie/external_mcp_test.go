package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExternalMCPCommandProbe(t *testing.T) {
	kind := os.Getenv("WINONE_TEST_EXTERNAL_MCP_KIND")
	if kind == "" {
		t.Skip("subprocess fixture only")
	}
	handled, err := runExternalMCPCommand([]string{"mcp", kind})
	if !handled || err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestExternalImageCommandUsesAuthenticatedHostModelRoute(t *testing.T) {
	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-mcp-key" {
			t.Error("image command lost Host gateway credential")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/images/generations" {
			called.Store(true)
			w.WriteHeader(403)
			w.Write([]byte(`{"error":{"message":"fixture gateway policy refused generation"}}`))
			return
		}
		var in struct{ ID json.RawMessage }
		json.NewDecoder(r.Body).Decode(&in)
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": in.ID, "result": map[string]any{}})
	}))
	defer server.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, exe, "-test.run=^TestExternalMCPCommandProbe$")
	command.Env = append(os.Environ(), "WINONE_TEST_EXTERNAL_MCP_KIND=image", "MAGPIE_GATEWAY="+server.URL, "WINONE_AI_MCP_KEY=fixture-mcp-key")
	command.Dir = t.TempDir()
	input,err:=command.StdinPipe();if err!=nil{t.Fatal(err)}
	output,err:=command.StdoutPipe();if err!=nil{t.Fatal(err)}
	if err:=command.Start();err!=nil{t.Fatal(err)}
	t.Cleanup(func(){input.Close();if command.ProcessState==nil{command.Process.Kill();command.Wait()}})
	io.WriteString(input,`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`+"\n")
	decoder:=json.NewDecoder(output);var init json.RawMessage;if err:=decoder.Decode(&init);err!=nil{t.Fatal(err)}
	io.WriteString(input,`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"generate_image","arguments":{"prompt":"fixture"}}}`+"\n")
	var out json.RawMessage;if err:=decoder.Decode(&out);err!=nil{t.Fatal(err)}
	input.Close();if err:=command.Wait();err!=nil{t.Fatal(err)}
	if !called.Load() || !strings.Contains(string(out), "fixture gateway policy refused generation") {
		t.Fatalf("image command did not preserve actual gateway execution/policy: %s", out)
	}
}

func TestExternalBuiltinCommandsUseUpstreamStdioMCP(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"browser", "search", "vision", "image"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture-mcp-key" {
					t.Error("stdio MCP command lost its endpoint credential")
				}
				var in struct{ ID json.RawMessage }
				json.NewDecoder(r.Body).Decode(&in)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": in.ID, "result": map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "serverInfo": map[string]string{"name": "fixture-mcp", "version": "1"}}})
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, exe, "-test.run=^TestExternalMCPCommandProbe$")
			command.Env = append(os.Environ(), "WINONE_TEST_EXTERNAL_MCP_KIND="+kind, "MAGPIE_GATEWAY="+server.URL, "WINONE_AI_MCP_KEY=fixture-mcp-key")
			command.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}` + "\n")
			out, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("stdio command: %v %s", err, out)
			}
			var reply struct {
				Result struct{ ServerInfo struct{ Name string } }
			}
			if err := json.Unmarshal(out, &reply); err != nil || reply.Result.ServerInfo.Name == "" {
				t.Fatalf("stdio initialization was not an MCP response: %s", out)
			}
		})
	}
}

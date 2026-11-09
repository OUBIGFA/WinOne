package aiservices

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestLibraryMCPConfigImport(t *testing.T) {
	for _, tc := range []struct{ input, name, transport, command string }{
		{`{"mcpServers":{"review server":{"command":["node","review.js"],"args":["--check"],"env":{"TOKEN":"${TOKEN}"},}}}`, "review-server", "stdio", "node"},
		{`{"servers":{"remote":{"type":"sse","url":"https://example.invalid/sse"}}}`, "remote", "sse", ""},
		{`"remote": {"serverUrl":"https://example.invalid/mcp"},`, "remote", "http", ""},
		{`{"command":"tool","environment":{"KEY":"value"}}`, "", "stdio", "tool"},
	} {
		rows, err := parseMCPConfigs(tc.input)
		if err != nil || len(rows) != 1 {
			t.Fatalf("parse %s: %v %v", tc.input, rows, err)
		}
		row := rows[0]
		if textOf(row, "name") != tc.name || textOf(row, "transport") != tc.transport || textOf(row, "command") != tc.command {
			t.Fatalf("wrong import: %v", row)
		}
	}
	if _, err := parseMCPConfigs(`{"neither":"server"}`); err == nil {
		t.Fatal("non-server JSON accepted")
	}
	rows, err := parseMCPConfigs(`{"command":"tool","env":{"TEXT":"value,}",},}`)
	if err != nil || textOf(objectOf(rows[0], "env"), "TEXT") != "value,}" {
		t.Fatalf("literal value changed: %v %v", rows, err)
	}
}

func TestLibraryMCPConfigRetainsLiteralFieldsThroughViews(t *testing.T) {
	for _, transport := range []string{"stdio", "http"} {
		for _, source := range []string{"existing", "pasted"} {
			for _, view := range []string{"Form", "JSON", "JSON-Form", "JSON-Form-JSON"} {
				t.Run(transport+"/"+source+"/"+view, func(t *testing.T) {
					_, h, p, _ := setup(t)
					want := map[string]any{"name": "literal", "transport": transport, "agents": []any{"codex"}, "command": "tool", "args": []any{"", " x ", "line one\nline two"}, "env": map[string]any{"TOKEN": " first\nsecond \n", "REF": "${TOKEN}"}}
					if transport == "http" {
						want = map[string]any{"name": "literal", "transport": "http", "url": "https://example.invalid/mcp", "agents": []any{"codex"}, "headers": map[string]any{"Authorization": "  ${TOKEN}  ", "Empty": ""}}
					}
					var received map[string]any
					posts := 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var body struct{ Server map[string]any }
						if r.Method != "POST" || r.URL.Path != "/api/library/servers/save" || json.NewDecoder(r.Body).Decode(&body) != nil {
							http.Error(w, "wrong save", 400)
							return
						}
						posts++
						received = body.Server
						w.Header().Set("Content-Type", "application/json")
						w.Write([]byte(`{"agents":[],"servers":[],"skills":[]}`))
					}))
					defer server.Close()
					x := newExternalPanel(p)
					x.url = server.URL + "/library.html"
					x.begin("servers", want)
					e := x.edit
					if source == "pasted" {
						config := map[string]any{}
						for key, value := range want {
							config[key] = value
						}
						data, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"literal": config}})
						e.serverJSON = string(data)
						if err := e.readServerJSON(); err != nil {
							t.Fatal(err)
						}
					}
					tt := ui.NewTester(func(c *ui.Context) {
						if x.edit != nil {
							ui.Column(c).Fill().Children(func() { x.editorView(c) })
						}
					}, 760, 2400)
					settle(h, tt)
					if view != "Form" {
						for _, tab := range strings.Split(view, "-") {
							label := tab
							if tab == "Form" {
								label = upstreamT("Form")
							}
							if err := tt.Click(label); err != nil {
								t.Fatal(err)
							}
							settle(h, tt)
						}
					}
					if err := tt.Click(upstreamT("Save")); err != nil {
						t.Fatal(err)
					}
					settle(h, tt)
					if posts != 1 {
						t.Fatalf("save failed: posts=%d error=%s", posts, x.err)
					}
					keys := []string{"args", "env"}
					if transport == "http" {
						keys = []string{"headers"}
					}
					for _, key := range keys {
						if !reflect.DeepEqual(received[key], want[key]) {
							t.Errorf("%s changed: got=%#v want=%#v", key, received[key], want[key])
						}
					}
				})
			}
		}
	}
}

func TestLibraryMCPPasteRejectsNormalizedNameCollisionBeforeSaving(t *testing.T) {
	_, h, p, _ := setup(t)
	x := newExternalPanel(p)
	x.begin("servers-import", nil)
	e := x.edit
	const original = `{"mcpServers":{"aaa":{"command":"first"},"two space":{"command":"second"},"two.space":{"command":"third"}}}`
	e.serverJSON = original
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		http.Error(w, "must reject before any request", 400)
	}))
	defer server.Close()
	x.url = server.URL + "/library.html"
	tt := ui.NewTester(func(c *ui.Context) { x.editorView(c) }, 700, 1300)
	settle(h, tt)
	if err := tt.Click(upstreamT("Save")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if posts != 0 || x.err == "" || x.edit != e || e.serverJSON != original {
		t.Fatalf("colliding input was changed or sent: posts=%d err=%s draft=%s", posts, x.err, e.serverJSON)
	}
}

func TestLibraryMCPFormEditKeepsOtherLiteralFields(t *testing.T) {
	_, h, p, _ := setup(t)
	x := newExternalPanel(p)
	x.begin("servers", map[string]any{"name": "literal", "transport": "stdio", "command": "tool", "args": []any{"", " original "}, "env": map[string]any{"TOKEN": "first\nsecond\n"}})
	// Editing the arguments deliberately replaces their old literal array,
	// but an untouched environment value must survive the same save.
	x.edit.values["args"] = "--changed\n${TOKEN}"
	var got struct {
		Server struct {
			Args []string
			Env  map[string]string
		}
	}
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/library/servers/save" || json.NewDecoder(r.Body).Decode(&got) != nil {
			http.Error(w, "wrong save", 400)
			return
		}
		posts++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"agents":[],"servers":[],"skills":[]}`))
	}))
	defer server.Close()
	x.url = server.URL + "/library.html"
	tt := ui.NewTester(func(c *ui.Context) {
		if x.edit != nil {
			x.editorView(c)
		}
	}, 760, 2400)
	settle(h, tt)
	if err := tt.Click("JSON"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if err := tt.Click(upstreamT("Save")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if posts != 1 || !reflect.DeepEqual(got.Server.Args, []string{"--changed", "${TOKEN}"}) || got.Server.Env["TOKEN"] != "first\nsecond\n" {
		t.Fatalf("field edit lost or changed other literals: posts=%d body=%+v error=%s", posts, got, x.err)
	}
}

func TestLibraryMCPJSONPreservesDraftAndVariables(t *testing.T) {
	e := &libraryEditor{values: map[string]string{"name": "original", "transport": "stdio", "command": "tool", "env": "TOKEN=${TOKEN}", "note": "keep note", "clientId": "keep oauth"}, agents: map[string]bool{"codex": true}}
	e.serverJSON = serverConfigJSON(e)
	var config map[string]any
	if err := json.Unmarshal([]byte(e.serverJSON), &config); err != nil {
		t.Fatal(err)
	}
	if err := e.readServerJSON(); err != nil {
		t.Fatal(err)
	}
	if e.values["env"] != "TOKEN=${TOKEN}" || e.values["note"] != "keep note" || !e.agents["codex"] {
		t.Fatalf("roundtrip changed unrelated fields: %v", e.values)
	}
	e.serverJSON = `{"mcpServers":{"new":{"url":"https://example.invalid/mcp"}}}`
	if err := e.readServerJSON(); err != nil {
		t.Fatal(err)
	}
	if e.values["name"] != "new" || e.values["transport"] != "http" || e.values["command"] != "" || e.values["clientId"] != "keep oauth" {
		t.Fatalf("wrong form conversion: %v", e.values)
	}
	e.serverJSON = `{"mcpServers":{"one":{"command":"a"},"two":{"command":"b"}}}`
	if err := e.readServerJSON(); err == nil || e.values["name"] != "new" {
		t.Fatal("multiple servers replaced the current draft")
	}
	e.serverJSON = `{`
	if err := e.readServerJSON(); err == nil || e.values["name"] != "new" {
		t.Fatal("invalid JSON replaced the current draft")
	}
}

func TestLibraryMCPPasteResumesAfterPartialFailure(t *testing.T) {
	_, h, p, _ := setup(t)
	x := newExternalPanel(p)
	x.begin("servers-import", nil)
	e := x.edit
	e.serverJSON = `{"mcpServers":{"one":{"command":"first"},"two":{"url":"https://example.invalid/mcp"}}}`
	names := []string{}
	failed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Old    string
			Server map[string]any
		}
		if r.URL.Path != "/api/library/servers/save" || json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		name := textOf(body.Server, "name")
		names = append(names, name)
		if name == "two" && !failed {
			failed = true
			http.Error(w, "fixture write failed", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"agents":[],"servers":[],"skills":[],"result":{"changed":["saved"]}}`))
	}))
	defer server.Close()
	x.url = server.URL + "/library.html"
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Children(func() { x.editorView(c) }) }, 700, 1300)
	tt.Frame()
	if err := tt.Click(upstreamT("Save")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if x.edit != e || x.err == "" || len(names) != 2 {
		t.Fatalf("partial failure lost: draft=%v err=%s names=%v", x.edit, x.err, names)
	}
	left, err := parseMCPConfigs(e.serverJSON)
	if err != nil || len(left) != 1 || textOf(left[0], "name") != "two" {
		t.Fatalf("saved server remains in retry draft: %v %v", left, err)
	}
	// View the live panel because a successful save closes the editor.
	tt = ui.NewTester(func(c *ui.Context) {
		if x.edit != nil {
			ui.Column(c).Fill().Children(func() { x.editorView(c) })
		}
	}, 700, 1300)
	tt.Frame()
	if err := tt.Click(upstreamT("Save")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if x.edit != nil || x.err != "" || len(names) != 3 || names[2] != "two" {
		t.Fatalf("retry repeated a saved server: %v err=%s", names, x.err)
	}
}

package aiservices

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
)

func TestLibraryCustomMarketUsesServerForm(t *testing.T) {
	for _, tc := range []struct {
		name, query, custom, transport, endpoint, command string
		args                                              []string
	}{
		{"HTTP", "https://fixture.invalid/mcp", `{"name":"fixture","transport":"http","url":"https://fixture.invalid/mcp"}`, "http", "https://fixture.invalid/mcp", "", nil},
		{"npm", "https://www.npmjs.com/package/@fixture/mcp", `{"name":"mcp","transport":"stdio","command":"npx","args":["-y","@fixture/mcp"]}`, "stdio", "", "npx", []string{"-y", "@fixture/mcp"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, h, p, _ := setup(t)
			i18n.SetLocale(i18n.English)
			defer i18n.SetLocale(i18n.Chinese)
			var reads, saves int
			var saved struct {
				Old    string
				Server struct {
					Name, Transport, URL, Command string
					Args, Agents                  []string
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == "GET" && r.URL.Path == "/session/key/api/library/market/servers" && r.URL.Query().Get("q") == tc.query:
					reads++
					w.Write([]byte(`{"items":[],"custom":` + tc.custom + `}`))
				case r.Method == "POST" && r.URL.Path == "/session/key/api/library/servers/save":
					if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
						http.Error(w, err.Error(), 400)
						return
					}
					saves++
					w.Write([]byte(`{"agents":[],"servers":[],"skills":[]}`))
				default:
					http.Error(w, "unexpected library action", 400)
				}
			}))
			defer server.Close()
			x := newExternalPanel(p)
			x.url = server.URL + "/session/key/library.html"
			x.marketQuery = tc.query
			if err := json.Unmarshal([]byte(`{"agents":[{"id":"codex","name":"Codex","mcp":"fixture"},{"id":"missing","name":"Missing","mcp":"fixture","gone":true},{"id":"no-mcp","name":"No MCP"}]}`), &x.library); err != nil {
				t.Fatal(err)
			}
			tt := ui.NewTester(func(c *ui.Context) {
				if x.edit == nil {
					x.marketView(c)
				} else {
					x.editorView(c)
				}
			}, 700, 2200)
			settle(h, tt)
			if err := tt.Click(externalT("market")); err != nil {
				t.Fatal(err)
			}
			settle(h, tt)
			if err := tt.Click(upstreamT("Add to the library")); err != nil {
				t.Fatal(err)
			}
			settle(h, tt)
			if x.edit == nil || x.edit.kind != "servers" || x.edit.old != "" {
				t.Fatalf("custom server did not open a new server form: %+v", x.edit)
			}
			if err := tt.Click(upstreamT("Save")); err != nil {
				t.Fatal(err)
			}
			settle(h, tt)
			if reads != 1 || saves != 1 || saved.Old != "" || saved.Server.Transport != tc.transport || saved.Server.URL != tc.endpoint || saved.Server.Command != tc.command || !reflect.DeepEqual(saved.Server.Args, tc.args) || !reflect.DeepEqual(saved.Server.Agents, []string{"codex"}) {
				t.Fatalf("custom server was not saved through its editable form: reads=%d saves=%d body=%+v error=%s", reads, saves, saved, x.err)
			}
		})
	}
}

func TestLibraryRTKBlockedHookCanBeDisabled(t *testing.T) {
	_, h, p, _ := setup(t)
	i18n.SetLocale(i18n.English)
	defer i18n.SetLocale(i18n.Chinese)
	var posts int
	var changed struct {
		Agent string
		On    bool
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/session/key/api/library/rtk" || json.NewDecoder(r.Body).Decode(&changed) != nil {
			http.Error(w, "unexpected RTK action", 400)
			return
		}
		posts++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"agents":[{"id":"opencode","name":"Existing hook","on":false,"blocked":"not supported"}]}`))
	}))
	defer server.Close()
	x := newExternalPanel(p)
	x.url = server.URL + "/session/key/library.html"
	if err := json.Unmarshal([]byte(`{"agents":[{"id":"opencode","name":"Existing hook","on":true,"blocked":"not supported"},{"id":"new","name":"Blocked new hook","on":false,"blocked":"not supported"},{"id":"wsl","name":"Sleeping distro","asleep":true}]}`), &x.rtk); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { x.rtkView(c) }, 700, 1200)
	settle(h, tt)
	for _, label := range []string{"Blocked new hook", "Sleeping distro"} {
		_ = tt.Click(label)
		settle(h, tt)
		if posts != 0 {
			t.Fatalf("disabled RTK row changed configuration: %s", label)
		}
	}
	if err := tt.Click("Existing hook"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if posts != 1 || changed.Agent != "opencode" || changed.On {
		t.Fatalf("blocked existing hook could not be disabled: posts=%d action=%+v", posts, changed)
	}
}

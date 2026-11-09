package aiservices

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
	"winone/desktopmygo/i18n"
)

func TestExternalSessionsReadManagementAndConfirmDeletion(t *testing.T) {
	_, h, p, _ := setup(t)
	i18n.SetLocale(i18n.English)
	defer i18n.SetLocale(i18n.Chinese)
	var reads, deletes int
	var request struct {
		Agent string
		IDs   []string
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/session/key/api/sessions/manage":
			reads++
			w.Write([]byte(`{"agent":"codex","agents":[{"agent":"codex","name":"Codex","count":1,"deletable":true}],"sessions":[{"id":"one","agent":"codex","title":"Fixture conversation","cwd":"C:/fixture","deletable":true}],"trash":[],"reads":{"asked":true}}`))
		case "/session/key/api/sessions/delete":
			deletes++
			_ = json.NewDecoder(r.Body).Decode(&request)
			w.Write([]byte(`{"deleted":["one"],"refused":[]}`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()
	x := newExternalPanel(p)
	x.url = server.URL + "/session/key/library.html"
	x.readSessions()
	tt := ui.NewTester(func(c *ui.Context) { x.view(c, "sessions") }, 1000, 1800)
	settle(h, tt)
	if reads != 1 || !tt.HasText("Fixture conversation") {
		t.Fatalf("management data not shown: reads=%d, error=%s", reads, x.err)
	}
	if err := tt.Click(upstreamT("Delete")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if deletes != 0 || x.confirm == nil {
		t.Fatal("delete must wait for explicit confirmation")
	}
	if err := tt.Click(upstreamT("Confirm")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if deletes != 1 || request.Agent != "codex" || len(request.IDs) != 1 || request.IDs[0] != "one" {
		t.Fatalf("wrong deletion scope: %+v, calls=%d", request, deletes)
	}
}

func TestExternalSessionScreens(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("explicit native snapshots")
	}
	_, h, p, _ := setup(t)
	x := newExternalPanel(p)
	x.url = "http://127.0.0.1:9/session/fixture/library.html"
	x.sessions.reads = map[string]any{"asked": true}
	x.sessions.agent = "codex"
	x.sessions.picked = map[string]bool{}
	x.sessions.agents = []sessionAgent{{Agent: "codex", Name: "Codex", Count: 2, Deletable: true}, {Agent: "claude", Name: "Claude Code", Count: 3, Deletable: true}}
	at := time.Date(2026, 11, 9, 10, 30, 0, 0, time.Local)
	x.sessions.rows = []externalSession{{ID: "first", Agent: "codex", Title: "Find the startup failure", Cwd: "C:/Projects/example", Last: at, Messages: 12, Input: 1000, Output: 500, Cost: 0.02, Deletable: true, Resume: "codex resume first"}, {ID: "second", Agent: "codex", Title: "Review a long conversation title with a complete project path", Cwd: "C:/Projects/example", Last: at, Messages: 18, Input: 3000, Output: 2000, Cost: 0.08, Deletable: true}}
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		p.external = x
		p.tab = tabSessions
		h.Shots(t, "external-sessions-"+locale, p.View, 1100, nil)
	}
	i18n.SetLocale(i18n.Chinese)
}

func TestExternalSessionTrashRestoresSelectedKey(t *testing.T) {
	_, h, p, _ := setup(t)
	i18n.SetLocale(i18n.English)
	defer i18n.SetLocale(i18n.Chinese)
	var key string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/session/key/api/sessions/restore" {
			var body struct{ Key string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			key = body.Key
			w.Write([]byte(`{"agent":"codex","id":"one"}`))
			return
		}
		w.Write([]byte(`{"agent":"codex","agents":[],"sessions":[],"trash":[],"reads":{"asked":true}}`))
	}))
	defer server.Close()
	x := newExternalPanel(p)
	x.url = server.URL + "/session/key/library.html"
	x.sessions.reads = map[string]any{"asked": true}
	x.sessions.trashOn = true
	x.sessions.trash = []sessionTrash{{Key: "codex/one", Title: "Fixture deleted"}}
	tt := ui.NewTester(func(c *ui.Context) { x.sessionsView(c) }, 900, 1000)
	tt.Frame()
	if err := tt.Click(upstreamT("Restore")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if key != "codex/one" {
		t.Fatalf("restore lost trash identity: %q", key)
	}
}

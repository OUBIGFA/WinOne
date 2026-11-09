package aiservices

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"os"
	"strings"
	"testing"
	"winone/desktopmygo/i18n"
)

// Review the actual Library views with representative data after an explicit
// read. No client files or external services are needed for this layout matrix.
func TestLibraryScreens(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("explicit visual inspection")
	}
	defer i18n.SetLocale(i18n.Chinese)
	_, h, p, _ := setup(t)
	x := libraryScreenFixture(t, p)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for tab := range libraryTabs {
			x.tab = tab
			h.Shots(t, fmt.Sprintf("library-%s-%d", locale, tab), func(c *ui.Context) {
				ui.Column(c).FillWidth().Gap(18).Padding(20).Children(func() { x.libraryView(c) })
			}, 1100, nil)
		}
		x.begin("servers", rowsOf(x.library, "servers")[0])
		h.Shots(t, "library-editor-"+locale, func(c *ui.Context) {
			ui.Column(c).FillWidth().Gap(18).Padding(20).Children(func() { x.editorView(c) })
		}, 1100, nil)
		x.edit.values["transport"] = "http"
		x.edit.values["url"] = "https://example.invalid/mcp"
		h.Shots(t, "library-http-"+locale, func(c *ui.Context) {
			ui.Column(c).FillWidth().Gap(18).Padding(20).Children(func() { x.editorView(c) })
		}, 1500, nil)
		x.edit.serverView = 1
		x.edit.serverJSON = serverConfigJSON(x.edit)
		h.Shots(t, "library-json-"+locale, func(c *ui.Context) {
			ui.Column(c).FillWidth().Gap(18).Padding(20).Children(func() { x.editorView(c) })
		}, 1100, nil)
		x.edit = nil
	}
}

func libraryScreenFixture(t *testing.T, p *page) *externalPanel {
	t.Helper()
	x := newExternalPanel(p)
	if err := json.Unmarshal([]byte(`{"agents":[{"id":"codex","name":"Codex"},{"id":"claude","name":"Claude Code"}],"instructions":{"sets":[{"id":"default","name":"Default","active":true,"text":"Keep changes within the selected project."}],"agents":[{"agent":"codex","name":"Codex","path":"C:/Projects/example/AGENTS.md","on":true}]},"servers":[{"name":"Project search","description":"Search files in the selected workspace","transport":"stdio","command":"project-search","on":true}],"skills":[{"name":"Review changes","description":"Check changes and explain findings","source":"owner/project","agents":["codex"]}],"hooks":[{"name":"After edit","event":"PostToolUse","command":"check-files"}],"subagents":[{"name":"Reviewer","description":"Review changes in the current project"}],"projects":[]}`), &x.library); err != nil {
		t.Fatal(err)
	}
	x.rtk = map[string]any{"installed": false}
	return x
}

// Opening either native page must leave all external client discovery dormant.
func TestExternalPagesWaitForExplicitRead(t *testing.T) {
	_, h, p, _ := setup(t)
	x := newExternalPanel(p)
	for _, kind := range []string{"library", "sessions"} {
		tt := ui.NewTester(func(c *ui.Context) { x.view(c, kind) }, 900, 1500)
		settle(h, tt)
		if x.url != "" || x.busy() {
			t.Fatal("page opened an external worker without a click")
		}
		if !strings.Contains(strings.Join(tt.Texts(), " "), externalT("read")) {
			t.Fatal("missing explicit read control")
		}
	}
}

func TestLibraryInvalidReceiptPreservesEditor(t *testing.T) {
	_, _, p, _ := setup(t)
	x := newExternalPanel(p)
	x.edit = &libraryEditor{kind: "servers", values: map[string]string{"name": "unsaved"}, original: map[string]any{}}
	x.acceptLibrary(json.RawMessage(`{}`))
	if x.edit == nil || x.err == "" {
		t.Fatal("invalid upstream receipt discarded draft")
	}
}

func TestExternalSessionFilterAndPagination(t *testing.T) {
	x := externalSessions{query: "alpha", agent: "codex", limit: 50, rows: []externalSession{
		{ID: "one", Agent: "codex", Title: "Alpha", Cwd: "C:/work"},
		{ID: "two", Agent: "claude", Title: "Alpha", Cwd: "C:/work"},
		{ID: "three", Agent: "codex", Title: "Beta", Cwd: "C:/work"},
	}}
	got := x.filtered()
	if len(got) != 1 || got[0].ID != "one" {
		t.Fatalf("wrong visible sessions: %+v", got)
	}
}

func TestLibraryServerFormFollowsTransport(t *testing.T) {
	_, h, p, _ := setup(t)
	x := newExternalPanel(p)
	x.library = map[string]any{}
	x.begin("servers", map[string]any{"name": "Example", "transport": "stdio", "command": "example"})
	tt := ui.NewTester(func(c *ui.Context) { x.editorView(c) }, 560, 2400)
	settle(h, tt)
	if !tt.HasText(upstreamT("Command")) || tt.HasText(upstreamT("URL")) || tt.HasText(upstreamT("Client ID")) {
		t.Fatalf("stdio form contains remote-only fields: %q", tt.Texts())
	}
	x.edit.values["transport"] = "http"
	settle(h, tt)
	if tt.HasText(upstreamT("Command")) || !tt.HasText(upstreamT("URL")) || !tt.HasText(upstreamT("Client ID")) {
		t.Fatalf("HTTP form is missing remote fields: %q", tt.Texts())
	}
	if x.edit.values["command"] != "example" {
		t.Fatal("changing transport lost the command draft")
	}
}

func TestLibraryNewServerAndDiscoveredSkills(t *testing.T) {
	_, h, p, _ := setup(t)
	x := newExternalPanel(p)
	if err := json.Unmarshal([]byte(`{"agents":[{"id":"codex","name":"Codex","mcp":"codex"},{"id":"missing","name":"Missing","mcp":"missing","gone":true}],"foundSkills":[],"newSkills":[{"name":"New review skill"}]}`), &x.library); err != nil {
		t.Fatal(err)
	}
	x.begin("servers", nil)
	tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Children(func() { x.editorView(c) }) }, 560, 2400)
	settle(h, tt)
	if !tt.HasText(upstreamT("Command")) || x.edit.values["transport"] != "stdio" {
		t.Fatal("new server does not start with the upstream command transport")
	}
	if !x.edit.agents["codex"] || x.edit.agents["missing"] {
		t.Fatal("new server has incorrect client defaults")
	}
	x.edit = nil
	tt = ui.NewTester(func(c *ui.Context) { x.foundView(c, "skills") }, 800, 600)
	settle(h, tt)
	if !tt.HasText("New review skill") {
		t.Fatal("new skills disappear when there are no found skills")
	}
}

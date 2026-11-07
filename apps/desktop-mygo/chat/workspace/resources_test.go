package workspace

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func TestResourcePaneUsesActualDiscoveryAndKeepsFailuresVisible(t *testing.T) {
	var failed atomic.Bool
	h := testhost.New(t, func(method string, raw json.RawMessage) (any, error) {
		var input map[string]string
		_ = json.Unmarshal(raw, &input)
		if method != "chat.resources" || input["id"] != "owned" {
			return nil, fmt.Errorf("unexpected resource scope")
		}
		if failed.Load() {
			return nil, fmt.Errorf("DISCOVERY_FAILED")
		}
		return json.RawMessage(`{"agents":[{"path":"C:\\work\\AGENTS.md"}],"skills":[{"name":"Example Skill","description":"Keeps reports consistent","path":"C:\\work\\skills\\example\\SKILL.md"}],"plugins":[{"name":"Local extension","path":"C:\\extensions\\local","enabled":false}],"mcpServers":[{"name":"Documents","enabled":true,"status":"connected"}],"diagnostics":["One optional resource is unavailable"]}`), nil
	})
	opened := ""
	p := NewResourcePane(h.Env, ResourceOptions{Prefix: "chat.", SessionID: "owned", OnOpenFile: func(path string) { opened = path }})
	t.Cleanup(p.Close)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.View(c) }, 500, 750)
	for range 3 {
		h.Settle(tt)
	}
	for _, text := range []string{"Example Skill", "Local extension", "Documents", "One optional resource is unavailable"} {
		if !tt.HasText(text) {
			t.Fatalf("discovered resource missing: %s; %v", text, tt.Texts())
		}
	}
	if err := tt.Click("Example Skill"); err != nil {
		t.Fatal(err)
	}
	if opened != `C:\work\skills\example\SKILL.md` {
		t.Fatalf("resource navigation changed its path: %q", opened)
	}
	failed.Store(true)
	p.Refresh()
	for range 3 {
		h.Settle(tt)
	}
	if p.err == "" || !tt.HasText("Example Skill") {
		t.Fatal("failed discovery erased the last known inventory or hid its error")
	}
}
func TestResourcePaneWithoutConversationDoesNotScanTheMachine(t *testing.T) {
	var calls atomic.Int32
	h := testhost.New(t, func(string, json.RawMessage) (any, error) { calls.Add(1); return nil, fmt.Errorf("unexpected scan") })
	p := NewResourcePane(h.Env, ResourceOptions{Prefix: "chat."})
	t.Cleanup(p.Close)
	tt := ui.NewTester(func(c *ui.Context) { p.View(c) }, 500, 500)
	h.Settle(tt)
	if calls.Load() != 0 {
		t.Fatal("resource pane scanned without a conversation scope")
	}
}

package webui

import (
	"encoding/json"
	"strings"
	"testing"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
)

func TestTabsShowWebPagesOnly(t *testing.T) {
	for _, address := range []string{"https://example.com/path?q=a#b", "http://127.0.0.1:8080/preview.html", "http://localhost:5173/", "about:blank"} {
		if got, err := navigationURL(address); err != nil || got != address {
			t.Errorf("%s: %q, %v", address, got, err)
		}
	}
	// Local files, scripts, the window's own pages and schemes (served at
	// <scheme>.localhost by WebView2) and credentials are refused, as the
	// Rust window refused them.
	for _, address := range []string{"file:///C:/Users/private.txt", "javascript:alert(1)", "data:text/html,test",
		"http://mygo.localhost/chat-embed.html", "http://winone.localhost/index.html", "http://winone-web.localhost/post/x",
		"https://user:password@example.com", "", "https://exa\nmple.com"} {
		if _, err := navigationURL(address); err == nil {
			t.Errorf("%q allowed", address)
		}
	}
}

func TestTabIdentityKeepsSessionPathsOut(t *testing.T) {
	if key, err := tabKey(browserPayload{SessionID: "chat-one", TabID: "browser:1234"}); err != nil || key != "chat-one/browser:1234" {
		t.Fatalf("%q, %v", key, err)
	}
	for _, p := range []browserPayload{{SessionID: "../chat", TabID: "browser:1"}, {SessionID: "chat", TabID: "a/b"}, {SessionID: "chat:1", TabID: "x"}, {SessionID: "", TabID: "x"}} {
		if _, err := tabKey(p); err == nil {
			t.Errorf("%+v accepted", p)
		}
	}
}

func TestBoundsRejectInvalidGeometry(t *testing.T) {
	if !(rect{10, 20, 200, 300}).valid() {
		t.Fatal("valid bounds refused")
	}
	for _, r := range []rect{{0, 0, 0, 10}, {-1, 0, 10, 10}, {0, 0, 40000, 10}} {
		if r.valid() {
			t.Errorf("%+v accepted", r)
		}
	}
}

func TestAPopoverOverThePageLeavesItsLargestSide(t *testing.T) {
	page := rect{10, 20, 400, 300}
	// A comment editor in the lower right quarter: the page shows above
	// it, across the whole width.
	got := beside(page, rect{200, 200, 150, 80})
	if want := (rect{10, 20, 400, 200}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// Covering everything leaves nothing to show.
	if got := beside(page, rect{0, 0, 400, 300}); got.Width*got.Height != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestPagesSendOnlyCommentsAndPanelKeys(t *testing.T) {
	var sent []map[string]any
	b := &Browser{tabs: map[string]*browserTab{}}
	tab := &browserTab{state: BrowserState{SessionID: "s", TabID: "t"}, commenting: true}
	tell := func(text string) {
		before := tab.commenting
		b.receive(tab, text)
		sent = append(sent, map[string]any{"commenting": before})
	}
	// receive applies what a message means for the tab.
	tell(`{"kind":"command","value":"reload"}`)
	tell(`{"kind":"command","value":"eval"}`)
	tell(`{"kind":"comment","channel":"browser-comment:mode-exited"}`)
	if tab.commenting {
		t.Fatal("leaving comment mode was not noted")
	}
	tab.commenting = true
	tell(`{"kind":"comment","channel":"browser-comment:enter-mode"}`)
	if !tab.commenting {
		t.Fatal("a page's own command changed the mode")
	}
	if len(sent) != 4 {
		t.Fatal(sent)
	}
}

func TestLinksOfPagesGoThroughTheShell(t *testing.T) {
	var opened []core.Intent
	var pages []string
	env := &core.Env{
		Open:     func(page string, intent core.Intent) { pages = append(pages, page); opened = append(opened, intent) },
		Navigate: func(page string, _ func()) { pages = append(pages, page) },
	}
	Navigate(env, json.RawMessage(`{"to":"conversation","id":"c1","file":{"path":"E:/a.md","line":4}}`))
	Navigate(env, json.RawMessage(`{"to":"conversation","id":"c2","file":{"path":"a\u0000b"}}`))
	Navigate(env, json.RawMessage(`{"to":"page","id":"chat","section":"settings"}`))
	Navigate(env, json.RawMessage(`{"to":"page","id":"ai-services"}`))
	Navigate(env, json.RawMessage(`{"to":"page","id":"settings"}`))
	Navigate(env, json.RawMessage(`{"to":"conversation"}`))
	Navigate(env, json.RawMessage(`{"to":"new-chat","text":"Diagnose"}`))
	if strings.Join(pages, ",") != "chat,chat,chat,ai-services,chat" {
		t.Fatalf("pages %v", pages)
	}
	if opened[0].Conversation != "c1" || opened[0].File == nil || opened[0].File.Line != 4 || opened[1].File != nil || !opened[2].ChatSettings || !opened[3].NewChat || opened[3].Prefill != "Diagnose" {
		t.Fatalf("intents %+v", opened)
	}
}

func TestExportsSaveOnlyWhereTheWindowChose(t *testing.T) {
	var receipt any = map[string]any{"status": "ok", "filePath": "", "fidelity": "partial"}
	h := testhost.New(t, func(method string, params json.RawMessage) (any, error) { return receipt, nil })
	// A page never names the destination.
	if _, err := ExportTask(h.Env, json.RawMessage(`{"id":"task","targetPath":"x.cshare"}`)); err == nil {
		t.Fatal("a page's destination was accepted")
	}
	// Cancelling the dialog writes nothing.
	out, err := ExportTask(h.Env, json.RawMessage(`{"id":"task"}`))
	if err != nil || string(out) != `{"status":"canceled"}` || len(h.Calls()) != 0 {
		t.Fatalf("%s %v %v", out, err, h.Calls())
	}
	h.Picked = map[string]any{i18n.T("chat.exportTitle"): `C:\out\task.cshare`}
	receipt = map[string]any{"status": "ok", "filePath": `C:\elsewhere.cshare`, "fidelity": "full"}
	if _, err := ExportTask(h.Env, json.RawMessage(`{"id":"task","password":"secret"}`)); err == nil {
		t.Fatal("a receipt for another file was accepted")
	}
	receipt = map[string]any{"status": "ok", "filePath": `C:\out\task.cshare`, "fidelity": "full"}
	if _, err := ExportTask(h.Env, json.RawMessage(`{"id":"task","excludeMedia":true}`)); err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	calls := h.Called("chat.export")
	if json.Unmarshal(calls[len(calls)-1].Params, &sent) != nil || sent["targetPath"] != `C:\out\task.cshare` || sent["excludeMedia"] != true {
		t.Fatalf("sent %v", sent)
	}
}

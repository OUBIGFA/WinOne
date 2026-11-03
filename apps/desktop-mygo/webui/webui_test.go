package webui

import "testing"

func TestTheBridgeAllowsWhatTheRustWindowAllowed(t *testing.T) {
	for _, method := range []string{"file.preview", "terminal.poll", "review.turn", "automations.list", "onebot.chat.poll", "onebot.bots.list", "onebot.voice.anything"} {
		if !chatAllowed(method) {
			t.Errorf("%s refused", method)
		}
	}
	for _, method := range []string{"", "secret.read", "onebot.chat.secret", "onebot.unknown", "settings.unknown"} {
		if chatAllowed(method) {
			t.Errorf("%s allowed", method)
		}
	}
}

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
	b.surface = nil
	tell := func(text string) {
		before := tab.commenting
		b.receive(tab, text)
		sent = append(sent, map[string]any{"commenting": before})
	}
	// Without a surface nothing is dispatched; receive still applies what
	// a message means for the tab.
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

package flashchat

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/flashchat"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

// fakeHost answers the flash chat's Host calls: the nickname, file
// handles and the transfer tasks it starts.
type fakeHost struct {
	mu       sync.Mutex
	nickname string
	n        int
	tasks    map[string]map[string]any
	stopped  []string
	started  []map[string]any
	failGet  bool
}

func (f *fakeHost) handle(method string, params json.RawMessage) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var args map[string]any
	_ = json.Unmarshal(params, &args)
	switch method {
	case "interconnect.chat.preferences.get":
		if f.failGet {
			return nil, testhost.Error{Code: "failed", Message: "preferences unreadable"}
		}
		return map[string]string{"nickname": f.nickname}, nil
	case "interconnect.chat.preferences.save":
		f.nickname = args["nickname"].(string)
		return map[string]string{"nickname": f.nickname}, nil
	case "interconnect.transfer_files":
		var files []map[string]any
		for _, p := range args["paths"].([]any) {
			path := p.(string)
			files = append(files, map[string]any{"path": path, "name": filepath.Base(path), "size": 3, "type": "application/octet-stream"})
		}
		return files, nil
	case "interconnect.transfer_start":
		f.n++
		id := fmt.Sprintf("task-%d", f.n)
		state := map[string]any{"status": "running", "ready": true}
		if args["mode"] == "share_server" {
			state["relayReady"], state["code"] = true, "tc-"+id
		} else {
			state["address"] = "127.0.0.1:5000"
		}
		f.tasks[id] = map[string]any{"id": id, "spec": map[string]any{"mode": args["mode"]}, "state": state}
		f.started = append(f.started, args)
		return map[string]string{"id": id}, nil
	case "interconnect.transfer_list":
		list := []map[string]any{}
		for _, task := range f.tasks {
			list = append(list, task)
		}
		return list, nil
	case "interconnect.transfer_stop":
		id := args["id"].(string)
		delete(f.tasks, id)
		f.stopped = append(f.stopped, id)
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected %s", method)
}

func (f *fakeHost) stoppedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stopped...)
}

func (f *fakeHost) starts() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.started...)
}

type fixture struct {
	host *fakeHost
	h    *testhost.Harness
	p    *page
	tt   *ui.Tester
}

func newFixture(t *testing.T, network *flashchat.MemoryNetwork, peer, nickname string, width, height float32, dark bool) *fixture {
	f := &fixture{host: &fakeHost{nickname: nickname, tasks: map[string]map[string]any{}}}
	f.h = testhost.New(t, f.host.handle)
	f.p = newPage(f.h.Env)
	f.p.join = func(o flashchat.JoinOptions) (*flashchat.Session, error) {
		o.Transport = network.Join(peer)
		return flashchat.Join(o)
	}
	view := func(c *ui.Context) {
		c.SetTheme(kit.Theme(c.Preferences(), dark))
		ui.Column(c).Fill().Background(c.Theme().Background).Children(func() { f.p.View(c) })
	}
	f.tt = ui.NewTester(view, int(width), int(height))
	f.tt.SetDark(dark)
	f.settle()
	t.Cleanup(f.p.Close)
	return f
}

func (f *fixture) settle() {
	for range 3 {
		f.h.Settle(f.tt)
	}
}

// until settles the page until ok holds, as network events come from
// goroutines the harness does not track.
func (f *fixture) until(t *testing.T, label string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		f.settle()
		if ok() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; texts %q; error %q", label, f.tt.Texts(), f.p.err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (f *fixture) click(t *testing.T, label string) {
	t.Helper()
	if err := f.tt.Click(label); err != nil {
		t.Fatalf("click %q: %v; texts %q", label, err, f.tt.Texts())
	}
	f.settle()
}

func shot(t *testing.T, f *fixture, name string) {
	dir := os.Getenv("WINONE_SHOTS")
	if dir == "" {
		return
	}
	f.tt.Frame()
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, f.tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestLobbyValidatesBeforeJoining(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	f := newFixture(t, flashchat.NewMemoryNetwork(), "peer-a", "Ada", 1000, 760, false)
	if f.p.nickname != "Ada" || !f.tt.HasText(tr("chatWelcome")) {
		t.Fatalf("lobby %q", f.tt.Texts())
	}
	f.p.tracker = "ws://example.com/announce"
	f.click(t, tr("createRoom"))
	if f.p.session != nil || !strings.Contains(f.p.err, "ws://example.com/announce") {
		t.Fatalf("tracker accepted: %q", f.p.err)
	}
	f.p.tracker, f.p.rtc = "", "[1]"
	f.click(t, tr("createRoom"))
	if f.p.session != nil || !strings.Contains(f.p.err, "JSON") {
		t.Fatalf("RTC configuration accepted: %q", f.p.err)
	}
	// Joining needs both the id and the password.
	f.p.rtc = ""
	f.click(t, tr("joinRoom"))
	f.p.roomID = "room"
	f.settle()
	f.click(t, tr("joinRoom"))
	if f.p.session != nil {
		t.Fatal("joined without a password")
	}
	f.click(t, tr("backToEntry"))
	if f.p.joining {
		t.Fatal("back did not return")
	}
	// The network settings open from the lobby.
	f.click(t, tr("networkOptions"))
	if !f.tt.HasText(tr("chatNetworkHint")) {
		t.Fatalf("network settings %q", f.tt.Texts())
	}
	shot(t, f, "zh-1000-light-network")
}

func TestNicknameMustLoadFirst(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	host := &fakeHost{failGet: true, tasks: map[string]map[string]any{}}
	h := testhost.New(t, host.handle)
	p := newPage(h.Env)
	tt := ui.NewTester(p.View, 1000, 760)
	h.Settle(tt)
	if p.nickReady || !tt.HasText("preferences unreadable") || !tt.HasText(tr("retry")) {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click(tr("createRoom")); err == nil {
		h.Settle(tt)
	}
	if p.session != nil {
		t.Fatal("created a room before the nickname loaded")
	}
	host.mu.Lock()
	host.failGet, host.nickname = false, "Ada"
	host.mu.Unlock()
	if err := tt.Click(tr("retry")); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if !p.nickReady || p.nickname != "Ada" {
		t.Fatalf("after retry %v %q", p.nickReady, p.nickname)
	}
}

func peersOf(p *page) int {
	if p.session == nil {
		return -1
	}
	return len(p.session.Group().Peers())
}

func TestTwoPagesTalkShareAndLeave(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	network := flashchat.NewMemoryNetwork()
	a := newFixture(t, network, "peer-a", "Ada", 1000, 760, false)
	b := newFixture(t, network, "peer-b", "", 1000, 760, false)
	shot(t, a, "zh-1000-light-lobby")

	a.p.roomName = "周会"
	a.click(t, tr("createRoom"))
	a.until(t, "the room", func() bool { return a.p.session != nil })
	if dirty, _ := a.h.Env.Activity.State(); !dirty {
		t.Fatal("a joined room does not ask before leaving the page")
	}
	if a.host.nickname != "Ada" || !a.tt.HasText("周会") || !a.tt.HasText(tr("chatSoloHint")) {
		t.Fatalf("workspace %q", a.tt.Texts())
	}
	shot(t, a, "zh-1000-light-alone")

	// A message sent alone reaches later members as history.
	a.p.drafts[""] = "早上好"
	a.settle()
	a.click(t, tr("sendMessage"))
	if a.p.drafts[""] != "" || !a.tt.HasText("早上好") {
		t.Fatalf("sent %q; texts %q", a.p.drafts[""], a.tt.Texts())
	}

	b.click(t, tr("joinRoom"))
	b.p.roomID, b.p.password = a.p.session.RoomID, a.p.session.Password
	b.settle()
	b.click(t, tr("joinRoom"))
	b.until(t, "b to meet a", func() bool { return peersOf(b.p) == 1 })
	a.until(t, "a to meet b", func() bool { return peersOf(a.p) == 1 })
	b.until(t, "the history", func() bool { return b.tt.HasText("早上好") })
	// b chose no nickname: its peers see the default one.
	if !a.tt.HasText(flashchat.DisplayName(flashchat.DefaultNickname, b.p.session.UserID)) || !a.tt.HasText(tr("verified")) {
		t.Fatalf("a's list %q", a.tt.Texts())
	}

	// A group message, then a private one.
	b.p.drafts[""] = "**你好**，大家"
	b.settle()
	b.click(t, tr("sendMessage"))
	a.until(t, "the group message", func() bool { return a.tt.HasText("你好") })
	shot(t, a, "zh-1000-light-group")
	bName := flashchat.DisplayName(flashchat.DefaultNickname, b.p.session.UserID)
	a.click(t, tr("chatPrivate")+" · "+bName)
	if a.p.selected == "" {
		t.Fatal("the private conversation did not open")
	}
	a.p.drafts[a.p.selected] = "只给你"
	a.settle()
	a.click(t, tr("sendMessage"))
	aName := flashchat.DisplayName("Ada", a.p.session.UserID)
	b.click(t, tr("chatPrivate")+" · "+aName)
	b.until(t, "the private message", func() bool { return b.tt.HasText("只给你") })
	b.click(t, tr("groupRoom"))
	if b.tt.HasText("只给你") {
		t.Fatal("the private message shows in the group")
	}
	a.until(t, "the private conversation", func() bool { return a.tt.HasText("只给你") })
	shot(t, a, "zh-1000-light-private")

	// a offers a file in the group; b browses it through its Host.
	a.click(t, tr("groupRoom"))
	a.h.Picked[tr("shareInChat")] = []string{`C:\interop\report.pdf`}
	a.click(t, tr("shareInChat"))
	a.until(t, "a's offer", func() bool { return a.tt.HasText(tr("stopChatShare")) })
	if s := a.host.starts(); len(s) != 1 || s[0]["mode"] != "share_server" {
		t.Fatalf("starts %v", s)
	}
	b.until(t, "the offer", func() bool { return b.tt.HasText(tr("browseShare")) })
	b.click(t, tr("browseShare"))
	b.until(t, "the share page", func() bool { return b.p.browsing })
	if s := b.host.starts(); len(s) != 1 || s[0]["mode"] != "share_client" || s[0]["code"] != "tc-task-1" || s[0]["listen"] != "127.0.0.1:0" {
		t.Fatalf("receive %v", s)
	}
	b.p.closeBrowse()

	// a leaves: its share stops, and b waits again.
	a.p.leave()
	a.until(t, "a's lobby", func() bool { return a.p.session == nil && !a.p.busy })
	if stopped := a.host.stoppedIDs(); len(stopped) != 1 || stopped[0] != "task-1" {
		t.Fatalf("a stopped %v", stopped)
	}
	b.until(t, "a to go", func() bool { return peersOf(b.p) == 0 })
	// Closing the page stops what it was receiving.
	b.p.Close()
	if stopped := b.host.stoppedIDs(); len(stopped) != 1 {
		t.Fatalf("b stopped %v", stopped)
	}
}

func TestEnterSendsAndShiftEnterBreaksTheLine(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	f := newFixture(t, flashchat.NewMemoryNetwork(), "peer-a", "Ada", 1000, 760, false)
	f.click(t, tr("createRoom"))
	f.until(t, "the room", func() bool { return f.p.session != nil })
	if err := f.tt.Click(tr("chatDraft")); err != nil {
		t.Fatal(err)
	}
	f.tt.Type("第一行")
	f.tt.Key(ui.Shift, ui.KeyEnter)
	f.tt.Type("第二行")
	f.settle()
	if f.p.drafts[""] != "第一行\n第二行" {
		t.Fatalf("draft %q", f.p.drafts[""])
	}
	f.tt.Key(0, ui.KeyEnter)
	f.until(t, "the message", func() bool { return len(f.p.session.Group().MessageLog()) == 1 })
	if got := f.p.session.Group().MessageLog()[0].Text; got != "第一行\n第二行" {
		t.Fatalf("sent %q", got)
	}
	f.until(t, "the draft to clear", func() bool { return f.p.drafts[""] == "" })
}

// TestShots renders the page's states in both languages and themes, at a
// wide and a narrow width, when WINONE_SHOTS names a folder.
func TestShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS not set")
	}
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, dark := range []bool{false, true} {
			for _, width := range []float32{1000, 520} {
				i18n.SetLocale(locale)
				theme := "light"
				if dark {
					theme = "dark"
				}
				prefix := fmt.Sprintf("%s-%d-%s-", strings.ToLower(locale[:2]), int(width), theme)
				network := flashchat.NewMemoryNetwork()
				a := newFixture(t, network, "peer-a", "Ada Lovelace", width, 700, dark)
				b := newFixture(t, network, "peer-b", "Bob", width, 700, dark)
				shot(t, a, prefix+"lobby")
				a.click(t, tr("networkOptions"))
				a.p.tracker = "wss://tracker.example.com/announce"
				a.p.rtc = `{"iceServers":[{"urls":"stun:stun.example.com:3478"}]}`
				a.settle()
				shot(t, a, prefix+"network")
				a.p.panel = ""
				a.p.tracker, a.p.rtc = "", ""
				a.p.roomName = "Weekly sync · 周会"
				a.settle()
				a.click(t, tr("createRoom"))
				a.until(t, "the room", func() bool { return a.p.session != nil })
				shot(t, a, prefix+"alone")
				b.click(t, tr("joinRoom"))
				b.p.roomID, b.p.password = a.p.session.RoomID, a.p.session.Password
				b.settle()
				b.click(t, tr("joinRoom"))
				a.until(t, "the peer", func() bool { return peersOf(a.p) == 1 })
				b.until(t, "the peer", func() bool { return peersOf(b.p) == 1 })
				_ = b.p.session.Group().SendMessage("Hello **everyone** — 大家好！\n\n```go\nfmt.Println(\"a long line of code that should wrap rather than scroll sideways in a narrow window\")\n```")
				_ = a.p.session.Group().SendMessage("A long message without spaces: " + strings.Repeat("abcdefghij", 12))
				_, _ = b.p.session.Group().OfferFiles([]flashchat.FileHandle{{Name: "report.pdf", Path: `C:\report.pdf`}})
				b.p.session.Group().NotifyTyping()
				a.until(t, "the messages", func() bool {
					return len(a.p.session.Group().MessageLog()) == 2 && len(a.p.session.Group().FileOffers()) == 1
				})
				shot(t, a, prefix+"group")
				if width < narrowWidth {
					a.click(t, tr("chatPeers"))
					shot(t, a, prefix+"members")
				}
				peer := a.p.session.Group().Peers()[0]
				a.p.selectPeer(peer.PeerID)
				a.settle()
				_ = a.p.session.Direct(peer.PeerID).SendMessage("Just between us")
				a.settle()
				shot(t, a, prefix+"private")
				a.p.panel = "room"
				a.settle()
				shot(t, a, prefix+"room")
				a.p.panel = ""
				a.p.Close()
				b.p.Close()
			}
		}
	}
}

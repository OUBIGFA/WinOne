package flashchat

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// The registered entry must present real native controls even while its
// communication engine is starting or unavailable.
func TestCompleteEntryStartsWithNativeControls(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	p := newCompletePage(&core.Env{Post: func(f func()) { f() }})
	defer p.Close()
	tt := ui.NewTester(p.View, 1000, 760)
	tt.Frame()
	if !tt.HasText(tr("peerChat")) || !tt.HasText(tr("nickname")) {
		t.Fatalf("native lobby missing: %q", tt.Texts())
	}
}

func TestNativePushToTalkReleasesWhileAnotherActionIsBusy(t *testing.T) {
	p := newCompletePage(&core.Env{})
	p.snapshot.RoomID = "test-room"
	// Keep the queue occupied so this checks native input-to-command ordering
	// without opening a real microphone in a default unit test.
	p.background = []nativeAction{{scope: "test", action: "pending"}}
	tt := ui.NewTester(p.nativeView, 1000, 760)
	tt.Frame()
	r, ok := tt.Find(nt("pushToTalk"))
	if !ok {
		t.Fatal("missing push-to-talk control")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	if !p.pushTalking || len(p.background) != 2 || p.background[1].args["enabled"] != true {
		t.Fatal("press did not start microphone")
	}
	p.busy = true
	tt.Release(x, y)
	if p.pushTalking || len(p.background) != 3 || p.background[2].args["enabled"] != false {
		t.Fatal("release was lost while busy")
	}
}

func TestNativeExternalVideoURLs(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/watch?v=example", true}, {"https://youtu.be/example", true},
		{"https://youtube.com.attacker.test/watch?v=example", false}, {"https://user@youtube.com/watch?v=example", false},
		{"javascript:alert(1)", false}, {"https://youtube.com/watch", false},
	} {
		if isVideoLink(tc.url) != tc.want {
			t.Errorf("video action for %q", tc.url)
		}
	}
}

// Render the actual product widgets across language/theme/viewport changes.
// This deliberately does not stand in for the real two-peer engine test.
func TestNativeFlashChatLayouts(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, dark := range []bool{false, true} {
			for _, width := range []int{520, 1000} {
				i18n.SetLocale(locale)
				p := newCompletePage(&core.Env{Post: func(f func()) { f() }})
				p.snapshot = nativeSnapshot{Ready: true, UserID: "self", Settings: nativeSettings{Nickname: "Ada", Typing: true}, Community: []string{"news", "hacking", "music", "games"}}
				p.formNickname = "Ada"
				p.formRoom = "room-with-a-long-readable-name"
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(kit.Theme(c.Preferences(), dark))
					ui.Column(c).Fill().Background(c.Theme().Background).Children(func() { p.nativeView(c) })
				}, width, 760)
				prefix := fmt.Sprintf("%s-%d-%t", locale, width, dark)
				capture := func(name string) {
					t.Helper()
					tt.Frame()
					tt.Frame()
					for _, text := range tt.Texts() {
						lower := strings.ToLower(text)
						if strings.Contains(lower, "github") || strings.Contains(lower, "chitchatter") || strings.Contains(lower, "flashchatnative.") || strings.Contains(lower, "interconnect.") {
							t.Fatalf("%s %s: unwanted UI text %q", prefix, name, text)
						}
					}
					if dir := os.Getenv("WINONE_SHOTS"); dir != "" {
						file, err := os.Create(filepath.Join(dir, prefix+"-"+name+".png"))
						if err != nil {
							t.Fatal(err)
						}
						err = png.Encode(file, tt.Image())
						file.Close()
						if err != nil {
							t.Fatal(err)
						}
					}
				}
				capture("lobby")
				if err := tt.Click(nt("settings")); err != nil {
					t.Fatal(err)
				}
				capture("settings")
				p.settingsTab = "network"
				capture("network")
				p.settingsTab = "profile"
				capture("profile")
				p.panel = ""
				p.snapshot.RoomID = "Weekly sync · 周会"
				p.snapshot.Peers = []nativePeer{{PeerID: "bob", UserID: "bob", Name: "Bob — 同事", Verified: 2}}
				p.snapshot.Rooms = map[string]nativeRoom{"": {Messages: []nativeMessage{{ID: "1", AuthorID: "bob", TimeSent: 1700000000000, Text: "Hello **everyone** · 大家好！\n\n```go\nfmt.Println(\"a long line that wraps inside the available conversation area\")\n```"}, {ID: "2", AuthorID: "self", TimeSent: 1700000001000, Text: strings.Repeat("abcdefghij", 12)}}}}
				capture("long-message")
				p.snapshot.RoomID = "design-review"
				p.snapshot.Peers = []nativePeer{{PeerID: "lin", UserID: "lin", Name: "小林", Verified: 2}, {PeerID: "ning", UserID: "ning", Name: "阿宁", Verified: 2}, {PeerID: "mika", UserID: "mika", Name: "Mika", Verified: 2}}
				p.snapshot.Rooms = map[string]nativeRoom{"": {Messages: []nativeMessage{
					{ID: "a", AuthorID: "lin", TimeSent: 1700000000000, Text: "大家下午好，今天一起确认新版的交互。"},
					{ID: "b", AuthorID: "ning", TimeSent: 1700000120000, Text: "我已经看过了。\n\n- 主要操作放在聊天区域\n- 设备设置从右上角打开\n- 文件可以直接拖到输入框"},
					{ID: "c", AuthorID: "self", TimeSent: 1700000180000, Text: "收到，我们先把完整流程走一遍。"},
					{ID: "d", AuthorID: "mika", TimeSent: 1700000240000, Text: "I’ll check the English layout and the smaller window."},
				}}, "lin": {Unread: 2, Messages: []nativeMessage{{ID: "e", AuthorID: "lin", TimeSent: 1700000300000, Text: "稍后把整理好的文件发给你。"}}}}
				p.drafts[""] = "我来记录这次的反馈。"
				capture("room")
				p.panel = "invite"
				capture("invite")
				p.panel = "help"
				capture("help")
			}
		}
	}
}

//go:build windows

package flashchat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"winone/desktopmygo/web"
)

// Explicit opt-in: ordinary tests never open a desktop or use the real profile.
func TestMain(m *testing.M) {
	if os.Getenv("WINONE_FLASHCHAT_NATIVE_WINDOW") != "1" {
		os.Exit(m.Run())
	}
	profile := os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE")
	if !filepath.IsAbs(profile) || !strings.Contains(filepath.ToSlash(profile), "/_temp/") {
		panic("private repository profile required")
	}
	// MyGo's Windows backend uses its own LocalAppData WebView2 directory;
	// PathUserData alone does not isolate the browser process. Give each run
	// an explicit folder so simultaneous private desktops cannot share it.
	if err := os.Setenv("WEBVIEW2_USER_DATA_FOLDER", filepath.Join(profile, "webview")); err != nil {
		panic(err)
	}
	mygo.App.SetName("WinOne Flash Chat Checks")
	if err := web.Register(); err != nil {
		panic(err)
	}
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(profile, "webview"))
	mygo.App.SetPath(mygo.PathDownloads, filepath.Join(profile, "downloads"))
	mygo.App.OnWindowAllClosed(func() {})
	code := 1
	mygo.App.WhenReady(func() { go func() { code = m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

// Compatibility names for existing opt-in runners now exercise native state.
func TestRealNativeFlashChat(t *testing.T) { TestNativeFlashChatUI(t) }
func TestNativePeersThroughRelay(t *testing.T) {
	if os.Getenv("WINONE_FLASHCHAT_PUBLIC_RELAY") != "1" {
		t.Skip("explicit public relay verification")
	}
	TestNativeFlashChatFlows(t)
}

// Live phone verification remains opt-in. It observes received messages from
// the production engine; it never looks for controls in the hidden DOM.
func TestLiveNativeFlashChat(t *testing.T) {
	room := os.Getenv("WINONE_FLASHCHAT_LIVE_ROOM")
	if os.Getenv("WINONE_FLASHCHAT_NATIVE_WINDOW") != "1" || room == "" {
		t.Skip("explicit cross-network session")
	}
	n := newNativeClient(t, "WinOne live verification", false)
	nativeInvoke(t, n, "shell", "join", map[string]any{"room": room, "nickname": "WinOne 验收"})
	nativeAwait(t, "live room", func() bool { return n.p.snapshot.RoomID == room })
	nativeInvoke(t, n, "room:", "send", map[string]any{"text": "WinOne 跨网验证：电脑已进入房间。请发送：手机已连接"})
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		var received bool
		mygo.RunOnMain(func() { received = nativeHas(n.p, "", "手机已连接") || nativeHas(n.p, "", "手机已经连接") })
		if received {
			nativeInvoke(t, n, "room:", "send", map[string]any{"text": "电脑已收到手机消息，跨网双向通信正常。"})
			data, err := n.w.CapturePage()
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE"), "live-connected.png"), data, 0600); err != nil {
				t.Fatal(err)
			}
			t.Log("native chat received the mobile participant's message")
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("no cross-network reply within thirty minutes")
}

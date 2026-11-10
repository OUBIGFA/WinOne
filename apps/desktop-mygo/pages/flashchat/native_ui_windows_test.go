//go:build windows

package flashchat

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Only the explicit build script runs this on a private Windows desktop.
// All accessible controls must be native. WebView2 may expose an empty document
// provider despite AccessHidden, but its inert DOM must expose no controls.
func TestNativeFlashChatUI(t *testing.T) {
	if os.Getenv("WINONE_FLASHCHAT_NATIVE_WINDOW") != "1" {
		t.Skip("explicit private desktop required")
	}
	profile := os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE")
	env := &core.Env{Directory: profile}
	p := newCompletePage(env)
	var window *mygo.Window
	mygo.RunOnMain(func() {
		i18n.SetLocale(i18n.Chinese)
		window = mygo.NewWindow(mygo.WindowOptions{Title: "WinOne Native Flash Chat", Width: 1000, Height: 800, Hidden: true, Content: ui.View(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), false))
			p.View(c)
		})})
		env.Post = func(fn func()) {
			mygo.RunOnMain(func() {
				if !window.IsDestroyed() {
					window.Update(fn)
				}
			})
		}
		env.WebViews = window.NewWebView
		window.Show()
	})
	defer mygo.RunOnMain(func() { p.Close(); window.Destroy() })
	deadline := time.Now().Add(50 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		mygo.RunOnMain(func() { ready = p.snapshot.Ready && !p.busy && p.formRoom != "" })
		if ready {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("engine did not initialize: %s", p.err)
	}
	frame, err := window.CapturePage()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "native-lobby.png"), frame, 0600); err != nil {
		t.Fatal(err)
	}
	// agent-desktop is launched by the test process, on its private desktop.
	driver := exec.Command("cmd.exe", "/c", "agent-desktop", "snapshot", "--window-id", fmt.Sprintf("w-%d", window.NativeHandle()), "--include-bounds", "--timeout-ms", "10000")
	output, err := driver.CombinedOutput()
	_ = os.WriteFile(filepath.Join(profile, "native-accessibility.json"), output, 0600)
	if err != nil {
		t.Fatalf("native accessibility: %s %v", output, err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(output, &snapshot); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), nt("enter")) || !strings.Contains(string(output), tr("nickname")) {
		t.Fatalf("missing native controls: %s", output)
	}
	var checkWebControls func(any, bool)
	checkWebControls = func(value any, inDocument bool) {
		switch node := value.(type) {
		case map[string]any:
			if inDocument && node["name"] != nil && node["name"] != "" {
				t.Fatalf("web content appeared in accessibility tree: %v", node)
			}
			for _, child := range node {
				checkWebControls(child, inDocument || node["role"] == "document")
			}
		case []any:
			for _, child := range node {
				checkWebControls(child, inDocument)
			}
		}
	}
	checkWebControls(snapshot, false)
	if strings.Contains(strings.ToLower(string(output)), "github") {
		t.Fatal("upstream information in accessibility tree")
	}
	t.Log(fmt.Sprintf("native lobby ready; %d bytes in accessibility tree", len(output)))
	drive := func(action, role, name, value string) {
		t.Helper()
		output, err := exec.Command("cmd.exe", "/c", "agent-desktop", "snapshot", "--window-id", fmt.Sprintf("w-%d", window.NativeHandle()), "--timeout-ms", "10000").CombinedOutput()
		if err != nil {
			t.Fatalf("snapshot: %s %v", output, err)
		}
		var document map[string]any
		if err := json.Unmarshal(output, &document); err != nil {
			t.Fatal(err)
		}
		var find func(any) string
		find = func(value any) string {
			switch node := value.(type) {
			case map[string]any:
				if node["name"] == name && node["role"] == role {
					if ref, ok := node["ref_id"].(string); ok {
						return ref
					}
				}
				for _, child := range node {
					if ref := find(child); ref != "" {
						return ref
					}
				}
			case []any:
				for _, child := range node {
					if ref := find(child); ref != "" {
						return ref
					}
				}
			}
			return ""
		}
		ref := find(document)
		if ref == "" {
			t.Fatalf("missing %s %s: %s", role, name, output)
		}
		args := []string{"/c", "agent-desktop", action, ref}
		if action == "set-value" {
			args = append(args, value)
		}
		out, err := exec.Command("cmd.exe", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s: %s %v", action, name, out, err)
		}
		t.Log(action, name)
	}
	wait := func(label string, check func() bool) {
		t.Helper()
		until := time.Now().Add(30 * time.Second)
		for time.Now().Before(until) {
			var done bool
			mygo.RunOnMain(func() { done = check() })
			if done {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("waiting for %s: %s %s", label, p.actionError, p.snapshot.Alert)
	}
	drive("set-value", "textfield", tr("nickname"), "Native Tester")
	drive("set-value", "textfield", nt("room"), "native-ui-"+randomToken()[:16])
	drive("click", "button", nt("enter"), "")
	wait("room", func() bool { return p.snapshot.RoomID != "" && !p.busy })
	drive("set-value", "textfield", tr("chatDraft"), "Hello from native controls")
	drive("click", "button", tr("sendMessage"), "")
	wait("native message", func() bool { return len(p.snapshot.Rooms[""].Messages) == 1 })
	drive("click", "button", nt("camera"), "")
	wait("camera frame", func() bool { return p.snapshot.Media.Camera && p.frames["self-camera"] != nil })
	drive("click", "button", nt("microphone"), "")
	wait("microphone", func() bool { return p.snapshot.Media.Microphone })
	drive("click", "button", nt("screen"), "")
	wait("screen frame", func() bool { return p.snapshot.Media.Screen && p.frames["self-screen"] != nil })
	frame, err = window.CapturePage()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "native-media.png"), frame, 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"screen", "camera", "microphone"} {
		drive("click", "button", nt("stop")+" · "+nt(kind), "")
	}
	wait("media stopped", func() bool {
		return !p.snapshot.Media.Camera && !p.snapshot.Media.Screen && !p.snapshot.Media.Microphone && len(p.snapshot.Media.Streams) == 0
	})
	frame, err = window.CapturePage()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(profile, "native-after-media.png"), frame, 0600); err != nil {
		t.Fatal(err)
	}
	drive("expand", "menubutton", nt("more"), "")
	drive("click", "menuitem", nt("invite"), "")
	wait("native invitation", func() bool { return p.panel == "invite" && p.qr != nil })
	drive("click", "button", nt("copyInvite"), "")
	drive("click", "button", nt("done"), "")
	drive("expand", "menubutton", nt("more"), "")
	drive("click", "menuitem", nt("settings"), "")
	drive("set-value", "textfield", tr("nickname"), "Updated Native Tester")
	drive("click", "button", nt("applyNickname"), "")
	wait("native nickname saved", func() bool { return !p.busy && p.snapshot.Settings.Nickname == "Updated Native Tester" })
	drive("click", "button", nt("done"), "")
	drive("expand", "menubutton", nt("more"), "")
	drive("click", "menuitem", tr("leaveRoom"), "")
	wait("lobby after leaving", func() bool { return p.snapshot.RoomID == "" && !p.busy })
}

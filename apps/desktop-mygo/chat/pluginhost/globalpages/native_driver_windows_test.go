//go:build windows

package globalpages

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
)

type nativeRect struct{ Left, Top, Right, Bottom int32 }

func nativeWindowRect(t *testing.T, window *mygo.Window) nativeRect {
	t.Helper()
	var rect nativeRect
	var ok uintptr
	mygo.RunOnMain(func() {
		ok, _, _ = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowRect").Call(window.NativeHandle(), uintptr(unsafe.Pointer(&rect)))
	})
	if ok == 0 {
		t.Fatal("actual owned HWND bounds are unavailable")
	}
	return rect
}

// Keyboard input needs the explicitly switched, watchdog-owned input desktop.
// Refuse to inject if it has already returned to the user's desktop. SendInput
// reaches WebView2's actual accelerator hook, not a DOM event or Host ticket.
func nativePanelKey(t *testing.T, window *mygo.Window, page *mygo.Page) {
	t.Helper()
	user32 := windows.NewLazySystemDLL("user32.dll")
	input, _, _ := user32.NewProc("OpenInputDesktop").Call(0, 0, 1)
	if input == 0 {
		t.Fatal("native keyboard cannot read the current input desktop")
	}
	defer user32.NewProc("CloseDesktop").Call(input)
	var name [256]uint16
	var needed uint32
	ok, _, _ := user32.NewProc("GetUserObjectInformationW").Call(input, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Sizeof(name)), uintptr(unsafe.Pointer(&needed)))
	if ok == 0 || os.Getenv("WINONE_NATIVE_INPUT_DESKTOP") == "" || windows.UTF16ToString(name[:]) != os.Getenv("WINONE_NATIVE_INPUT_DESKTOP") {
		t.Fatal("native keyboard requires the explicitly selected private input desktop")
	}
	if _, err := page.Eval(`window.__nativePanelKey=0;document.addEventListener('keydown',event=>{if(event.isTrusted)window.__nativePanelKey++},true);return true`); err != nil {
		t.Fatal(err)
	}
	// Win32 INPUT is 40 bytes on our Windows amd64/arm64 native test targets.
	type keyInput struct {
		Type        uint32
		_           uint32
		VK, Scan    uint16
		Flags, Time uint32
		_           uint32
		Extra       uintptr
		_           [8]byte
	}
	// WebView2 defines accelerator input as Ctrl/Alt or non-character keys;
	// plain letters and Enter do not raise this native callback. F8 has no
	// fixture/application command and still exercises the production hook.
	inputs := [2]keyInput{{Type: 1, VK: 0x77}, {Type: 1, VK: 0x77, Flags: 2}}
	var sent uintptr
	mygo.RunOnMain(func() {
		foreground, _, _ := user32.NewProc("GetForegroundWindow").Call()
		if foreground == window.NativeHandle() {
			sent, _, _ = user32.NewProc("SendInput").Call(2, uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
		}
	})
	if sent != 2 {
		t.Fatal("native keyboard did not target the owned foreground window")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		value, err := page.Eval(`return window.__nativePanelKey`)
		if count, ok := value.(float64); err == nil && ok && count > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("actual OS keyboard input did not reach the focused plugin document")
}

// Dispatch only to the test-owned MyGo surface on its private desktop. The
// native hit test/embedded focus/gesture hook runs unchanged; no DOM dispatch,
// claimed document.hasFocus or synthetic Host permission call is used.
func nativePanelClick(t *testing.T, window *mygo.Window, page *mygo.Page, bounds ui.Rect) {
	t.Helper()
	user32 := windows.NewLazySystemDLL("user32.dll")
	find := user32.NewProc("FindWindowExW")
	getClient := user32.NewProc("GetClientRect")
	send := user32.NewProc("SendMessageW")
	var surface uintptr
	var rect struct{ Left, Top, Right, Bottom int32 }
	mygo.RunOnMain(func() {
		class := windows.StringToUTF16Ptr("MyGoSurface")
		var child uintptr
		for {
			child, _, _ = find.Call(window.NativeHandle(), child, uintptr(unsafe.Pointer(class)), 0)
			if child == 0 {
				break
			}
			var candidate struct{ Left, Top, Right, Bottom int32 }
			if ok, _, _ := getClient.Call(child, uintptr(unsafe.Pointer(&candidate))); ok != 0 && candidate.Right*candidate.Bottom > rect.Right*rect.Bottom {
				surface, rect = child, candidate
			}
		}
	})
	if surface == 0 {
		t.Fatal("owned native panel surface is absent")
	}
	width, height := window.ContentSize()
	x := (bounds.X + bounds.W/2) * float32(rect.Right) / float32(width)
	y := (bounds.Y + bounds.H*.7) * float32(rect.Bottom) / float32(height)
	point := uintptr(uint16(x)) | uintptr(uint16(y))<<16
	if _, err := page.Eval(`window.__nativePanelPointer=0;document.addEventListener('mousedown',()=>window.__nativePanelPointer++,true);return true`); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mygo.RunOnMain(func() {
			send.Call(surface, 0x200, 0, point)
			send.Call(surface, 0x201, 1, point)
			send.Call(surface, 0x202, 0, point)
		})
		value, err := page.Eval(`return window.__nativePanelPointer`)
		if count, ok := value.(float64); err == nil && ok && count > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("OS-dispatched input did not reach the placed native panel (%v)", bounds)
}

// A request travels through the original page BroadcastChannel relay to the
// actual installed sandbox's cindy SDK. The reply is not a fake Host result.
func nativePanelRequest(t *testing.T, page *mygo.Page, input map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	value, err := page.EvalContext(ctx, `return await window.nativeCapabilityRequest(`+string(raw)+`)`)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("original SDK reply is not an object: %v", value)
	}
	return result
}

func nativeWindowAction(t *testing.T, window *mygo.Window, label string) {
	t.Helper()
	call := func(method string, params any) json.RawMessage {
		t.Helper()
		conn, err := net.DialTimeout("unix", os.Getenv("MYGO_AUTOMATION"), 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		if err := json.NewEncoder(conn).Encode(map[string]any{"id": 1, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(conn).ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			Result json.RawMessage
			Error  *struct{ Code, Message string }
		}
		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatal(err)
		}
		if response.Error != nil {
			t.Fatalf("native %s: %s %s", method, response.Error.Code, response.Error.Message)
		}
		return response.Result
	}
	var tree struct {
		Nodes []struct {
			ID, Label       string
			States, Actions []string
		}
	}
	if err := json.Unmarshal(call("tree", map[string]any{"window": window.ID()}), &tree); err != nil {
		t.Fatal(err)
	}
	for _, node := range tree.Nodes {
		if node.Label == label && slices.Contains(node.Actions, "press") && !slices.Contains(node.States, "offscreen") && !slices.Contains(node.States, "disabled") {
			call("act", map[string]any{"window": window.ID(), "id": node.ID, "action": "press"})
			return
		}
	}
	t.Fatalf("actual native control %q is unavailable", label)
}

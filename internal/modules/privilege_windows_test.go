//go:build windows

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"winone/internal/host"
)

func TestPrivilegeCancellationRestoresRealOwnedWindow(t *testing.T) {
	testPrivilegeTransition(t, false)
}

func TestLiveAdministratorPrivilegeRoundTrip(t *testing.T) {
	if os.Getenv("WINONE_LIVE_ELEVATION_TEST") != "1" {
		t.Skip("explicit UAC consent test only")
	}
	testPrivilegeTransition(t, true)
}

func testPrivilegeTransition(t *testing.T, live bool) {
	t.Helper()
	exe, fixture := os.Getenv("WINONE_AOT_EXE"), os.Getenv("WINONE_WINDOW_FIXTURE")
	if exe == "" || fixture == "" {
		t.Skip("set native worker and fixture paths")
	}
	if !live {
		original := elevatedLaunch
		elevatedLaunch = func(string, []string) (uint32, func() error, error) {
			return 0, nil, errors.New("test consent cancelled")
		}
		defer func() { elevatedLaunch = original }()
	}
	dir := t.TempDir()
	identityFile := filepath.Join(dir, "window.json")
	cmd := exec.Command(fixture, identityFile)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	var identity struct {
		Handle  string `json:"handle"`
		PID     uint32 `json:"pid"`
		Created string `json:"created"`
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(identityFile)
		if err == nil && json.Unmarshal(data, &identity) == nil && identity.Handle != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture unavailable")
		}
		time.Sleep(25 * time.Millisecond)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-privilege-test-%d`, time.Now().UnixNano())
	var administratorIdentity json.RawMessage
	var administratorPath string
	if live {
		path := filepath.Join(dir, "administrator-window.json")
		administratorPath = path
		_, wait, err := launchElevated(fixture, []string{path, "--controlled"})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.WriteFile(path+".close", []byte("close"), 0600); err != nil {
				t.Error(err)
			}
			done := make(chan error, 1)
			go func() { done <- wait() }()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(5 * time.Second):
				t.Error("administrator fixture did not close")
			}
		}()
		limit := time.Now().Add(5 * time.Second)
		for {
			data, err := os.ReadFile(path)
			if err == nil && json.Valid(data) {
				administratorIdentity = data
				break
			}
			if time.Now().After(limit) {
				t.Fatal("administrator fixture not ready")
			}
			time.Sleep(25 * time.Millisecond)
		}
		if err := os.WriteFile(path+".show", []byte("show"), 0600); err != nil {
			t.Fatal(err)
		}
		var visibleIdentity struct {
			Handle string `json:"handle"`
		}
		json.Unmarshal(administratorIdentity, &visibleIdentity)
		visibleHandle, _ := strconv.ParseUint(visibleIdentity.Handle, 10, 64)
		limit = time.Now().Add(5 * time.Second)
		for {
			visible, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("IsWindowVisible").Call(uintptr(visibleHandle))
			if visible != 0 {
				break
			}
			if time.Now().After(limit) {
				t.Fatal("administrator target did not become visible")
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	lazy := NewLazy(context.Background(), "always-on-top", "置顶", exe, pipe, filepath.Join(dir, "data"))
	defer func() {
		if err := lazy.Close(); err != nil {
			t.Error(err)
		}
	}()
	call := func(method string, params any) host.Response {
		data, _ := json.Marshal(params)
		return lazy.Handle(host.Request{Version: 1, ID: "test", Method: method, Params: data})
	}
	snapshot := call("snapshot", nil)
	if snapshot.Error != nil {
		t.Fatal(snapshot.Error)
	}
	var parsed struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(snapshot.Result, &parsed); err != nil {
		t.Fatal(err)
	}
	parsed.Settings["blockInGameMode"] = false
	parsed.Settings["frame"] = false
	if r := call("settings.save", parsed.Settings); r.Error != nil {
		t.Fatal(r.Error)
	}
	if r := call("window.set", map[string]any{"window": identity, "pinned": true}); r.Error != nil {
		t.Fatal(r.Error)
	}
	if live {
		if r := call("window.set", map[string]any{"window": administratorIdentity, "pinned": true}); r.Error == nil {
			t.Fatal("ordinary worker unexpectedly controlled administrator window")
		}
	}
	h, _ := strconv.ParseUint(identity.Handle, 10, 64)
	user32 := windows.NewLazySystemDLL("user32.dll")
	user32.NewProc("ShowWindow").Call(uintptr(h), 0)
	if r := call("privilege.start", map[string]bool{"administrator": true}); r.Error != nil {
		t.Fatal(r.Error)
	}
	deadline = time.Now().Add(120 * time.Second)
	for {
		r := call("privilege.status", nil)
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		var status PrivilegeState
		if err := json.Unmarshal(r.Result, &status); err != nil {
			t.Fatal(err)
		}
		if status.State != "running" {
			if live && status.State != "completed" || !live && (status.State != "failed" || !strings.Contains(status.Error, "test consent cancelled")) {
				t.Fatalf("unexpected consent result: %+v", status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("privilege rollback did not finish")
		}
		time.Sleep(25 * time.Millisecond)
	}
	style, _, _ := user32.NewProc("GetWindowLongPtrW").Call(uintptr(h), ^uintptr(19))
	if style&8 == 0 {
		t.Fatal("cancelled consent lost the hidden window pin")
	}
	response := call("snapshot", nil)
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var restored struct {
		Administrator bool              `json:"administrator"`
		OwnedWindows  []json.RawMessage `json:"ownedWindows"`
	}
	if err := json.Unmarshal(response.Result, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Administrator != live || len(restored.OwnedWindows) != 1 {
		t.Fatalf("requested mode was not restored: %+v", restored)
	}
	if live {
		if r := call("window.set", map[string]any{"window": administratorIdentity, "pinned": true}); r.Error != nil {
			t.Fatalf("administrator pin failed: %+v", r.Error)
		}
		var high struct {
			Handle string `json:"handle"`
		}
		json.Unmarshal(administratorIdentity, &high)
		highHandle, _ := strconv.ParseUint(high.Handle, 10, 64)
		style, _, readErr := user32.NewProc("GetWindowLongPtrW").Call(uintptr(highHandle), ^uintptr(19))
		var privileged struct {
			OwnedWindows []struct {
				Handle  string `json:"handle"`
				Pinned  bool   `json:"pinned"`
				Topmost bool   `json:"topmost"`
			} `json:"ownedWindows"`
		}
		deadline = time.Now().Add(2 * time.Second)
		for {
			observed := call("snapshot", nil)
			if observed.Error != nil || json.Unmarshal(observed.Result, &privileged) != nil {
				t.Fatal("cannot inspect administrator window")
			}
			found := false
			for _, window := range privileged.OwnedWindows {
				if window.Handle == high.Handle && window.Pinned && window.Topmost {
					found = true
				}
			}
			if found {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("administrator window not confirmed: windows=%+v mediumStyle=%x mediumRead=%v", privileged.OwnedWindows, style, readErr)
			}
			time.Sleep(25 * time.Millisecond)
		}
		if err := os.WriteFile(administratorPath+".hide", []byte("hide"), 0600); err != nil {
			t.Fatal(err)
		}
		deadline = time.Now().Add(5 * time.Second)
		for {
			visible, _, _ := user32.NewProc("IsWindowVisible").Call(uintptr(highHandle))
			if visible == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("administrator target did not hide")
			}
			time.Sleep(25 * time.Millisecond)
		}
		if r := call("window.set", map[string]any{"window": administratorIdentity, "pinned": false}); r.Error != nil {
			t.Fatal(r.Error)
		}
		if r := call("privilege.start", map[string]bool{"administrator": false}); r.Error != nil {
			t.Fatal(r.Error)
		}
		deadline = time.Now().Add(15 * time.Second)
		for {
			r := call("privilege.status", nil)
			var status PrivilegeState
			if r.Error != nil || json.Unmarshal(r.Result, &status) != nil {
				t.Fatal("cannot read downgrade status")
			}
			if status.State != "running" {
				if status.State != "completed" {
					t.Fatalf("downgrade failed: %+v", status)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("downgrade timeout")
			}
			time.Sleep(25 * time.Millisecond)
		}
		r := call("snapshot", nil)
		if r.Error != nil || json.Unmarshal(r.Result, &restored) != nil || restored.Administrator || len(restored.OwnedWindows) != 1 {
			t.Fatalf("ordinary mode lost pins: %+v", restored)
		}
	}
}

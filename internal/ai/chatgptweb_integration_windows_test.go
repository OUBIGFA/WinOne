//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The test runner substitutes a local website, not the plugin's auth, browser
// or worker. This crosses WinOne's real short-lived process and profile boundary.
func TestChatGPTWebPluginSignInAndReply(t *testing.T) {
	fixture, executable := os.Getenv("WINONE_CHATGPT_WEB_FIXTURE"), os.Getenv("WINONE_AI_PLUGIN_TEST_EXE")
	if fixture == "" || executable == "" {
		t.Skip("explicit reviewed ChatGPT Web fixture")
	}
	assertBackgroundBootstrapWaits(t, executable)
	ctx, cancel := context.WithTimeout(context.Background(), 330*time.Second)
	defer cancel()
	directory := os.Getenv("WINONE_CHATGPT_WEB_DATA")
	if directory == "" {
		directory = t.TempDir()
	}
	runtime := Translator{Executable: executable, DataDirectory: directory, AccountBackend: "plugin", PluginLifetime: ctx}
	var options map[string]any
	if err := json.Unmarshal([]byte(os.Getenv("WINONE_CHATGPT_WEB_OPTIONS")), &options); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(runtime.DataDirectory, "third-party", "plugins.json")
	if err := os.MkdirAll(filepath.Dir(registry), 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"plugins": []any{map[string]any{"spec": fixture, "options": options}}})
	if err := os.WriteFile(registry, raw, 0600); err != nil {
		t.Fatal(err)
	}
	listing, err := runtime.Accounts(ctx, "accounts.list", "chatgpt-web")
	if err != nil {
		t.Fatal(err)
	}
	var before struct{ Methods []json.RawMessage }
	if json.Unmarshal(listing, &before) != nil || len(before.Methods) != 4 {
		t.Fatalf("all four authorization methods required: %s", listing)
	}
	method := 0
	done, loginVisible := false, false
	err = runtime.SignInMethod(ctx, "chatgpt-web", &method, make(chan string), func(raw json.RawMessage) {
		var event struct {
			Error  string
			SignIn struct{ State, Error string }
		}
		if json.Unmarshal(raw, &event) != nil {
			t.Errorf("invalid event: %s", raw)
			return
		}
		if event.Error != "" || event.SignIn.Error != "" {
			t.Errorf("authorization failure: %s", raw)
		}
		if event.SignIn.State == "waiting" && options["headless"] != true && options["executablePath"] == nil {
			// CDP can be ready just before Windows shows the browser. The fixture
			// keeps authorization pending during this bounded visibility check.
			until := time.Now().Add(3 * time.Second)
			for !ownedBrowserVisible(fixture) && time.Now().Before(until) {
				time.Sleep(25 * time.Millisecond)
			}
			loginVisible = ownedBrowserVisible(fixture)
			t.Logf("waiting: visible=%v browser-cache=%q windows=%s", loginVisible, os.Getenv("PLAYWRIGHT_BROWSERS_PATH"), ownedBrowserWindowStates(fixture))
			if !loginVisible {
				t.Error("login browser was not visible while awaiting authorization")
				cancel()
				return
			}
			if marker := os.Getenv("WINONE_CHATGPT_WEB_VISIBLE_MARKER"); marker != "" {
				if err := os.WriteFile(marker, []byte("visible"), 0600); err != nil {
					t.Error(err)
					cancel()
				}
			}
		}
		if event.SignIn.State == "done" {
			done = true
		}
	})
	if err != nil || !done {
		t.Fatalf("dedicated browser login did not finish: %v", err)
	}
	if options["headless"] != true && options["executablePath"] == nil {
		if !loginVisible {
			t.Fatal("login browser was not visible while awaiting authorization")
		}
		until := time.Now().Add(3 * time.Second)
		for ownedBrowserVisible(fixture) && time.Now().Before(until) {
			time.Sleep(25 * time.Millisecond)
		}
		if ownedBrowserVisible(fixture) {
			t.Fatal("authenticated browser did not move to background")
		}
	}
	listing, err = runtime.Accounts(ctx, "accounts.list", "chatgpt-web")
	if err != nil || !strings.Contains(string(listing), "fixture@example.invalid") {
		t.Fatalf("login lost across workers: %s %v", listing, err)
	}
	// Generation callers select the backend in Preferences, not in Translator.
	// Repeat after the login worker exits so the production lifetime is tested.
	runtime.AccountBackend = ""
	// A cold request (also reached after restarting WinOne) must never put a
	// browser on the interactive desktop. Close only this fixture's browser.
	closeFixtureBrowser(t, fixture)
	reply, err := runtime.GenerateStream(ctx, Preferences{Engine: "ai", AuthMode: "account", AgentBackend: "plugin", Agent: "chatgpt-web", Model: "web-current"}, "", Generation{Text: "本地网页文本测试"}, func(Delta) error {
		if options["headless"] != true && ownedBrowserVisible(fixture) {
			t.Error("browser became visible during a background request")
		}
		return nil
	})
	if err != nil || !strings.Contains(reply, "中文流式") {
		t.Fatalf("browser reply across workers: %q %v", reply, err)
	}
	if options["headless"] != true && ownedBrowserVisible(fixture) {
		t.Fatal("request exposed its dedicated browser on the interactive desktop")
	}
	reply, err = runtime.GenerateStream(ctx, Preferences{Engine: "ai", AuthMode: "account", AgentBackend: "plugin", Agent: "chatgpt-web", Model: "web-current"}, "", Generation{Text: "第二次中文请求"}, func(Delta) error { return nil })
	if err != nil || !strings.Contains(reply, "中文流式") {
		t.Fatalf("second browser request: %q %v", reply, err)
	}
	if count := ownedBrowserProcessCount(t, fixture); count == 0 {
		t.Fatal("keep-open browser did not survive the request")
	} else {
		t.Logf("background browser processes before Host exit: %d", count)
	}
	cancel()
	until := time.Now().Add(5 * time.Second)
	for ownedBrowserProcessCount(t, fixture) > 0 && time.Now().Before(until) {
		time.Sleep(25 * time.Millisecond)
	}
	if count := ownedBrowserProcessCount(t, fixture); count != 0 {
		t.Fatalf("Host exit left %d browser descendants", count)
	}
	// A fresh Host can reuse the saved profile after every browser has exited.
	restarted, stopRestarted := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopRestarted()
	runtime.PluginLifetime = restarted
	reply, err = runtime.GenerateStream(restarted, Preferences{Engine: "ai", AuthMode: "account", AgentBackend: "plugin", Agent: "chatgpt-web", Model: "web-current"}, "", Generation{Text: "重启后请求"}, func(Delta) error { return nil })
	if err != nil || !strings.Contains(reply, "中文流式") {
		t.Fatalf("saved login after Host restart: %q %v", reply, err)
	}
	requestCtx, stopRequest := context.WithCancel(restarted)
	_, err = runtime.GenerateStream(requestCtx, Preferences{Engine: "ai", AuthMode: "account", AgentBackend: "plugin", Agent: "chatgpt-web", Model: "web-current"}, "", Generation{Text: "取消流式请求"}, func(Delta) error { stopRequest(); return nil })
	stopRequest()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("stream cancellation was not returned: %v", err)
	}
	stopRestarted()
	until = time.Now().Add(5 * time.Second)
	for ownedBrowserProcessCount(t, fixture) > 0 && time.Now().Before(until) {
		time.Sleep(25 * time.Millisecond)
	}
	if n := ownedBrowserProcessCount(t, fixture); n != 0 {
		t.Fatalf("restart/cancel left %d browser processes", n)
	}
}

func ownedBrowserProcessCount(t *testing.T, directory string) int {
	t.Helper()
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	count := 0
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		p, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if e != nil {
			continue
		}
		var path [32768]uint16
		size := uint32(len(path))
		e = windows.QueryFullProcessImageName(p, 0, &path[0], &size)
		windows.CloseHandle(p)
		if e == nil {
			rel, e := filepath.Rel(directory, windows.UTF16ToString(path[:size]))
			if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				count++
			}
		}
	}
	return count
}

// Deliberately delay job assignment. No descendant may escape while the Host
// is still preparing the worker; the first request is its admission signal.
func assertBackgroundBootstrapWaits(t *testing.T, executable string) {
	t.Helper()
	cmd := exec.Command(executable)
	cmd.Env = append(isolatedEnvironment(t.TempDir()), "WINONE_AI_BACKGROUND_DESKTOP=1")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	hideConsole(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	time.Sleep(300 * time.Millisecond)
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ParentProcessID == uint32(cmd.Process.Pid) {
			if p, e := windows.OpenProcess(windows.PROCESS_TERMINATE, false, entry.ProcessID); e == nil {
				windows.TerminateProcess(p, 1)
				windows.CloseHandle(p)
			}
			t.Error("background bootstrap started a child before the Host admitted its request")
		}
	}
}

// Capture only windows belonging to this test's copied browser. Hidden
// windows distinguish an actual launch problem from a visibility race.
func ownedBrowserWindowStates(codeDirectory string) string {
	user32 := windows.NewLazySystemDLL("user32.dll")
	var states []map[string]any
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			return 1
		}
		defer windows.CloseHandle(process)
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		if windows.QueryFullProcessImageName(process, 0, &buffer[0], &size) != nil {
			return 1
		}
		path := windows.UTF16ToString(buffer[:size])
		relative, err := filepath.Rel(codeDirectory, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return 1
		}
		var class [128]uint16
		user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
		states = append(states, map[string]any{"window": hwnd, "pid": pid, "class": windows.UTF16ToString(class[:]), "visible": visible != 0, "path": relative})
		return 1
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	raw, _ := json.Marshal(states)
	return string(raw)
}

func closeFixtureBrowser(t *testing.T, directory string) {
	t.Helper()
	var states []struct {
		Window uintptr
		PID    uint32
		Class  string
	}
	if err := json.Unmarshal([]byte(ownedBrowserWindowStates(directory)), &states); err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Class != "Chrome_WidgetWin_1" {
			continue
		}
		process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, state.PID)
		if err != nil {
			continue
		}
		windows.NewLazySystemDLL("user32.dll").NewProc("PostMessageW").Call(state.Window, 0x10, 0, 0)
		status, err := windows.WaitForSingleObject(process, 10000)
		windows.CloseHandle(process)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatalf("fixture browser did not close: %d %v", status, err)
		}
	}
}

// Verify an actual top-level window of this fresh browser copy. Process
// existence and CDP readiness alone cannot prove that sign-in is visible.
func ownedBrowserVisible(codeDirectory string) bool {
	user32 := windows.NewLazySystemDLL("user32.dll")
	visible := false
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if shown, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd); shown == 0 {
			return 1
		}
		var pid uint32
		user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			return 1
		}
		defer windows.CloseHandle(process)
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		if windows.QueryFullProcessImageName(process, 0, &buffer[0], &size) == nil {
			relative, err := filepath.Rel(codeDirectory, windows.UTF16ToString(buffer[:size]))
			if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
				visible = true
				return 0
			}
		}
		return 1
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	return visible
}

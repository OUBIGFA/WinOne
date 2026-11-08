package resultui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"winone/internal/ai"
	"winone/internal/host"
	"winone/internal/protocol"
	"winone/internal/resultwindow"
	"winone/internal/transport"
)

// This opt-in probe runs the production executable with a pipe owned by the
// test process. No real Host, settings, onboarding, AI request or clipboard is
// used. The build entry explicitly selects it after producing the executable.
type nativeResultHost struct {
	requests chan host.Request
}

func (h *nativeResultHost) Handle(r host.Request) host.Response {
	var value any = map[string]any{}
	switch r.Method {
	case "results.ready", "results.closed", "results.selected":
		h.requests <- r
	case "ai.configuration.get":
		value = ai.Configuration{}
	case "ai.generate.start":
		h.requests <- r
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "configuration", Message: "请先配置 AI 服务"}}
	case "results.composerHint.claim":
		value = map[string]bool{"show": false}
	case "quick-assistant.settings.get":
		value = map[string]any{"alwaysOnTop": true}
	case "results.learning.get":
		value = map[string]string{"sustainable": "focused", "coordination": "ignored"}
	default:
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "probe", Message: "unexpected method: " + r.Method}}
	}
	data, _ := json.Marshal(value)
	return host.Response{ID: r.ID, Result: data}
}

func TestNativeResultProcess(t *testing.T) {
	exe, output := os.Getenv("WINONE_RESULT_PROBE_EXE"), os.Getenv("WINONE_RESULT_PROBE_DIR")
	if exe == "" || output == "" {
		t.Skip("build/build-results.ps1 -RunWindowChecks selects the isolated native probe")
	}
	for _, theme := range []string{"light", "dark"} {
		for _, language := range []string{"zh-CN", "en-US"} {
			t.Run(theme+"/"+language, func(t *testing.T) {
				directory := filepath.Join(output, theme+"-"+language)
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				pipe := fmt.Sprintf(`\\.\pipe\WinOne-Result-Probe-%d-%d`, os.Getpid(), time.Now().UnixNano())
				listener, err := transport.Listen(pipe)
				if err != nil {
					t.Fatal(err)
				}
				h := &nativeResultHost{requests: make(chan host.Request, 32)}
				go transport.Serve(ctx, listener, h)
				cmd := exec.CommandContext(ctx, exe)
				cmd.Dir = directory
				cmd.Env = append(os.Environ(), "APPDATA="+directory, "LOCALAPPDATA="+directory, "MYGO_LOCALE="+LocaleTag(language), "MYGO_GPU=0")
				var diagnostics bytes.Buffer
				cmd.Stdout, cmd.Stderr = &diagnostics, &diagnostics
				stdin, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				exited := make(chan struct{})
				var exitErr error
				go func() { exitErr = cmd.Wait(); close(exited) }()
				defer func() {
					stdin.Close()
					cancel()
					<-exited
				}()
				send := func(command resultwindow.Command) {
					t.Helper()
					data, err := json.Marshal(command)
					if err == nil {
						err = protocol.WriteFrame(stdin, data)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				await := func(method, id string) map[string]string {
					t.Helper()
					for {
						select {
						case r := <-h.requests:
							var params map[string]string
							if err := json.Unmarshal(r.Params, &params); err != nil {
								t.Fatal(err)
							}
							if r.PeerPID != uint32(cmd.Process.Pid) {
								t.Fatal("receipt from another process")
							}
							if r.Method == method && params["id"] == id {
								return params
							}
						case <-exited:
							t.Fatalf("exited before %s: %v; %s", method, exitErr, diagnostics.String())
						case <-ctx.Done():
							t.Fatalf("timeout waiting for %s", method)
						}
					}
				}
				open := func(id, source string, toolbar bool) uintptr {
					t.Helper()
					send(resultwindow.Command{Kind: "open", Initial: &resultwindow.Initial{ID: id, Token: "probe-token", Pipe: pipe, HostPID: uint32(os.Getpid()), Mode: "chat", Source: source, Toolbar: toolbar, Theme: theme, Language: language, Text: "Isolated draft · 隔离测试草稿", Quick: resultwindow.QuickOptions{AlwaysOnTop: true}}})
					ready := await("results.ready", id)
					window, err := strconv.ParseUint(ready["window"], 10, 64)
					if err != nil || window == 0 || ready["token"] != "probe-token" {
						t.Fatal("invalid ready receipt", ready, err)
					}
					var pid uint32
					frameUser.NewProc("GetWindowThreadProcessId").Call(uintptr(window), uintptr(unsafe.Pointer(&pid)))
					if pid != uint32(cmd.Process.Pid) {
						t.Fatal("window is not owned by the result process")
					}
					return uintptr(window)
				}
				quick := open("quick", "quick-assistant", false)
				style := frameUser.NewProc("GetWindowLongPtrW")
				quickEx, _, _ := style.Call(quick, ^uintptr(19))
				if quickEx&0x80 != 0 || quickEx&8 == 0 {
					t.Fatalf("quick styles=%#x", quickEx)
				}
				if shot := os.Getenv("WINONE_WINDOW_SHOT_EXE"); shot != "" {
					capture := exec.CommandContext(ctx, shot, strconv.Itoa(cmd.Process.Pid), filepath.Join(directory, "quick.png"), "-wait", "300")
					if data, err := capture.CombinedOutput(); err != nil {
						t.Fatalf("window screenshot: %v; %s", err, data)
					} else {
						t.Log(string(data))
					}
				}
				for _, toolbar := range []bool{true, false} {
					id := fmt.Sprint("selection-", toolbar)
					window := open(id, "selection", toolbar)
					ex, _, _ := style.Call(window, ^uintptr(19))
					ws, _, _ := style.Call(window, ^uintptr(15))
					owner, _, _ := frameUser.NewProc("GetWindow").Call(window, 4)
					visible, _, _ := frameUser.NewProc("IsWindowVisible").Call(window)
					if ex&0x80 == 0 || ex&0x40000 != 0 || ex&8 == 0 || owner == 0 || visible == 0 {
						t.Fatalf("selection toolbar=%v ex=%#x owner=%x visible=%d", toolbar, ex, owner, visible)
					}
					if (ws&0x40000 == 0) != toolbar {
						t.Fatalf("resize policy toolbar=%v style=%#x", toolbar, ws)
					}
					if !toolbar {
						var client struct{ Left, Top, Right, Bottom int32 }
						frameUser.NewProc("GetClientRect").Call(window, uintptr(unsafe.Pointer(&client)))
						x, y := int32(client.Right/2), int32(1)
						// v0.7.3 implements the top band as a layered child
						// above the canvas. Hit-test the window that actually
						// receives the mouse, not the parent's HTCLIENT area.
						edge, _, _ := frameUser.NewProc("ChildWindowFromPointEx").Call(window, uintptr(uint32(x))|uintptr(uint32(y))<<32, 0)
						point := struct{ X, Y int32 }{x, y}
						frameUser.NewProc("ClientToScreen").Call(window, uintptr(unsafe.Pointer(&point)))
						hit, _, _ := frameUser.NewProc("SendMessageW").Call(edge, 0x84, 0, uintptr(uint16(point.X))|uintptr(uint16(point.Y))<<16)
						if edge == 0 || edge == window || hit != 12 {
							t.Fatalf("top edge is not resizable: child=%x hit=%d", edge, hit)
						}
					}
					send(resultwindow.Command{Kind: "close", ID: id})
					await("results.closed", id)
				}
				send(resultwindow.Command{Kind: "read-selection", ID: "quick", Request: "read-empty"})
				selected := await("results.selected", "quick")
				if selected["request"] != "read-empty" || selected["text"] != "" {
					t.Fatal("unexpected selection", selected)
				}
				// Exercise the production learning route and real bundled dictionary
				// for all four input classes without using any account or AI network.
				for i, text := range []string{"went", "take care", "Sustainable development needs coordination.", strings.Repeat("Sustainable development needs coordination and investment. ", 15)} {
					id := fmt.Sprintf("learning-%d", i)
					send(resultwindow.Command{Kind: "open", Initial: &resultwindow.Initial{ID: id, Token: "probe-token", Pipe: pipe, HostPID: uint32(os.Getpid()), Mode: "chat", Source: "selection", Action: "learn", Text: text, Language: language, Theme: theme}})
					await("results.ready", id)
					select {
					case request := <-h.requests:
						var params struct {
							Session, System string
							Messages        []ai.Message
						}
						if err := json.Unmarshal(request.Params, &params); err != nil {
							t.Fatal(err)
						}
						wantMessages := 1
						if i >= 2 {
							wantMessages = 2
						}
						if request.Method != "ai.generate.start" || params.Session != id || !strings.Contains(params.System, "English learning tutor") || len(params.Messages) != wantMessages {
							t.Fatalf("learning route: %s %+v", request.Method, params)
						}
						if i == 0 && !strings.Contains(params.Messages[0].Content, `"word":"go"`) {
							t.Fatal("bundled inflection lookup missing")
						}
						if i >= 2 && (strings.Contains(params.Messages[0].Content, `"word":"coordination"`) || !strings.Contains(params.Messages[0].Content, `"word":"sustainable"`) || !strings.Contains(params.Messages[1].Content, `"focus":["sustainable"]`) || !strings.Contains(params.Messages[1].Content, `"ignore":["coordination"]`)) {
							t.Fatal("production learning route did not apply saved term states")
						}
					case <-ctx.Done():
						t.Fatal("learning route timed out")
					}
					send(resultwindow.Command{Kind: "close", ID: id})
					await("results.closed", id)
				}
				send(resultwindow.Command{Kind: "quit"})
				stdin.Close()
				select {
				case <-exited:
					if exitErr != nil {
						t.Fatalf("quit: %v; %s", exitErr, diagnostics.String())
					}
				case <-ctx.Done():
					t.Fatal("result process did not exit")
				}
				if err := os.WriteFile(filepath.Join(directory, "process.log"), diagnostics.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// Keep the startup probe independently runnable: unlike the window probe it
// never opens UI. The nonce and reported path must describe this build.
func TestNativeResultStartupProbe(t *testing.T) {
	exe := os.Getenv("WINONE_RESULT_PROBE_EXE")
	if exe == "" {
		t.Skip("requires built result executable")
	}
	cmd := exec.Command(exe, "--check-startup", "result-migration-probe")
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Protocol          int
		Nonce, Executable string
		PID               int
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Protocol != 1 || report.Nonce != "result-migration-probe" || report.PID <= 0 || !sameProbePath(report.Executable, exe) {
		t.Fatalf("bad startup receipt: %s", data)
	}
}

func sameProbePath(a, b string) bool {
	x, e1 := os.Stat(a)
	y, e2 := os.Stat(b)
	return e1 == nil && e2 == nil && os.SameFile(x, y)
}

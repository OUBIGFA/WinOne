//go:build windows

package mcpapp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func TestMain(m *testing.M) {
	if os.Getenv("WINONE_MCP_APPS_NATIVE") != "1" {
		os.Exit(m.Run())
	}
	directory := os.Getenv("WINONE_MCP_APPS_NATIVE_PROFILE")
	if !filepath.IsAbs(directory) || !strings.Contains(filepath.Clean(directory), string(filepath.Separator)+"_temp"+string(filepath.Separator)) {
		fmt.Fprintln(os.Stderr, "MCP App native system check needs its explicit repository-private profile")
		os.Exit(1)
	}
	mygo.App.SetName("WinOne MCP App Native Check")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(directory, "native-browser"))
	mygo.App.OnWindowAllClosed(func() {})
	code := 1
	mygo.App.WhenReady(func() { go func() { code = m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

type nativeFixture struct{ BaseURL, Key, SessionID, ClientID, PluginID string }

func startNativeFixture(t *testing.T, frameOrigins ...string) nativeFixture {
	t.Helper()
	repository, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp(os.Getenv("WINONE_MCP_APPS_NATIVE_PROFILE"), "host-")
	if err != nil {
		t.Fatal(err)
	}
	bun := os.Getenv("WINONE_MCP_APPS_BUN")
	if bun == "" {
		bun = filepath.Join(repository, "dist", "resources", "tools", "bun-1.4.2", "bun.exe")
	}
	command := exec.Command(bun, filepath.Join(repository, "modules", "chat-pi", "ghost", "fixtures", "mcpapp-native-host.mjs"), directory)
	command.Dir = repository
	if len(frameOrigins) > 0 {
		command.Env = append(os.Environ(), "WINONE_MCP_APPS_NATIVE_FRAME_ORIGIN="+frameOrigins[0])
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := os.Create(filepath.Join(directory, "fixture-stderr.log"))
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = diagnostic
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		_ = stdin.Close()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			_ = command.Process.Kill()
			<-done
		}
		_ = diagnostic.Close()
	})
	line := make(chan []byte, 1)
	go func() { reader := bufio.NewReader(stdout); bytes, _ := reader.ReadBytes('\n'); line <- bytes }()
	var metadata nativeFixture
	select {
	case bytes := <-line:
		if json.Unmarshal(bytes, &metadata) != nil || metadata.ClientID == "" {
			t.Fatalf("production fixture did not publish metadata; see %s", diagnostic.Name())
		}
	case <-time.After(45 * time.Second):
		t.Fatalf("production fixture startup did not finish; see %s", diagnostic.Name())
	}
	return metadata
}
func (f nativeFixture) request(path string, input any, out any) error {
	var body io.Reader
	method := "GET"
	if input != nil {
		bytes, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(bytes))
		method = "POST"
	}
	req, err := http.NewRequest(method, f.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+f.Key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		bytes, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		return fmt.Errorf("fixture response %d: %s", response.StatusCode, bytes)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}

// ExecuteScript's SDK callback is HRESULT + LPCWSTR, unlike an event's two
// interface pointers. Sharing its Invoke signature with events loses both the
// error-code width and the UTF-16 pointer type at the native callback boundary.
type nativeScriptCompletion struct {
	comHandler
	complete func(int32, *uint16)
}

var scriptCompletionInvoke = syscall.NewCallback(func(self *nativeScriptCompletion, status int32, text *uint16) uintptr {
	handlers.Lock()
	live := handlers.all[&self.comHandler] != nil
	handlers.Unlock()
	if live {
		self.complete(status, text)
	}
	return 0
})

func newNativeScriptCompletion(complete func(int32, *uint16)) *nativeScriptCompletion {
	table := &comEventVTable{comVTable: handlerTable.comVTable, invoke: scriptCompletionInvoke}
	completed := &nativeScriptCompletion{comHandler: comHandler{
		comObject: comObject{table: &table.comVTable}, iid: mustGUID("49511172-cc67-4bca-9923-137112f4c4cc"),
	}, complete: complete}
	completed.refs.Store(1)
	handlers.Lock()
	handlers.all[&completed.comHandler] = &completed.comHandler
	handlers.Unlock()
	return completed
}

// Native COM ExecuteScript is used only by this explicit test to observe the
// actual App frame. Same-origin policy prevents the proxy from doing this.
func evalApp(page *Page, expression string) (json.RawMessage, error) {
	return evalNativeFrame(page, expression, true, "")
}
func evalNativeFrame(page *Page, expression string, root bool, suffix string) (json.RawMessage, error) {
	result := make(chan struct {
		value json.RawMessage
		err   error
	}, 1)
	mygo.RunOnMain(func() {
		policy, ok := page.policy.(*nativePolicy)
		if !ok {
			result <- struct {
				value json.RawMessage
				err   error
			}{err: errors.New("native policy missing")}
			return
		}
		var frame *comObject
		for item := range policy.frames {
			if item.root == root && !item.closed && (root && item.rootLoaded || !root && strings.HasSuffix(item.address, suffix)) {
				frame = item.frame2
				break
			}
		}
		if frame == nil {
			result <- struct {
				value json.RawMessage
				err   error
			}{err: errors.New("actual App frame missing")}
			return
		}
		completed := newNativeScriptCompletion(func(status int32, text *uint16) {
			if status < 0 {
				result <- struct {
					value json.RawMessage
					err   error
				}{err: fmt.Errorf("App script HRESULT %#x", uint32(status))}
				return
			}
			value := windows.UTF16PtrToString(text)
			result <- struct {
				value json.RawMessage
				err   error
			}{value: json.RawMessage(value)}
		})
		script, err := windows.UTF16PtrFromString(expression)
		if err != nil {
			handlerRef(&completed.comHandler, -1)
			result <- struct {
				value json.RawMessage
				err   error
			}{err: err}
			return
		}
		status := com(frame, 19, uintptr(unsafe.Pointer(script)), uintptr(unsafe.Pointer(&completed.comObject)))
		handlerRef(&completed.comHandler, -1)
		if failedCOM(status) {
			result <- struct {
				value json.RawMessage
				err   error
			}{err: fmt.Errorf("native App eval HRESULT %#x", status)}
		}
	})
	select {
	case reply := <-result:
		return reply.value, reply.err
	case <-time.After(8 * time.Second):
		return nil, errors.New("native App eval timeout")
	}
}
func awaitNative(t *testing.T, check func() bool, label string) {
	t.Helper()
	until := time.Now().Add(12 * time.Second)
	for time.Now().Before(until) {
		if check() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s did not finish", label)
}
func appBool(page *Page, expression string) bool {
	value, err := evalApp(page, expression)
	return err == nil && string(value) == "true"
}

func TestNativeMcpAppEndToEndUsesActualPluginBackendAndOfficialApp(t *testing.T) {
	if os.Getenv("WINONE_MCP_APPS_NATIVE") != "1" {
		t.Skip("explicit isolated WebView2/plugin system check")
	}
	defer i18n.SetLocale(i18n.Chinese)
	fixture := startNativeFixture(t)
	transport := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		var result any
		err := fixture.request("/call", map[string]any{"method": method, "params": json.RawMessage(params)}, &result)
		return result, err
	})
	var page *Page
	dark := false
	window := mygo.NewWindow(mygo.WindowOptions{Title: "WinOne standard MCP App check", Width: 820, Height: 740, Content: ui.View(func(c *ui.Context) {
		c.SetTheme(kit.Theme(c.Preferences(), dark))
		ui.Column(c).Fill().Padding(8).Children(func() {
			if page != nil {
				page.View(c)
			}
		})
	})})
	defer window.Destroy()
	transport.Env.Post = func(fn func()) {
		if !window.IsDestroyed() {
			window.Update(fn)
		}
	}
	transport.Env.WebViews = window.NewWebView
	mygo.RunOnMain(func() {
		var err error
		page, err = New(transport.Env, fixture.SessionID, fixture.ClientID, "Official App fixture")
		if err != nil {
			t.Error(err)
		}
		window.Show()
	})
	if page == nil {
		t.Fatal("native App was not created")
	}
	defer func() {
		page.Close()
		select {
		case <-page.Done():
		case <-time.After(12 * time.Second):
			t.Error("native App Close did not reclaim its View")
		}
	}()
	awaitNative(t, func() bool { return page.State().Initialized || page.Err() != nil }, "official App initialization")
	if page.Err() != nil {
		t.Fatal(page.Err())
	}
	awaitNative(t, func() bool {
		return appBool(page, `window.fx?.ready&&window.fx.input?.arguments.title==='Native initial'&&window.fx.results[0]?._meta.private==='FIXTURE_UI_ONLY_NOT_FOR_MODEL'`)
	}, "original input and private UI result")
	var state struct {
		ModelResult  json.RawMessage
		Cards        []any
		Messages     []map[string]any
		Downloaded   *string
		ModelContext string
		Diagnostics  []string
	}
	if err := fixture.request("/state", nil, &state); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(state.ModelResult, []byte("FIXTURE_UI_ONLY_NOT_FOR_MODEL")) {
		t.Fatal("UI private _meta entered the actual model result")
	}
	if len(state.Cards) != 1 {
		t.Fatal("original ghost_call did not create exactly one App card")
	}

	run := func(op string, condition string) {
		t.Helper()
		if _, err := evalApp(page, `void window.fx.run(`+strconvQuote(op)+`)`); err != nil {
			t.Fatal(err)
		}
		awaitNative(t, func() bool { return appBool(page, condition) }, op)
	}
	run("files", `window.fx.file==='name,value\nalpha,42\n'`)
	run("read", `window.fx.resource?.contents[0].mimeType==='text/html;profile=mcp-app'`)
	if _, err := evalApp(page, `document.querySelector('#value').value='Actual native App choice'`); err != nil {
		t.Fatal(err)
	}
	run("save", `window.fx.saved?.structuredContent.value==='Actual native App choice'`)
	if appBool(page, `window.fx.saved?.structuredContent.reservedReceived!==null`) {
		t.Fatal("App supplied a forged reserved handoff argument")
	}
	run("cancel", `window.fx.cancelled===true&&window.fx.events.some(e=>e.type==='progress')`)
	run("context", `window.fx.contextUpdated===true`)
	run("message", `window.fx.messageSent===true`)
	run("size", `window.fx.sizeSent===true`)
	awaitNative(t, func() bool { return page.State().Height == 650 }, "original native bounded size")
	mygo.RunOnMain(func() { i18n.SetLocale(i18n.English); window.Update(func() { dark = true }) })
	awaitNative(t, func() bool {
		return appBool(page, `window.fx.app.getHostContext().theme==='dark'&&window.fx.app.getHostContext().locale==='en-US'&&!!window.fx.app.getHostContext().styles?.variables['--font-sans']&&!!window.fx.app.getHostContext().timeZone`)
	}, "real native theme/locale/environment context")
	run("download", `window.fx.download?.isError!==true&&window.fx.events.some(e=>e.type==='download')`)
	if err := fixture.request("/state", nil, &state); err != nil {
		t.Fatal(err)
	}
	if state.Downloaded == nil || *state.Downloaded != "name,value\nalpha,42\n" {
		t.Fatal("official resource download was not written to fixture destination")
	}
	if !strings.Contains(state.ModelContext, "Context from actual App") || len(state.Messages) != 1 {
		t.Fatal("actual App context/message did not reach production backend")
	}
	if len(state.Cards) != 1 {
		t.Fatal("an app-only tool created a duplicate App card")
	}

	run("private-bridge", `window.fx.bridgeDenied===true`)
	if _, err := evalApp(page, `window.fx.crossOriginDenied=false;try{void parent.location.href;}catch{window.fx.crossOriginDenied=true;}`); err != nil {
		t.Fatal(err)
	}
	if !appBool(page, `window.fx.crossOriginDenied===true`) {
		t.Fatal("App and proxy shared an origin")
	}
	if _, err := evalApp(page, `parent.postMessage({jsonrpc:'2.0',method:'ui/notifications/host-context-changed',params:{displayMode:'fullscreen'}},'*')`); err != nil {
		t.Fatal(err)
	}
	if page.State().DisplayMode != "inline" {
		t.Fatal("guest notification forged Host display mode")
	}
	run("fullscreen", `window.fx.mode==='fullscreen'`)
	awaitNative(t, func() bool { return page.State().DisplayMode == "fullscreen" }, "standard display mode notification")
	if _, err := evalApp(page, `location.href='https://not-declared.example.invalid/escape'`); err != nil {
		t.Fatal(err)
	}
	awaitNative(t, func() bool { return appBool(page, `location.pathname==='/resource'&&window.fx.ready`) }, "native root iframe navigation denial")

	page.mu.Lock()
	old := page.session
	page.mu.Unlock()
	page.Close()
	select {
	case <-page.Done():
	case <-time.After(12 * time.Second):
		t.Fatal("native teardown did not finish")
	}
	request, _ := http.NewRequest("GET", old.Bridge.BaseURL+"/resource", nil)
	request.Header.Set("Authorization", "Bearer "+old.Bridge.Key)
	response, err := nativeHTTP.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("closed View retained native HTML capability")
	}
	mygo.RunOnMain(func() {
		var err error
		page, err = New(transport.Env, fixture.SessionID, fixture.ClientID, "Reopened official App")
		if err != nil {
			t.Error(err)
		}
	})
	awaitNative(t, func() bool { return page.State().Initialized || page.Err() != nil }, "same card re-open")
	if page.Err() != nil {
		t.Fatal(page.Err())
	}
	awaitNative(t, func() bool {
		return appBool(page, `window.fx.ready&&window.fx.input?.arguments.title==='Native initial'`)
	}, "re-opened original input")
	if err := fixture.request("/state", nil, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Cards) != 1 {
		t.Fatal("reopen replayed or duplicated the original card")
	}
}
func strconvQuote(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }

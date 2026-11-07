//go:build windows

package globalpages

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/plugins/automation"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/kit"
)

func TestMain(m *testing.M) {
	if os.Getenv("WINONE_NATIVE_GLOBAL_PAGES") != "1" {
		os.Exit(m.Run())
	}
	profile := os.Getenv("WINONE_NATIVE_UI_REAL_PROFILE")
	if !filepath.IsAbs(profile) || !strings.Contains(filepath.ToSlash(profile), "/_temp/") {
		fmt.Fprintln(os.Stderr, "use the isolated native UI matrix profile")
		os.Exit(1)
	}
	mygo.App.SetName("WinOne Native Plugin Containers Check")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(profile, "native-page-browser"))
	mygo.App.OnWindowAllClosed(func() {})
	code := 1
	mygo.App.WhenReady(func() { go func() { code = m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

// Real Host, installed manifests, page-session capabilities and WebView2/MyGo
// windows. Only fixture HTML is deterministic; there is no fake Host service,
// snapshot image, DOM-reported focus or substitute page implementation.
func TestNativeGlobalPluginContainersWithRealHostAndWebViews(t *testing.T) {
	if os.Getenv("WINONE_NATIVE_GLOBAL_PAGES") != "1" {
		t.Skip("explicit private desktop/native Host entry")
	}
	client, err := host.NewClient(os.Getenv("WINONE_NATIVE_UI_REAL_PIPE"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cancel()
	profile := os.Getenv("WINONE_NATIVE_UI_REAL_PROFILE")
	phase := os.Getenv("WINONE_NATIVE_PAGES_PHASE")
	if phase == "" {
		phase = "all"
	}
	call := func(method string, params, out any) {
		t.Helper()
		if err := client.CallInto(method, params, out); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	instances := map[string]string{}
	install := func(id, position string, main, adaptive bool, mainOptions ...map[string]any) {
		t.Helper()
		manifest := map[string]any{"schemaVersion": 3, "id": id, "name": id, "description": "Isolated native containers fixture", "version": "1.0.0", "minCindyVersion": "1.2.3", "entry": "main.js", "panel": map[string]any{"title": id + " panel", "html": "panel.html", "position": position}, "settingsHtml": "settings.html"}
		if main {
			view := map[string]any{"title": id + " app", "html": "main.html", "icon": "database", "open": "page"}
			for _, options := range mainOptions {
				for key, value := range options {
					view[key] = value
				}
			}
			manifest["mainView"] = view
			manifest["taskView"] = map[string]any{"title": id + " task", "html": "task.html"}
		}
		if !adaptive {
			manifest["settingsHeight"] = 360
		}
		data, _ := json.Marshal(manifest)
		files := map[string][]byte{"ghost.json": data, "main.js": []byte(`const nativeChannel=new BroadcastChannel('native-containers');nativeChannel.onmessage=async event=>{const input=event.data;if(input?.marker!=='native-capability-check')return;let result;if(input.op==='task')result=await cindy.taskView({sessionId:input.sessionId,focus:input.focus});else result={ok:true};nativeChannel.postMessage({requestId:input.requestId,result})};`),
			"panel.html":    []byte(`<!doctype html><body><button id="inside">Native panel content</button><script src="page.js"></script></body>`),
			"main.html":     []byte(`<!doctype html><body><button id="inside">Native app content</button><script src="page.js"></script></body>`),
			"task.html":     []byte(`<!doctype html><body><div id="task">Task content</div><script src="page.js"></script></body>`),
			"settings.html": []byte(`<!doctype html><body style="margin:0"><div id="content" style="height:80px">Adaptive native settings</div><script src="page.js"></script></body>`),
			"page.js":       []byte(`const nativeChannel=new BroadcastChannel('native-containers');const replies=new Map();nativeChannel.onmessage=event=>{const reply=event.data;if(replies.has(reply?.requestId)){replies.get(reply.requestId)(reply.result);replies.delete(reply.requestId)}};window.nativeCapabilityRequest=input=>new Promise(resolve=>{const requestId=crypto.randomUUID();replies.set(requestId,resolve);nativeChannel.postMessage({...input,marker:'native-capability-check',requestId})});document.documentElement.dataset.realPage='ready';`)}
		var raw bytes.Buffer
		archive := zip.NewWriter(&raw)
		for name, data := range files {
			entry, err := archive.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = entry.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err = archive.Close(); err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(filepath.Dir(profile), id+".cindy")
		if err = os.WriteFile(name, raw.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		var inspected struct{ Token string }
		call("chat.ghosts.inspect", map[string]string{"path": name, "language": "en-US"}, &inspected)
		var installed struct {
			PluginID string `json:"pluginId"`
		}
		call("chat.ghosts.install", map[string]any{"token": inspected.Token, "confirmed": true}, &installed)
		instances[id] = installed.PluginID
	}
	for _, side := range sides {
		install("native-"+side, side, side == "left", side == "left")
	}
	install("native-exclusive", "tab", false, false)
	var rows []Declaration
	call("chat.ghosts.pages", map[string]string{"language": "en-US"}, &rows)
	var window *mygo.Window
	var settings *SettingsPage
	var route *routePage
	rects := map[string]ui.Rect{}
	intents := map[string][]core.Intent{}
	var jobs atomic.Int64
	settingsMode := false
	env := &core.Env{Host: client, Directory: profile, Activity: &core.Activity{}, Rail: &core.Rail{}, Refresh: func() {}, Toast: func(string) {}, Focuses: func() int { return 0 }, TakeIntents: func(string) []core.Intent { return nil }, Navigate: func(string, func()) {}, Open: func(string, core.Intent) {}}
	env.Post = func(fn func()) {
		if window != nil && !window.IsDestroyed() {
			window.Update(fn)
		}
	}
	env.RequestChange = func(fn func()) { fn() }
	env.Spawn = func(fn func()) { jobs.Add(1); go func() { defer jobs.Add(-1); fn() }() }
	env.TakeIntents = func(name string) []core.Intent { value := intents[name]; delete(intents, name); return value }
	env.Open = func(name string, intent core.Intent) {
		if route != nil {
			route.Close()
		}
		intents[name] = append(intents[name], intent)
		if name == MainRoute {
			route = NewMainPage(env).(*routePage)
		} else {
			route = NewPanelPage(env).(*routePage)
		}
		settingsMode = false
		window.Invalidate()
	}
	env.Navigate = func(_ string, apply func()) {
		if route != nil {
			route.Close()
			route = nil
		}
		settingsMode = false
		if apply != nil {
			apply()
		}
		window.Invalidate()
	}
	env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) { return window.NewWebView(options) }
	controller := For(env)
	mygo.RunOnMain(func() {
		controller.loaded, controller.language = true, "en-US"
		controller.Apply(rows)
		window = mygo.NewWindow(mygo.WindowOptions{Title: "Native plugin containers", Width: 1400, Height: 900, Hidden: true, Content: ui.View(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), false))
			if settingsMode {
				settings.View(c)
				return
			}
			if route != nil && !route.closed {
				route.View(c)
				return
			}
			ui.Row(c).Fill().Children(func() {
				ui.Column(c).Width(180).FillHeight().Children(func() { controller.Navigation(c, false) })
				ui.Column(c).Grow(1).Basis(0).FillHeight().MinWidth(0).Children(func() {
					controller.Layout(c, true, func() { ui.Column(c).Fill().Label("Real native conversation area") })
					for side, handle := range controller.groups {
						rects[side] = handle.Bounds(c)
					}
				})
			})
		})})
		controller.root = window
		window.Show()
		window.Focus()
	})
	defer mygo.RunOnMain(func() { controller.Close(); window.Destroy() })
	await := func(label string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			ok := false
			mygo.RunOnMain(func() { ok = check() })
			if ok {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		var settingsDocument *mygo.Page
		mygo.RunOnMain(func() {
			t.Logf("declarations=%d tabs=%+v error=%q jobs=%d rootFocused=%v rects=%v", len(controller.state.declarations), controller.state.Tabs, controller.err, jobs.Load(), window.IsFocused(), rects)
			for id, body := range controller.bodies {
				t.Logf("body %s: loading=%v page=%v visible=%v error=%q focused=%v", id, body.loading, body.page != nil, body.visible, body.err, body.page != nil && body.page.IsFocused())
			}
			if settings != nil && settings.body != nil && settings.body.page != nil {
				t.Logf("settings height=%v fixed=%v observer=%v error=%q", settings.Height(), settings.body.decl.SettingsHeight, settings.body.heightCancel != nil, settings.body.err)
				settingsDocument = settings.body.page.WebView().Page()
			}
		})
		if settingsDocument != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			value, err := settingsDocument.EvalContext(ctx, `return {state:document.readyState,prepared:!!document.getElementById('__winone_settings_layout'),last:window.__winoneSettingsLastHeight,body:document.body.getBoundingClientRect().bottom,content:document.getElementById('content').getBoundingClientRect().bottom,viewport:innerHeight}`)
			cancel()
			t.Logf("actual settings geometry=%v Eval=%v", value, err)
		}
		t.Fatalf("%s did not settle", label)
	}
	// IsLoading initially reports false before WebView2 begins navigation.
	// An actual committed URL plus a complete document and the fixture's own
	// script marker prove readiness; a late listener may miss DidFinishLoad.
	awaitDocument := func(label, route string, body func() *pageBody) *mygo.Page {
		t.Helper()
		var page *mygo.Page
		await(label+" native guest", func() bool {
			current := body()
			if current == nil || current.page == nil {
				return false
			}
			page = current.page.WebView().Page()
			return true
		})
		var finished atomic.Bool
		off := page.OnDidFinishLoad(func() { finished.Store(true) })
		defer off()
		deadline := time.Now().Add(15 * time.Second)
		var value any
		var err error
		for time.Now().Before(deadline) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			value, err = page.EvalContext(ctx, `return {url:location.href,state:document.readyState,marker:document.documentElement.dataset.realPage,compat:document.compatMode}`)
			cancel()
			if state, ok := value.(map[string]any); err == nil && ok && state["state"] == "complete" && state["marker"] == "ready" && state["url"] == page.URL() && strings.HasSuffix(strings.Split(page.URL(), "?")[0], "/"+route) {
				t.Logf("%s ready: DidFinishLoad observed=%v URL=%s DOM=%v", label, finished.Load(), page.URL(), state)
				return page
			}
			time.Sleep(20 * time.Millisecond)
		}
		mygo.RunOnMain(func() {
			current := body()
			if current != nil {
				t.Logf("%s mount: visible=%v closed=%v loading=%v error=%q", label, current.visible, current.closed, current.loading, current.err)
			}
		})
		t.Fatalf("%s document not ready: DidFinishLoad=%v nativeURL=%q DOM=%v Eval=%v", label, finished.Load(), page.URL(), value, err)
		return nil
	}
	bodyOf := func(id string) *pageBody { return controller.bodies[tabID(instances[id], "")] }
	for _, side := range sides {
		if phase == "all" || phase == "layout" || phase == "gesture" {
			awaitDocument(side, "__page_panel__", func() *pageBody { return bodyOf("native-" + side) })
		}
	}
	mygo.RunOnMain(func() {
		if controller.state.findTab(tabID(instances["native-exclusive"], "")) != nil {
			t.Error("exclusive panel appeared globally")
		}
	})
	// Detach into a real native OS window, merge another tab, then return it.
	leftID, rightID := tabID(instances["native-left"], ""), tabID(instances["native-right"], "")
	if phase == "all" || phase == "layout" {
		mygo.RunOnMain(func() { controller.detach(leftID) })
		awaitDocument("detached", "__page_panel__", func() *pageBody {
			w := controller.windows[leftID]
			if w == nil {
				return nil
			}
			return w.bodies[leftID]
		})
		mygo.RunOnMain(func() { controller.joinWindow(rightID, controller.windows[leftID]) })
		await("second real tab in detached window", func() bool {
			w := controller.windows[rightID]
			return w != nil && len(w.tabs) == 2 && w.bodies[rightID] != nil && w.bodies[rightID].page != nil
		})
		mygo.RunOnMain(func() {
			if controller.WindowCount() != 1 {
				t.Error("merge created duplicate native windows")
			}
			controller.Move(rightID, "bottom")
			controller.Move(leftID, "right")
		})
		await("return closes detached OS window", func() bool {
			return controller.WindowCount() == 0 && bodyOf("native-left") != nil && bodyOf("native-left").page != nil
		})
		// Closing releases the actual guest, but reopening keeps the user's edge.
		var closedView *mygo.WebView
		mygo.RunOnMain(func() { closedView = bodyOf("native-left").page.WebView() })
		nativeWindowAction(t, window, tr("closeTab", "name", "native-left panel"))
		await("closed guest released", func() bool { return closedView.IsDestroyed() && controller.state.findTab(leftID) == nil })
		mygo.RunOnMain(func() { controller.Open(instances["native-left"], "panel") })
		awaitDocument("reopened edge", "__page_panel__", func() *pageBody { return bodyOf("native-left") })
		mygo.RunOnMain(func() {
			if tab := controller.state.findTab(leftID); tab == nil || tab.Location.Side != "right" {
				t.Error("reopen forgot the user's edge")
			}
		})
		for _, edge := range sides {
			mygo.RunOnMain(func() { controller.Move(leftID, edge) })
			awaitDocument("move to "+edge, "__page_panel__", func() *pageBody { return bodyOf("native-left") })
		}
		mygo.RunOnMain(func() { controller.Move(leftID, "right") })
		awaitDocument("returned right edge", "__page_panel__", func() *pageBody { return bodyOf("native-left") })
		// The real native maximize/restore controls keep other groups parked.
		nativeWindowAction(t, window, tr("maximize"))
		await("native maximized panel", func() bool { return controller.maximized != "" })
		nativeWindowAction(t, window, tr("maximize"))
		await("native restored groups", func() bool { return controller.maximized == "" })
	}
	if phase == "gesture" {
		mygo.RunOnMain(func() { controller.Move(leftID, "right") })
		awaitDocument("gesture source at right", "__page_panel__", func() *pageBody { return bodyOf("native-left") })
	}
	if phase == "layout" {
		t.Log("real four edges, native detach/merge/return, close/reopen/move/maximize/restore passed")
		return
	}

	if phase == "geometry" {
		// Exercise the production StateKey/default store and exact HWND bounds,
		// not a copy of MyGo's persistence algorithm or a remembered Go value.
		mygo.RunOnMain(func() { controller.detach(leftID) })
		awaitDocument("geometry native window", "__page_panel__", func() *pageBody {
			if w := controller.windows[leftID]; w != nil {
				return w.bodies[leftID]
			}
			return nil
		})
		var standalone *mygo.Window
		var oldGuest *mygo.WebView
		mygo.RunOnMain(func() {
			standalone = controller.windows[leftID].win
			oldGuest = controller.windows[leftID].bodies[leftID].page.WebView()
			standalone.SetSize(640, 480)
			standalone.SetPosition(80, 80)
		})
		await("native move/resize", func() bool {
			x, y := standalone.Position()
			w, h := standalone.Size()
			return x == 80 && y == 80 && w == 640 && h == 480
		})
		wanted := nativeWindowRect(t, standalone)
		mygo.RunOnMain(standalone.Close)
		await("geometry window and guest closed", func() bool {
			return standalone.IsDestroyed() && oldGuest.IsDestroyed() && controller.windows[leftID] == nil
		})
		mygo.RunOnMain(func() { controller.Open(instances["native-left"], "panel") })
		reopened := func() *pageBody {
			if w := controller.windows[leftID]; w != nil {
				return w.bodies[leftID]
			}
			return nil
		}
		awaitDocument("geometry close/reopen", "__page_panel__", reopened)
		checkBounds := func(label string) {
			t.Helper()
			var current *mygo.Window
			mygo.RunOnMain(func() { current = controller.windows[leftID].win })
			if got := nativeWindowRect(t, current); got != wanted {
				t.Fatalf("%s lost actual HWND position/size: got=%+v want=%+v", label, got, wanted)
			}
			t.Logf("%s exact HWND bounds: %+v", label, wanted)
		}
		checkBounds("reopen")
		mygo.RunOnMain(func() {
			controller.Close()
			controller = For(env)
			controller.root, controller.loaded, controller.language = window, true, "en-US"
			controller.Apply(rows)
		})
		awaitDocument("geometry new controller reads persisted placement", "__page_panel__", reopened)
		checkBounds("new controller")
		mygo.RunOnMain(func() {
			standalone = controller.windows[leftID].win
			oldGuest = controller.windows[leftID].bodies[leftID].page.WebView()
		})
		call("chat.ghosts.enabled", map[string]any{"pluginId": instances["native-left"], "enabled": false}, nil)
		call("chat.ghosts.pages", map[string]string{"language": "en-US"}, &rows)
		mygo.RunOnMain(func() { controller.Apply(rows) })
		await("disabled native window and guest released", func() bool {
			return standalone.IsDestroyed() && oldGuest.IsDestroyed() && controller.WindowCount() == 0
		})
		call("chat.ghosts.enabled", map[string]any{"pluginId": instances["native-left"], "enabled": true}, nil)
		call("chat.ghosts.pages", map[string]string{"language": "en-US"}, &rows)
		mygo.RunOnMain(func() { controller.Apply(rows) })
		awaitDocument("enabled window restored without guest snapshot", "__page_panel__", reopened)
		checkBounds("enable")
		return
	}

	if phase == "all" || phase == "gesture" {
		// Real native input, the current plugin page binding and the original SDK
		// enforce focus:true. Focus/DOM click do not mint, one click consumes once,
		// and a ticket deliberately aged past the documented 3s lifetime expires.
		var sourcePage *mygo.Page
		var sourceBounds ui.Rect
		mygo.RunOnMain(func() {
			controller.focusTab(*controller.state.findTab(leftID))
			sourcePage = bodyOf("native-left").page.WebView().Page()
			window.Focus()
			bodyOf("native-left").Focus()
		})
		await("actual source focus/attachment", func() bool {
			// Restore may have parked this guest in the preceding frame. Ask for
			// native focus only after its real placement becomes visible, and wait
			// for the native focus fact; a focus request grants no gesture.
			window.Focus()
			bodyOf("native-left").Focus()
			return jobs.Load() == 0 && bodyOf("native-left").page.IsFocused() && rects["right"].H > 80
		})
		mygo.RunOnMain(func() { sourceBounds = rects["right"] })
		sessionIds := []string{}
		for index := range 2 {
			directory := filepath.Join(filepath.Dir(profile), fmt.Sprint("native-task-", index))
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			var created struct{ ID string }
			call("chat.conversation.new", map[string]string{"directory": directory, "title": "Native task view owner"}, &created)
			if created.ID == "" {
				t.Fatal("real task owner was not created")
			}
			sessionIds = append(sessionIds, created.ID)
		}
		call("chat.ghosts.session-focus", map[string]string{"sessionId": sessionIds[0]}, nil)
		mygo.RunOnMain(func() { controller.Session(sessionIds[0]) })
		request := map[string]any{"op": "task", "sessionId": sessionIds[1], "focus": true}
		if reply := nativePanelRequest(t, sourcePage, request); reply["ok"] != false || reply["errorCode"] != "PERMISSION_DENIED" {
			t.Fatalf("native focus alone minted a gesture: %v", reply)
		}
		if _, err := sourcePage.Eval(`document.getElementById('inside').click();return true`); err != nil {
			t.Fatal(err)
		}
		if reply := nativePanelRequest(t, sourcePage, request); reply["ok"] != false || reply["errorCode"] != "PERMISSION_DENIED" {
			t.Fatalf("DOM click minted a gesture: %v", reply)
		}
		nativePanelClick(t, window, sourcePage, sourceBounds)
		await("real OS gesture delivered", func() bool { return jobs.Load() == 0 })
		if reply := nativePanelRequest(t, sourcePage, request); reply["ok"] != true || reply["focusRequested"] != true || reply["sessionId"] != sessionIds[1] {
			t.Fatalf("actual OS gesture was not accepted: %v", reply)
		}
		if reply := nativePanelRequest(t, sourcePage, request); reply["ok"] != false || reply["errorCode"] != "PERMISSION_DENIED" {
			t.Fatalf("a gesture was replayed: %v", reply)
		}
		nativePanelClick(t, window, sourcePage, sourceBounds)
		await("expiry gesture delivered", func() bool { return jobs.Load() == 0 })
		expires := time.NewTimer(3100 * time.Millisecond)
		<-expires.C // Testing the real TTL, not waiting for navigation/render readiness.
		if reply := nativePanelRequest(t, sourcePage, request); reply["ok"] != false || reply["errorCode"] != "PERMISSION_DENIED" {
			t.Fatalf("expired gesture stayed valid: %v", reply)
		}
		var nativeKeys atomic.Uint32
		var offKey func()
		mygo.RunOnMain(func() { offKey = bodyOf("native-left").page.WebView().OnUserGesture(func() { nativeKeys.Add(1) }) })
		defer offKey()
		nativePanelKey(t, window, sourcePage)
		await("real WebView2 keyboard gesture delivered", func() bool { return nativeKeys.Load() > 0 && jobs.Load() == 0 })
		if reply := nativePanelRequest(t, sourcePage, request); reply["ok"] != true || reply["focusRequested"] != true || reply["sessionId"] != sessionIds[1] {
			t.Fatalf("actual OS keyboard gesture was not accepted: %v", reply)
		}
		if reply := nativePanelRequest(t, sourcePage, request); reply["ok"] != false || reply["errorCode"] != "PERMISSION_DENIED" {
			t.Fatalf("a keyboard gesture was replayed: %v", reply)
		}
		var pushes struct {
			Events []struct {
				Channel string
				Payload json.RawMessage
			}
		}
		call("chat.ghosts.pending", map[string]int{"after": 0}, &pushes)
		delivered := false
		for _, event := range pushes.Events {
			var task struct {
				InstanceID, SessionID string
				Focus                 bool
			}
			if event.Channel == "task-view" && json.Unmarshal(event.Payload, &task) == nil && task.InstanceID == instances["native-left"] && task.SessionID == sessionIds[1] && task.Focus {
				delivered = true
			}
		}
		if !delivered {
			t.Fatal("accepted original SDK task view did not produce its real Host push")
		}
		mygo.RunOnMain(func() {
			controller.Session(sessionIds[1])
			controller.OpenTask(instances["native-left"], sessionIds[1])
		})
		taskID := tabID(instances["native-left"], sessionIds[1])
		taskPage := awaitDocument("task owner bucket", "__page_task__", func() *pageBody { return controller.bodies[taskID] })
		if !strings.Contains(taskPage.URL(), "session="+sessionIds[1]) {
			t.Fatal("task guest changed session")
		}
		mygo.RunOnMain(func() {
			controller.OpenTask(instances["native-left"], sessionIds[1])
			controller.detach(taskID)
			controller.Session(sessionIds[0])
		})
		awaitDocument("detached task owner", "__page_task__", func() *pageBody {
			if w := controller.windows[taskID]; w != nil {
				return w.bodies[taskID]
			}
			return nil
		})
		mygo.RunOnMain(func() {
			if controller.state.findTab(taskID).Session != sessionIds[1] {
				t.Error("detached task followed another active task")
			}
			controller.CloseTab(taskID)
		})
		await("detached task released", func() bool { return controller.windows[taskID] == nil })
	}
	if phase == "gesture" {
		t.Log("real native focus rejected DOM/no-gesture, accepted one OS click and OS F8 accelerator, rejected replay/3s expiry and kept detached task session")
		return
	}

	if phase == "all" || phase == "main" {
		// Same-HTML main views keep their declaration's single/multiple policy
		// across actual native Page, sidebar and Window lifecycles.
		for _, multiple := range []bool{false, true} {
			id := "native-single"
			if multiple {
				id = "native-multiple"
			}
			options := map[string]any{"html": "panel.html"}
			// The installed manifest accepts only literal true; omission is the
			// original SDK's single-instance declaration, not multiple:false.
			if multiple {
				options["multiple"] = true
			}
			install(id, "right", true, false, options)
			call("chat.ghosts.pages", map[string]string{"language": "en-US"}, &rows)
			mygo.RunOnMain(func() { controller.Apply(rows); controller.OpenAt(instances[id], "main", Location{Mode: "page"}) })
			awaitDocument(id+" real page", "__page_main__", func() *pageBody {
				if route != nil {
					return route.body
				}
				return nil
			})
			mygo.RunOnMain(func() {
				if (controller.state.findTab(tabID(instances[id], "")) != nil) != multiple {
					t.Error("real main page violated single/multiple")
				}
				controller.OpenAt(instances[id], "main", Location{Mode: "window", Side: "right"})
			})
			mainID := tabID(instances[id], "")
			awaitDocument(id+" actual window", "__page_main__", func() *pageBody {
				if w := controller.windows[mainID]; w != nil {
					return w.bodies[mainID]
				}
				return nil
			})
			var standalone *mygo.Window
			mygo.RunOnMain(func() { standalone = controller.windows[mainID].win; controller.Open(instances[id], "main") })
			mygo.RunOnMain(func() {
				if controller.windows[mainID].win != standalone {
					t.Error("repeat main open created another window")
				}
				standalone.Close()
			})
			await(id+" close native window", func() bool { return standalone.IsDestroyed() && controller.windows[mainID] == nil })
			mygo.RunOnMain(func() { controller.Open(instances[id], "main") })
			awaitDocument(id+" reopen native window", "__page_main__", func() *pageBody {
				if w := controller.windows[mainID]; w != nil {
					return w.bodies[mainID]
				}
				return nil
			})
			mygo.RunOnMain(func() { controller.Move(mainID, "right"); controller.LeaveMain(); route = nil; window.Invalidate() })
			await(id+" window returned", func() bool { return controller.windows[mainID] == nil })
			awaitDocument(id+" real sidebar", "__page_main__", func() *pageBody { return controller.bodies[mainID] })
		}
	}
	if phase == "main" {
		t.Log("real Page/Window single/multiple, repeat focus, close/reopen and return passed")
		return
	}

	// Real adaptive settings read DOM geometry and react to growth/shrink,
	// including malformed/fixed-width content in a narrow native container.
	mygo.RunOnMain(func() {
		settings, _ = NewSettingsPage(env, instances["native-left"])
		settingsMode = true
		window.SetSize(560, 900)
		window.Invalidate()
	})
	defer mygo.RunOnMain(func() { settings.Close() })
	page := awaitDocument("adaptive settings", "__page_settings__", func() *pageBody { return settings.body })
	await("adaptive settings height", func() bool { return settings.Height() < 160 })
	if _, err := page.Eval(`document.getElementById('content').style.height='1000px';return true`); err != nil {
		t.Fatal(err)
	}
	await("adaptive cap 800", func() bool { return settings.Height() == 800 })
	if _, err := page.Eval(`document.getElementById('content').style.height='80px';return true`); err != nil {
		t.Fatal(err)
	}
	await("adaptive shrinks again", func() bool { return settings.Height() <= 100 })
	if _, err := page.Eval(`document.getElementById('content').style.height='20px';document.getElementById('content').style.width='1200px';return true`); err != nil {
		t.Fatal(err)
	}
	await("adaptive minimum 48", func() bool { return settings.Height() == 48 })
	geometry, err := page.Eval(`return {viewport:innerWidth,width:document.getElementById('content').getBoundingClientRect().width,scroll:document.documentElement.scrollWidth}`)
	if err != nil {
		t.Fatal(err)
	}
	measured, ok := geometry.(map[string]any)
	if !ok || measured["width"] != measured["viewport"] || measured["scroll"] != measured["viewport"] {
		t.Fatalf("actual narrow settings escaped its viewport: %v", geometry)
	}
	var oldGuest *mygo.WebView
	mygo.RunOnMain(func() {
		oldGuest = settings.body.page.WebView()
		settings.Close()
		settings, _ = NewSettingsPage(env, instances["native-left"])
		window.Invalidate()
	})
	await("closed adaptive guest destroyed", oldGuest.IsDestroyed)
	page = awaitDocument("adaptive fresh document after reopen", "__page_settings__", func() *pageBody { return settings.body })
	await("reopened content remeasured, not cached snapshot", func() bool { return settings.Height() == 80 })
	if value, err := page.Eval(`return document.getElementById('content').style.height`); err != nil || value != "80px" {
		t.Fatalf("reopen retained old DOM instead of fresh settings: %v %v", value, err)
	}
	mygo.RunOnMain(func() {
		settings.Close()
		settings, _ = NewSettingsPage(env, instances["native-right"])
		window.Invalidate()
	})
	fixed := awaitDocument("fixed settings", "__page_settings__", func() *pageBody { return settings.body })
	if _, err := fixed.Eval(`document.getElementById('content').style.height='1000px';return document.getElementById('content').getBoundingClientRect().height`); err != nil {
		t.Fatal(err)
	}
	mygo.RunOnMain(func() {
		if settings.Height() != 360 || settings.body.heightCancel != nil {
			t.Error("fixed settings height was replaced by an adaptive observer")
		}
	})
	if prepared, err := fixed.Eval(`return !!document.getElementById('__winone_settings_layout')`); err != nil || prepared != false {
		t.Fatalf("fixed declaration rewrote the author's layout: %v %v", prepared, err)
	}
	// Source-cache native handle is real WebView2, not a DOM bridge.
	if handle := fixed.NativeWebViewHandle(); handle == 0 {
		t.Fatal("settings page has no real native WebView handle")
	}
	t.Log("real adaptive DOM height 80→800→80→48, narrow fixed-width reflow, fresh settings reopen and fixed 360 without rewriting author layout")
}

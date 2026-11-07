//go:build windows

package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/kit"
)

// Only the explicit private-desktop entry starts a real MyGo event loop;
// ordinary unit tests retain their existing non-system behavior.
func TestMain(m *testing.M) {
	if os.Getenv("WINONE_NATIVE_BROWSER_CHECK") == "" {
		os.Exit(m.Run())
	}
	directory := os.Getenv("WINONE_NATIVE_UI_REAL_PROFILE")
	mygo.App.SetName("WinOne Native Browser Check")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(directory, "browser-test-profile"))
	mygo.App.OnWindowAllClosed(func() {})
	code := 1
	mygo.App.WhenReady(func() { go func() { code = m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

// The browser, guest script, screenshot, attachment import and draft store are
// all real. DOM text selection drives the same upstream guest's selection path
// without sending physical input to the user's desktop or synthesizing replies.
func TestRealNativeBrowserCommentScreenshotAndHostDraft(t *testing.T) {
	pipe, profile := os.Getenv("WINONE_NATIVE_UI_REAL_PIPE"), os.Getenv("WINONE_NATIVE_UI_REAL_PROFILE")
	if pipe == "" || profile == "" || os.Getenv("WINONE_NATIVE_BROWSER_CHECK") == "" {
		t.Skip("explicit real browser/native matrix entry required")
	}
	client, err := host.NewClient(pipe)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cancel()
	var conversation struct{ ID string }
	if err := client.CallInto("chat.conversation.new", map[string]string{"title": "Browser annotation matrix"}, &conversation); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><title>Native annotation fixture</title></head><body style="padding:80px;font:20px Segoe UI"><h1 id="target">Selected native browser text</h1><p>No external resource or account.</p></body></html>`)
	}))
	defer server.Close()
	var window *mygo.Window
	var pane *NativeBrowserPane
	var imported []model.Attachment
	env := &core.Env{Host: client, Directory: profile, Toast: func(string) {}}
	env.Post = func(fn func()) {
		if window != nil && !window.IsDestroyed() {
			window.Update(fn)
		}
	}
	webViewsCreated := 0
	env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) {
		webViewsCreated++
		return window.NewWebView(options)
	}
	window = mygo.NewWindow(mygo.WindowOptions{Title: "WinOne native browser annotation", Width: 900, Height: 700, Content: ui.View(func(c *ui.Context) {
		c.SetTheme(kit.Theme(c.Preferences(), false))
		if pane != nil {
			pane.View(c)
		}
	})})
	defer window.Destroy()
	mygo.RunOnMain(func() {
		pane = NewNativeBrowserPane(env, NativeBrowserOptions{SessionID: conversation.ID, TabID: "annotation", URL: server.URL, MarkerNumbers: func() []float64 {
			result := []float64{}
			for _, file := range imported {
				var annotation struct{ MarkerNumber float64 }
				_ = json.Unmarshal(file.Annotation, &annotation)
				result = append(result, annotation.MarkerNumber)
			}
			return result
		}, OnComment: func(raw json.RawMessage, done func(error)) {
			var annotation struct{ ScreenshotPath string }
			if err := json.Unmarshal(raw, &annotation); err != nil {
				done(err)
				return
			}
			existing := append([]model.Attachment(nil), imported...)
			core.Go(env, func() ([]model.Attachment, error) {
				var result struct{ Attachments []model.Attachment }
				err := client.CallInto("chat.attachments.add", map[string]any{"id": conversation.ID, "paths": []string{annotation.ScreenshotPath}}, &result)
				if err != nil {
					return nil, err
				}
				for index := range result.Attachments {
					result.Attachments[index].Annotation = raw
				}
				result.Attachments = append(existing, result.Attachments...)
				err = client.CallInto("chat.draft.save", map[string]any{"id": conversation.ID, "draft": map[string]any{"text": "Preserved user draft", "attachments": result.Attachments}}, nil)
				return result.Attachments, err
			}, func(files []model.Attachment, err error) {
				if err == nil {
					imported = files
				}
				done(err)
			})
		}})
	})
	defer mygo.RunOnMain(pane.Close)
	window.Update(func() {})
	await := func(check func() bool, description string) {
		t.Helper()
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			ready := false
			mygo.RunOnMain(func() { ready = check() })
			if ready {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatalf("%s: browser did not settle", description)
	}
	await(func() bool {
		return pane.ready && !pane.state.Loading && pane.state.Title == "Native annotation fixture"
	}, "real browser navigation")
	var view *mygo.WebView
	mygo.RunOnMain(func() {
		for _, tab := range pane.browser.tabs {
			view = tab.view
		}
	})
	if view == nil {
		t.Fatal("real browser view was not materialized")
	}
	// Send messages only to this process's own MyGoSurface child. The positive
	// control proves these reach the real page before the native modal covers
	// it; no SendInput or user's desktop/application is involved.
	type rect struct{ Left, Top, Right, Bottom int32 }
	user32 := windows.NewLazySystemDLL("user32.dll")
	findChild, getRect, sendMessage := user32.NewProc("FindWindowExW"), user32.NewProc("GetClientRect"), user32.NewProc("SendMessageW")
	var surface uintptr
	var bounds rect
	mygo.RunOnMain(func() {
		// MyGo can also create a title/inspector surface. Match the largest
		// owned client surface, not whichever same-class child was created first.
		var child uintptr
		for {
			child, _, _ = findChild.Call(window.NativeHandle(), child, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("MyGoSurface"))), 0)
			if child == 0 {
				break
			}
			var candidate rect
			if ok, _, _ := getRect.Call(child, uintptr(unsafe.Pointer(&candidate))); ok != 0 && (candidate.Right-candidate.Left)*(candidate.Bottom-candidate.Top) > (bounds.Right-bounds.Left)*(bounds.Bottom-bounds.Top) {
				surface, bounds = child, candidate
			}
		}
	})
	if surface == 0 {
		t.Fatal("the owned native browser surface was not found")
	}
	t.Logf("owned native browser surface bounds: %dx%d", bounds.Right-bounds.Left, bounds.Bottom-bounds.Top)
	point := uintptr(uint16((bounds.Right-bounds.Left)/2)) | uintptr(uint16((bounds.Bottom-bounds.Top)/2))<<16
	sendInput := func() {
		// Match MyGo's native input hook: dispatch on the UI thread, avoiding
		// cross-thread SendMessage's synchronous COM/input reentrancy boundary.
		mygo.RunOnMain(func() {
			sendMessage.Call(surface, 0x200, 0, point) // WM_MOUSEMOVE
			sendMessage.Call(surface, 0x201, 1, point) // WM_LBUTTONDOWN
			sendMessage.Call(surface, 0x202, 0, point) // WM_LBUTTONUP
		})
	}
	_, err = view.Page().Eval(`window.__nativePointerCount=0;document.addEventListener('mousedown',()=>window.__nativePointerCount++,true);`)
	if err != nil {
		t.Fatal(err)
	}
	// Navigation completion precedes composition placement. Wait for the
	// actual embedded viewport before sending a native surface click.
	deadline := time.Now().Add(5 * time.Second)
	placed := false
	for time.Now().Before(deadline) {
		value, err := view.Page().Eval("return window.innerWidth >= 700 && window.innerHeight >= 400")
		if err != nil {
			t.Fatal(err)
		}
		if value == true {
			placed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !placed {
		t.Fatal("real browser composition viewport was not placed")
	}
	var positive any
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// WebView2's native input placement can lag navigation/viewport readiness.
		// Wait for actual delivery, not a timer or an assumed ready flag.
		mygo.RunOnMain(func() { user32.NewProc("RedrawWindow").Call(surface, 0, 0, 0x101) })
		sendInput()
		positive, err = view.Page().Eval("return window.__nativePointerCount")
		if err != nil {
			t.Fatal(err)
		}
		if count, ok := positive.(float64); ok && count >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if count, ok := positive.(float64); !ok || count < 1 {
		t.Fatalf("owned surface pointer input did not reach the unoccluded browser: %v", positive)
	}
	// Select actual DOM text before entering annotation mode; the unmodified
	// Cindy guest detects the selection itself and sends its real target facts.
	_, err = view.Page().Eval(`window.__nativePointerCount=0;const range=document.createRange();range.selectNodeContents(document.getElementById('target'));const selection=window.getSelection();selection.removeAllRanges();selection.addRange(range);`)
	if err != nil {
		t.Fatal(err)
	}
	window.Update(pane.toggleComment)
	await(func() bool { return len(pane.target) > 0 || pane.error != "" }, "real guest selection")
	mygo.RunOnMain(func() {
		if pane.error != "" {
			t.Errorf("guest: %s", pane.error)
		}
	})
	image, err := window.CapturePage()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(profile), "browser-comment-editor.png"), image, 0600); err != nil {
		t.Fatal(err)
	}
	visibility, visibilityErr := view.Page().Eval("return document.visibilityState")
	t.Logf("native annotation dialog page visibility: %v error=%v", visibility, visibilityErr)
	sendInput()
	blocked, err := view.Page().Eval("return new Promise(resolve => setTimeout(() => resolve(window.__nativePointerCount), 150))")
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := blocked.(float64); !ok || count != 0 {
		t.Fatalf("native modal leaked pointer input to the actual browser: %v", blocked)
	}
	window.Update(func() { pane.comment = "Actual native annotation"; pane.submit() })
	await(func() bool { return !pane.submitting && (pane.notice != "" || pane.error != "") }, "screenshot/import/commit")
	mygo.RunOnMain(func() {
		if pane.error != "" {
			t.Errorf("annotation: %s", pane.error)
		}
		if len(imported) != 1 || len(pane.target) != 0 || pane.nextMarker() != 2 {
			t.Errorf("annotation was not committed exactly once: files=%d target=%s marker=%v", len(imported), pane.target, pane.nextMarker())
		}
	})
	var draft struct {
		Text        string
		Attachments []model.Attachment
	}
	if err := client.CallInto("chat.draft.get", map[string]string{"id": conversation.ID}, &draft); err != nil {
		t.Fatal(err)
	}
	if draft.Text != "Preserved user draft" || len(draft.Attachments) != 1 || !strings.Contains(string(draft.Attachments[0].Annotation), "Actual native annotation") {
		t.Fatalf("actual Host draft lost its annotation or user text: %+v", draft)
	}
	data, err := os.ReadFile(draft.Attachments[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ui.DecodeBitmap(data); err != nil {
		t.Fatalf("actual imported screenshot is not a PNG: %v", err)
	}
	// Continue on the same live page: actual native element selection drives
	// the guest, then the native design controls preview/reset real DOM styles.
	pagePoint := func(x, y float64) uintptr {
		t.Helper()
		mygo.RunOnMain(func() {
			user32.NewProc("RedrawWindow").Call(surface, 0, 0, 0x101)
			getRect.Call(surface, uintptr(unsafe.Pointer(&bounds)))
		})
		value, err := view.Page().Eval("return [window.innerWidth,window.innerHeight]")
		if err != nil {
			t.Fatal(err)
		}
		viewport := value.([]any)
		scale := float64(bounds.Right-bounds.Left) / viewport[0].(float64)
		top := float64(bounds.Bottom-bounds.Top) - viewport[1].(float64)*scale
		return uintptr(uint16(int(x*scale))) | uintptr(uint16(int(top+y*scale)))<<16
	}
	elementRect, err := view.Page().Eval("window.getSelection().removeAllRanges();const r=document.getElementById('target').getBoundingClientRect();return [r.x+r.width/3,r.y+r.height/2]")
	if err != nil {
		t.Fatal(err)
	}
	pointValues := elementRect.([]any)
	elementPoint := pagePoint(pointValues[0].(float64), pointValues[1].(float64))
	mygo.RunOnMain(func() {
		sendMessage.Call(surface, 0x200, 0, elementPoint)
		sendMessage.Call(surface, 0x201, 1, elementPoint)
		sendMessage.Call(surface, 0x202, 0, elementPoint)
	})
	await(func() bool { return pane.targetData.Kind == "element" && len(pane.target) > 0 }, "actual element annotation target")
	originalFont := ""
	mygo.RunOnMain(func() {
		if pane.targetData.DesignBaseline == nil {
			t.Error("actual element omitted its design baseline")
			return
		}
		originalFont = pane.targetData.DesignBaseline.Styles["font-size"]
		pane.styles["font-size"] = "22px"
		pane.editedText = "Native preview text"
		pane.previewDesign(false)
	})
	awaitDOM := func(expression, description string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			value, err := view.Page().Eval("return " + expression)
			if err != nil {
				t.Fatal(err)
			}
			if value == true {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal(description)
	}
	awaitDOM("document.getElementById('target').textContent==='Native preview text' && getComputedStyle(document.getElementById('target')).fontSize==='22px'", "real design preview was not applied")
	mygo.RunOnMain(func() {
		for name, value := range pane.targetData.DesignBaseline.Styles {
			pane.styles[name] = value
		}
		pane.editedText = *pane.targetData.DesignBaseline.EditableText
		pane.previewDesign(true)
	})
	encodedFont, _ := json.Marshal(originalFont)
	awaitDOM("document.getElementById('target').textContent==='Selected native browser text' && getComputedStyle(document.getElementById('target')).fontSize==="+string(encodedFont), "real design reset was not applied")
	window.Update(func() {
		pane.styles["font-size"] = "22px"
		pane.previewDesign(false)
		pane.comment = "Actual element annotation"
	})
	awaitDOM("getComputedStyle(document.getElementById('target')).fontSize==='22px'", "final real design preview was not applied")
	window.Update(pane.submit)
	await(func() bool { return !pane.submitting && len(imported) == 2 }, "element screenshot/import/commit")
	// Shift is supplied by the owned native mouse messages, not by changing
	// keyboard state or sending input on the user's desktop.
	_, err = view.Page().Eval(`window.__regionEvents=[];for(const type of ['mousedown','mousemove','mouseup'])document.addEventListener(type,event=>window.__regionEvents.push({type,x:event.clientX,y:event.clientY,shift:event.shiftKey,buttons:event.buttons}),true);`)
	if err != nil {
		t.Fatal(err)
	}
	regionStart, regionEnd := pagePoint(180, 230), pagePoint(320, 330)
	// The previous modal keeps its pointer shield through its exit animation.
	// Wait for actual hover delivery before beginning the native Shift drag.
	deadline = time.Now().Add(5 * time.Second)
	pointerReady := false
	for time.Now().Before(deadline) {
		mygo.RunOnMain(func() {
			user32.NewProc("RedrawWindow").Call(surface, 0, 0, 0x101)
			sendMessage.Call(surface, 0x200, 4, regionStart)
		})
		value, err := view.Page().Eval("return window.__regionEvents.some(event=>event.type==='mousemove')")
		if err != nil {
			t.Fatal(err)
		}
		if value == true {
			pointerReady = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !pointerReady {
		t.Fatal("native modal pointer shield did not finish its exit")
	}
	mygo.RunOnMain(func() {
		sendMessage.Call(surface, 0x201, 5, regionStart)
		sendMessage.Call(surface, 0x200, 5, regionEnd)
		sendMessage.Call(surface, 0x202, 4, regionEnd)
	})
	regionEvents, _ := view.Page().Eval("return new Promise(resolve=>setTimeout(()=>resolve(window.__regionEvents),100))")
	t.Logf("actual region native mouse events: %v", regionEvents)
	await(func() bool { return pane.targetData.Kind == "region" && len(pane.target) > 0 }, "actual region annotation target")
	window.Update(func() { pane.comment = "Actual region annotation"; pane.submit() })
	await(func() bool { return !pane.submitting && len(imported) == 3 }, "region screenshot/import/commit")
	if err := client.CallInto("chat.draft.get", map[string]string{"id": conversation.ID}, &draft); err != nil {
		t.Fatal(err)
	}
	if len(draft.Attachments) != 3 || draft.Text != "Preserved user draft" {
		t.Fatal("multiple native annotations replaced earlier attachments or user text")
	}
	kinds := []string{"text", "element", "region"}
	for index, file := range draft.Attachments {
		var annotation struct {
			MarkerNumber int
			Target       struct{ Kind string }
			StyleChanges []struct{ Property, Value string }
		}
		if err := json.Unmarshal(file.Annotation, &annotation); err != nil {
			t.Fatal(err)
		}
		if annotation.MarkerNumber != index+1 || annotation.Target.Kind != kinds[index] {
			t.Fatalf("actual annotation order/identity lost: %s", file.Annotation)
		}
		if index == 1 && (len(annotation.StyleChanges) != 1 || annotation.StyleChanges[0].Property != "font-size" || annotation.StyleChanges[0].Value != "22px") {
			t.Fatal("actual design change was not included in its annotation")
		}
	}
	mygo.RunOnMain(func() {
		if webViewsCreated != 1 || view.IsDestroyed() {
			t.Errorf("annotation recreated or destroyed its actual WebView: creates=%d", webViewsCreated)
		}
		for _, tab := range pane.browser.tabs {
			if tab.view != view {
				t.Error("annotation switched to another WebView")
			}
		}
	})
	t.Log("real browser text/element/region targets + design preview/reset + modal pointer shielding + real screenshot/Host drafts + same-WebView marker commits")
}

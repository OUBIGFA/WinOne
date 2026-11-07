//go:build windows

package mcpapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

// This independent native test continues security/lifecycle verification even
// if an unrelated plugin tool-preparation regression blocks the full workflow.
// It still installs the actual package and runs the official App in WebView2.
func TestNativeMcpAppActualFrameOriginAndDescendantNavigation(t *testing.T) {
	if os.Getenv("WINONE_MCP_APPS_NATIVE") != "1" {
		t.Skip("explicit isolated native frame check")
	}
	var nestedHits, foreignHits atomic.Int32
	const frameOrigin = "http://public.frames-mcp-fixture.localhost"
	const foreignOrigin = "http://foreign.frames-mcp-fixture.localhost"
	// Native MyGo serves these two test-only origins directly. This avoids
	// system DNS/proxy configuration while still exercising genuine Chromium
	// frames, CSP and the standard App's original backend.
	err := mygo.Protocol.HandleFunc("frames-mcp-fixture", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Host == "foreign" {
			foreignHits.Add(1)
			fmt.Fprint(w, "unexpected navigation")
			return
		}
		nestedHits.Add(1)
		if r.URL.Path == "/frame" {
			fmt.Fprintf(w, `<!doctype html><title>Owned declared nested frame</title><iframe src="%s/grandchild"></iframe>`, frameOrigin)
		} else {
			fmt.Fprint(w, `<!doctype html><title>Owned grandchild frame</title><p>Native frame fixture</p>`)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mygo.Protocol.Unhandle("frames-mcp-fixture")
	fixture := startNativeFixture(t, frameOrigin)
	transport := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		var result any
		err := fixture.request("/call", map[string]any{"method": method, "params": json.RawMessage(params)}, &result)
		return result, err
	})
	var page *Page
	window := mygo.NewWindow(mygo.WindowOptions{Title: "WinOne native MCP frame policy", Width: 740, Height: 640, Content: ui.View(func(c *ui.Context) {
		c.SetTheme(kit.Theme(c.Preferences(), false))
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
		page, err = New(transport.Env, fixture.SessionID, fixture.ClientID, "Actual frame policy")
		if err != nil {
			t.Error(err)
		}
		window.Show()
	})
	if page == nil {
		t.Fatal("native viewer was not created")
	}
	defer func() {
		page.Close()
		select {
		case <-page.Done():
		case <-time.After(12 * time.Second):
			t.Error("native policy viewer leaked")
		}
	}()
	awaitNative(t, func() bool { return page.State().Initialized || page.Err() != nil }, "official App initialize for frame checks")
	if page.Err() != nil {
		t.Fatal(page.Err())
	}
	if _, err := evalApp(page, `const child=document.createElement('iframe');child.src=`+strconvQuote(frameOrigin+"/frame")+`;document.body.appendChild(child);`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if t.Failed() {
			frames := []string{}
			mygo.RunOnMain(func() {
				if policy, ok := page.policy.(*nativePolicy); ok {
					for frame := range policy.frames {
						frames = append(frames, fmt.Sprintf("root=%v address=%s closed=%v", frame.root, frame.address, frame.closed))
					}
				}
			})
			browser, browserErr := evalNativeFrame(page, `({url:location.href,title:document.title,ready:document.readyState})`, false, "/grandchild")
			rootBrowser, rootErr := evalApp(page, `({url:location.href,title:document.title,fx:typeof window.fx,ready:window.fx?.ready})`)
			t.Logf("native frame diagnostics: hits=%d foreign=%d domains=%v frames=%v error=%v grandchild=%s %v root=%s %v", nestedHits.Load(), foreignHits.Load(), page.session.Policy.FrameDomains, frames, page.Err(), browser, browserErr, rootBrowser, rootErr)
		}
	}()
	awaitNative(t, func() bool {
		count := 0
		mygo.RunOnMain(func() {
			if policy, ok := page.policy.(*nativePolicy); ok {
				for frame := range policy.frames {
					if !frame.root && !frame.closed {
						count++
					}
				}
			}
		})
		return count >= 2 && nestedHits.Load() >= 2 || page.Err() != nil
	}, "actual Frame7 recursive child creation")
	if page.Err() != nil {
		t.Fatal(page.Err())
	}
	value, err := evalNativeFrame(page, `location.href`, false, "/grandchild")
	if err != nil || !strings.Contains(string(value), "/grandchild") {
		t.Fatalf("declared grandchild did not load: %s %v", value, err)
	}

	observe := func(root bool, suffix string) (<-chan bool, func()) {
		t.Helper()
		notification := make(chan bool, 4)
		var frame *comObject
		var token int64
		mygo.RunOnMain(func() {
			policy := page.policy.(*nativePolicy)
			for item := range policy.frames {
				if item.root == root && !item.closed && (root || strings.HasSuffix(item.address, suffix)) {
					frame = item.frame2
					break
				}
			}
			if frame == nil {
				t.Error("native frame identity disappeared")
				return
			}
			com(frame, 1)
			var err error
			token, err = addCOMEvent(frame, 11, iidFrameNavigate, func(_, args *comObject) {
				var cancelled int32
				com(args, 7, uintptr(unsafe.Pointer(&cancelled)))
				notification <- cancelled != 0
			})
			if err != nil {
				t.Error(err)
			}
		})
		return notification, func() {
			mygo.RunOnMain(func() {
				if frame != nil {
					var destroyed int32
					com(frame, 10, uintptr(unsafe.Pointer(&destroyed)))
					if destroyed == 0 {
						com(frame, 12, uintptr(token))
					}
					releaseCOM(frame)
				}
			})
		}
	}
	childNavigation, offChild := observe(false, "/grandchild")
	defer offChild()
	if _, err := evalNativeFrame(page, `location.href=`+strconvQuote(foreignOrigin+"/denied"), false, "/grandchild"); err != nil {
		t.Fatal(err)
	}
	select {
	case cancelled := <-childNavigation:
		if !cancelled {
			t.Fatal("undeclared grandchild navigation was not cancelled")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("real grandchild NavigationStarting did not arrive")
	}
	value, err = evalNativeFrame(page, `location.href`, false, "/grandchild")
	// Frame Cancel may dispose a descendant or show Chromium's local error
	// document. Neither is a grant: the forbidden origin must never load.
	if foreignHits.Load() != 0 || err == nil && strings.Contains(string(value), "foreign.frames-mcp-fixture.localhost") {
		t.Fatalf("grandchild escaped its declared origin: %s %v hits=%d", value, err, foreignHits.Load())
	}

	if _, err := evalApp(page, `void window.fx.run('private-bridge')`); err != nil {
		t.Fatal(err)
	}
	awaitNative(t, func() bool { return appBool(page, `window.fx.bridgeDenied===true`) }, "App cannot call native-only proxy bridge")
	// Ordinary App navigation is prevented before WebView2 discards the form.
	if _, err := evalApp(page, `location.href=`+strconvQuote(frameOrigin+"/root-escape")); err != nil {
		t.Fatal(err)
	}
	awaitNative(t, func() bool { return appBool(page, `location.pathname==='/resource'&&!!window.fx?.app`) }, "original App document retained")
	// A privileged proxy-side src change bypasses that UX guard. It still
	// reaches the real OS frame gate and revokes the document, with no replay.
	rootNavigation, offRoot := observe(true, "")
	defer offRoot()
	page.mu.Lock()
	view := page.view
	page.mu.Unlock()
	if _, err := view.Page().Eval(`document.querySelector('iframe').src=` + strconvQuote(frameOrigin+"/native-bypass")); err != nil {
		t.Fatal(err)
	}
	select {
	case cancelled := <-rootNavigation:
		if !cancelled {
			t.Fatal("real root frame bypass was allowed")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("real root NavigationStarting did not arrive")
	}
	awaitNative(t, func() bool { return page.Err() != nil }, "native root bypass revocation")
	page.mu.Lock()
	before := page.session
	page.mu.Unlock()
	page.Close()
	<-page.Done()
	request, _ := http.NewRequest("GET", before.Bridge.BaseURL+"/resource", nil)
	request.Header.Set("Authorization", "Bearer "+before.Bridge.Key)
	response, err := nativeHTTP.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("closed native origin retained a document grant")
	}
}

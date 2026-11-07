//go:build windows

package aiservices

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
)

// The fixture is an authored, no-network page loaded by Magpie's actual
// scripts-only sandbox. Its SDK action and allowed read must survive the native
// manager migration, not just the presence of a WebView or a mocked callback.
func TestRealNativeAIThirdPartyPageKeepsSDKActionAndPermissionRead(t *testing.T) {
	if os.Getenv("WINONE_AI_NATIVE_WINDOW") != "1" {
		t.Skip("explicit repository-private desktop")
	}
	p, h, tt, _ := nativeAIWithRuntime(t)
	folder := t.TempDir()
	for file, body := range map[string]string{
		"package.json":    `{"name":"native-page-fixture","version":"1.0.0","magpie":{"page":{"title":"Native SDK fixture","file":"page/index.html"},"permissions":["usage:summary"],"actions":[{"id":"inspect","menu":"provider","title":"Inspect fixture"}]}}`,
		"page/index.html": `<!doctype html><html><body><h2>Native SDK fixture</h2><p>Author content in the original sandbox.</p><script src="_magpie/bridge.js"></script><script src="app.js"></script></body></html>`,
		"page/app.js":     `magpie.onAction(async (action) => { await magpie.ready; const usage = await magpie.read("usage:summary", {period:"today"}); await magpie.options.set({validated:true, action:action.id, provider:action.provider.id, readCalls:usage.calls}); });`,
	} {
		path := filepath.Join(folder, file)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p.switchTab(tabPlugins)
	settle(h, tt)
	p.plugins.act("add", map[string]string{"spec": folder})
	settle(h, tt)
	if p.plugins.api.err != "" || len(p.extensions.state.Pages) != 1 {
		t.Fatalf("real author page not discovered: %s %+v", p.plugins.api.err, p.extensions.state)
	}
	p.plugins.request = &pageRequest{key: p.extensions.state.Pages[0].Key, action: "inspect", menu: "provider", item: &named{ID: "fixture", Name: "Fixture"}, nonce: nonce()}
	var window *mygo.Window
	var guest *mygo.WebView
	mygo.RunOnMain(func() {
		window = mygo.NewWindow(mygo.WindowOptions{Title: "WinOne native author page", Width: 900, Height: 760, Hidden: true, Content: ui.View(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), false))
			ui.Scroll(c).Fill().Padding(20).Children(func() { p.View(c) })
		})})
		h.Env.Post = func(fn func()) {
			mygo.RunOnMain(func() {
				if !window.IsDestroyed() {
					window.Update(fn)
				}
			})
		}
		h.Env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) {
			view, err := window.NewWebView(options)
			guest = view
			return view, err
		}
		window.Show()
	})
	defer mygo.RunOnMain(func() { p.Close(); window.Destroy() })
	var api nativeAPI
	var state managedPlugins
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		ready := false
		mygo.RunOnMain(func() {
			if !api.busy {
				api.request(p, "GET", "/api/plugins", nil, func(raw json.RawMessage) { nativeDecode(&api, raw, &state) })
			}
			ready = len(state.Plugins) == 1 && state.Plugins[0].Options["validated"] == true
		})
		if ready {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	mygo.RunOnMain(func() {
		if len(state.Plugins) != 1 || state.Plugins[0].Options["validated"] != true || state.Plugins[0].Options["action"] != "inspect" || state.Plugins[0].Options["provider"] != "fixture" {
			t.Errorf("real sandbox SDK action/read did not complete: %+v; error=%s", state, api.err)
		}
		api.close()
	})
	if guest != nil {
		data, err := guest.CapturePage()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(os.Getenv("WINONE_AI_NATIVE_PROFILE"), "third-party-sdk-content.png"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	frame, err := window.CapturePage()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("WINONE_AI_NATIVE_PROFILE"), "third-party-sdk-page.png"), frame, 0600); err != nil {
		t.Fatal(err)
	}
}

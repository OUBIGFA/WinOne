//go:build windows

package aiservices

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/web"
)

func TestMain(m *testing.M) {
	if os.Getenv("WINONE_AI_NATIVE_WINDOW") != "1" {
		os.Exit(m.Run())
	}
	profile := os.Getenv("WINONE_AI_NATIVE_PROFILE")
	if !filepath.IsAbs(profile) || !strings.Contains(filepath.ToSlash(profile), "/_temp/") {
		fmt.Fprintln(os.Stderr, "原生 AI 窗口测试需要仓库内的隔离目录")
		os.Exit(1)
	}
	mygo.App.SetName("WinOne AI Native Checks")
	if err := web.Register(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(profile, "native-window"))
	mygo.App.OnWindowAllClosed(func() {})
	code := 1
	mygo.App.WhenReady(func() { go func() { code = m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func TestRealNativeAIWindowsNeedNoWebViewForBuiltInPages(t *testing.T) {
	if os.Getenv("WINONE_AI_NATIVE_WINDOW") != "1" {
		t.Skip("explicit repository-private desktop")
	}
	p, h, _, _ := nativeAIWithRuntime(t)
	var window *mygo.Window
	dark := false
	webviews, frames := 0, 0
	var section func(*ui.Context)
	mygo.RunOnMain(func() {
		window = mygo.NewWindow(mygo.WindowOptions{Title: "WinOne AI native verification", Width: 1000, Height: 900, Hidden: true, Content: ui.View(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), dark))
			ui.Scroll(c).Fill().Padding(20).Children(func() {
				if section != nil {
					section(c)
				} else {
					p.View(c)
				}
			})
			frames++
		})})
		h.Env.Post = func(fn func()) {
			mygo.RunOnMain(func() {
				if !window.IsDestroyed() {
					window.Update(fn)
				}
			})
		}
		h.Env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) {
			webviews++
			return window.NewWebView(options)
		}
		window.Show()
	})
	defer mygo.RunOnMain(func() { p.Close(); window.Destroy() })
	await := func(check func() bool) {
		t.Helper()
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			ready := false
			mygo.RunOnMain(func() { ready = check() })
			if ready {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("原生 AI 页面未在限定时间内完成绘制")
	}
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, night := range []bool{false, true} {
			for _, width := range []int{1000, 560} {
				// Use actual Windows rendering for every rebuilt section, not
				// only each page's default tab. Policy APIs keep the private runtime.
				for scene := 0; scene < 24; scene++ {
					before := 0
					mygo.RunOnMain(func() {
						i18n.SetLocale(locale)
						dark = night
						window.SetContentSize(width, 900)
						section = nil
						if scene < 3 {
							p.switchTab([]int{tabRouting, tabPlugins, tabAdvanced}[scene])
							p.routing.discard()
							p.routing.tab = 0
						}
						if scene == 2 {
							p.advancedTab = advancedPrivacy
						}
						if scene >= 3 && scene < 7 {
							pr := &p.draft.Value.Providers[0].Value
							ed := newEditor(p, pr, pr.ID)
							ed.preferenceTab = scene - 3
							section = func(c *ui.Context) { ed.preferences(c, pr, false) }
						}
						if scene >= 7 && scene < 11 {
							p.advanced.privacyTab = scene - 7
							section = func(c *ui.Context) { p.advanced.privacy(c, &magpieCatalog{}, false) }
						}
						if scene >= 11 && scene < 16 {
							p.routing.begin(core.Preserved[routeGroup]{Value: routeGroup{ID: "window-check", Name: "Daily assistant", Members: []string{"fixture/model"}, Rules: []core.Preserved[routeRule]{{Value: routeRule{Use: "fixture/model", Intent: "coding"}}}}})
							p.routing.editorTab = scene - 11
							section = p.routing.editor
						}
						if scene == 16 {
							p.routing.discard()
							p.switchTab(tabRouting)
							p.routing.tab = 1
						}
						if scene == 17 {
							p.switchTab(tabHome)
						}
						if scene == 18 {
							p.switchTab(tabMore)
						}
						if scene >= 19 && scene < 23 {
							p.switchTab(tabGateway)
							p.gateway.tab = scene - 19
						}
						if scene == 23 {
							p.routing.discard()
							p.switchTab(tabRouting)
							p.routing.tab = 2
						}
						before = frames
						window.Invalidate()
					})
					await(func() bool {
						return frames > before && !p.routing.api.busy && !p.plugins.api.busy && !p.gateway.loading && !p.gateway.access.api.busy && !p.gateway.history.api.busy && (p.advanced == nil || p.advanced.records == nil || !p.advanced.records.api.busy)
					})
					frame, err := window.CapturePage()
					if err != nil {
						t.Fatal(err)
					}
					if len(frame) < 10000 {
						t.Fatal("原生窗口截图为空")
					}
					path := filepath.Join(os.Getenv("WINONE_AI_NATIVE_PROFILE"), fmt.Sprintf("%s-%t-%d-%d.png", locale, night, width, scene))
					if err := os.WriteFile(path, frame, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	mygo.RunOnMain(func() {
		if webviews != 0 {
			t.Errorf("内置页面创建了 %d 个 WebView", webviews)
		}
	})
}

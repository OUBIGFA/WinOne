//go:build webui

// Command webuiprobe checks Chat's React tools in a real MyGo window: it
// serves a fake Host on a private pipe, shows the side panel (package
// webui) at the file library of a conversation, and writes what the page
// rendered, the Host requests it made and a screenshot to the directory
// it is given. tests/desktop-mygo-webui/run-private.ps1 runs it on a
// private desktop, out of the user's sight.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/webui"

	internalhost "winone/internal/host"
	"winone/internal/transport"
)

// report is what the probe writes as probe.json.
type report struct {
	Rendered bool     `json:"rendered"`
	Text     string   `json:"text"`
	Calls    []string `json:"calls"`
	Embeds   []string `json:"embeds"`
	Browser  string   `json:"browser"`
	Views    []string `json:"views"`
	HTML     []string `json:"html"`
	Error    string   `json:"error,omitempty"`
}

type fakeHost struct {
	mu    sync.Mutex
	calls []string
	dir   string
	// files is the loopback address that serves the listing, as the
	// Host's preview server does: the page fetches it from its origin.
	files string
}

// Handle answers what the panel's file library and resources read.
func (h *fakeHost) Handle(r internalhost.Request) internalhost.Response {
	h.mu.Lock()
	h.calls = append(h.calls, r.Method)
	h.mu.Unlock()
	var result any = map[string]any{}
	switch r.Method {
	case "chat.files.list":
		result = map[string]any{"url": h.files}
	case "chat.settings.get":
		result = map[string]any{"workspaceDirectory": h.dir, "skillDirectories": []string{}, "pluginDirectories": []string{},
			"mcpServers": []any{}, "automations": []any{}}
	// The answers of the React tests' fake Host (cindy/host/SchedulerHost.test.tsx).
	case "chat.automations.list", "chat.automations.runs":
		result = []any{}
	case "chat.automations.templates":
		result = map[string]any{"templates": []any{}, "categories": []any{}}
	case "chat.automations.sidebarIndexRuns":
		result = map[string]any{"runs": []any{}, "inflightRunIds": []any{}, "inflightPolicies": []any{}}
	case "chat.automations.events":
		// A long poll: nothing happens, after a while.
		time.Sleep(time.Second)
		result = map[string]any{"cursor": 7, "events": []any{}}
	case "chat.automations.channels":
		result = map[string]any{"feishu": map[string]any{"status": "disconnected"},
			"wecomGroup": map[string]any{"configured": false, "enabled": false}}
	case "chat.automations.runtime":
		result = map[string]any{"schedulerInstanceId": "probe", "inFlight": 0, "slotsInUse": 0,
			"maxConcurrentRuns": 2, "inFlightRuns": []any{}, "waitingSchedules": []any{}}
	case "chat.resources":
		result = map[string]any{"skills": []any{}, "prompts": []any{}, "extensions": []any{}}
	}
	data, _ := json.Marshal(result)
	return internalhost.Response{ID: r.ID, Result: data}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: webuiprobe <output directory>")
		os.Exit(2)
	}
	out := os.Args[1]
	if err := os.MkdirAll(out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fake := &fakeHost{dir: out}
	loopback, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fake.files = "http://" + loopback.Addr().String() + "/files"
	go http.Serve(loopback, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			// The web page a browser tab of the panel shows.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<!doctype html><title>Probe page</title><h1>Probe page</h1>"))
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"directory": out, "entries": []map[string]any{
			{"name": "README.md", "path": filepath.Join(out, "README.md"), "directory": false, "size": 42},
			{"name": "src", "path": filepath.Join(out, "src"), "directory": true, "size": 0},
		}})
	}))
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-webuiprobe-%d`, os.Getpid())
	listener, err := transport.Listen(pipe)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	go transport.Serve(context.Background(), listener, fake)
	client, err := host.NewClient(pipe)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	webui.Register()

	var win *mygo.Window
	env := &core.Env{Host: client, Directory: out, Activity: &core.Activity{}}
	env.Post = func(fn func()) {
		if win != nil {
			win.Update(fn)
		}
	}
	env.WebViews = func(o mygo.WebViewOptions) (*mygo.WebView, error) { return win.NewWebView(o) }
	var embeds []string
	var mu sync.Mutex
	browser := webui.NewBrowser(env)
	panel := webui.New(env, webui.Options{View: "panel", Label: "panel", Browser: browser,
		Receive: func(kind string, payload json.RawMessage, reply func(any, error)) {
			mu.Lock()
			embeds = append(embeds, kind+" "+string(payload))
			mu.Unlock()
			reply(nil, nil)
		}})
	state := map[string]any{
		"language": "zh-CN", "theme": "light",
		"host": map[string]any{"catalog": map[string]any{"projects": []any{}, "conversations": []any{}}, "providers": []any{}},
		"panel": map[string]any{
			"key": "probe:" + out, "id": "probe", "cwd": out, "initialTab": "files", "initialTabRequest": 1,
			"dock": "right", "visible": true, "running": false, "lastTurnPaths": []string{}, "commentMarkerNumbers": []float64{},
			"composerLocked": false,
		},
	}
	// show is the view the window shows: the panel, then the automations
	// page, then Chat settings.
	show := "panel"
	automations := webui.New(env, webui.Options{View: "automations", Label: "automations",
		Receive: func(kind string, _ json.RawMessage, reply func(any, error)) { reply(nil, nil) }})
	settings := webui.New(env, webui.Options{View: "settings", Label: "settings",
		Receive: func(kind string, _ json.RawMessage, reply func(any, error)) { reply(nil, nil) }})
	view := func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			switch show {
			case "automations":
				automations.View(c, state).Fill()
			case "settings":
				settings.View(c, state).Fill()
			default:
				panel.View(c, state).Fill().Children(func() { browser.Overlay(c) })
			}
		})
	}
	state["settings"] = map[string]any{"providers": []any{}, "defaultModel": map[string]any{"provider": "", "model": ""},
		"modelsLoading": false, "modelsError": ""}
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{Title: "WinOne webui probe", Width: 520, Height: 720, Content: ui.View(view)})
		go func() {
			r := run(panel, out)
			if r.Rendered {
				// Open a web page in the panel, as a link of a reply asks.
				page := "http://" + loopback.Addr().String() + "/page"
				win.Update(func() {
					props := state["panel"].(map[string]any)
					props["initialTab"], props["initialTabRequest"], props["initialURL"] = "home", 2, page
				})
				r.Browser = browse(panel)
				time.Sleep(time.Second)
				if err := printWindow(uintptr(win.NativeHandle()), filepath.Join(out, "window.png")); err != nil {
					r.Error = "window capture: " + err.Error()
				}
				for _, name := range []string{"automations", "settings"} {
					surface := automations
					if name == "settings" {
						surface = settings
					}
					win.Update(func() { show = name })
					// Chat settings is a dialog, rendered in a portal of the body.
					selector := ".chat-embed-root"
					if name == "settings" {
						selector = "[role=dialog]"
					}
					text := shown(surface, selector)
					if html, err := mygo.EvalAs[string](surface.Page(), `return document.body.innerHTML.slice(0, 3000)`); err == nil {
						r.HTML = append(r.HTML, name+": "+html)
					}
					r.Views = append(r.Views, name+": "+text)
					if text == "" {
						r.Rendered, r.Error = false, "the "+name+" view did not render"
					}
					if err := printWindow(uintptr(win.NativeHandle()), filepath.Join(out, name+".png")); err != nil {
						r.Error = "window capture: " + err.Error()
					}
				}
				if !strings.Contains(r.Browser, "Probe page") {
					r.Rendered, r.Error = false, "the browser tab did not show the page's title"
				}
			}
			fake.mu.Lock()
			r.Calls = append(r.Calls, fake.calls...)
			fake.mu.Unlock()
			mu.Lock()
			r.Embeds = append(r.Embeds, embeds...)
			mu.Unlock()
			data, _ := json.MarshalIndent(r, "", " ")
			_ = os.WriteFile(filepath.Join(out, "probe.json"), data, 0o644)
			code := 0
			if !r.Rendered {
				code = 1
			}
			os.Exit(code)
		}()
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run waits for the panel's file library to list the fake folder, then
// takes its text and a screenshot.
func run(panel *webui.Surface, out string) report {
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		page := panel.Page()
		if page == nil {
			continue
		}
		text, err := mygo.EvalAs[string](page, `const panel = document.querySelector(".chat-side-panel"); return panel ? panel.innerText : ""`)
		if err != nil || text == "" {
			continue
		}
		var r report
		r.Text = text
		r.Rendered = true
		if view := page.WebView(); view != nil {
			if png, err := view.CapturePage(); err == nil {
				_ = os.WriteFile(filepath.Join(out, "panel.png"), png, 0o644)
			} else {
				r.Error = err.Error()
			}
		}
		// Give the library a moment to list the folder before reporting.
		time.Sleep(1500 * time.Millisecond)
		if more, err := mygo.EvalAs[string](page, `return document.querySelector(".chat-side-panel")?.innerText ?? ""`); err == nil {
			r.Text = more
		}
		// The library lists the folder only if the page reached the Host
		// through App and fetched the listing from its loopback server.
		r.Rendered = strings.Contains(r.Text, "README.md")
		if !r.Rendered {
			r.Error = "the file library did not list the folder"
		}
		return r
	}
	return report{Error: "the panel did not render in 40 seconds"}
}

// browse waits for the panel's browser tab to name the page it shows: the
// page loaded in Go's web view, and its title came back to the panel.
func browse(panel *webui.Surface) string {
	deadline := time.Now().Add(30 * time.Second)
	text := ""
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		value, err := mygo.EvalAs[string](panel.Page(), `return document.querySelector(".chat-side-panel")?.innerText ?? ""`)
		if err == nil {
			text = value
			if strings.Contains(text, "Probe page") {
				break
			}
		}
	}
	return text
}

// shown waits for a view's page to render text under selector, and
// returns it, or "" after 30 seconds.
func shown(surface *webui.Surface, selector string) string {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		page := surface.Page()
		if page == nil {
			continue
		}
		text, err := mygo.EvalAs[string](page, `return document.querySelector(`+strconvQuote(selector)+`)?.innerText ?? ""`)
		if err == nil && strings.TrimSpace(text) != "" {
			time.Sleep(time.Second)
			if again, err := mygo.EvalAs[string](page, `return document.querySelector(`+strconvQuote(selector)+`)?.innerText ?? ""`); err == nil {
				text = again
			}
			return text
		}
	}
	return ""
}

func strconvQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// Command ghostworker runs only a Host-owned Cindy plugin logic page. It never
// opens WinOne settings and exposes no Go bindings to the untrusted page.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/egoist/mygo"
)

type initialization struct {
	URL       string `json:"url"`
	Directory string `json:"directory"`
	SDK       string `json:"sdk"`
}

type incoming struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

var bootPath = regexp.MustCompile(`^/[a-f0-9]{64}/__boot__$`)

// The Host supplies one task-private, unguessable loopback document. A helper
// invoked without a complete initialization cannot fall back to a real profile.
func validateInit(v initialization) error {
	u, err := url.Parse(v.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !bootPath.MatchString(u.Path) {
		return errors.New("invalid plugin sandbox URL")
	}
	if !filepath.IsAbs(v.Directory) || filepath.Clean(v.Directory) != v.Directory || v.SDK == "" || len(v.SDK) > 128<<10 {
		return errors.New("invalid plugin sandbox initialization")
	}
	return nil
}

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--stdio" {
		fmt.Fprintln(os.Stderr, "插件沙箱须由 WinOne Host 启动")
		os.Exit(2)
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 64<<10), 128<<20) // Cindy network text responses can be large.
	if !reader.Scan() {
		return errors.New("plugin sandbox initialization is missing")
	}
	var init initialization
	if err := json.Unmarshal(reader.Bytes(), &init); err != nil {
		return err
	}
	if err := validateInit(init); err != nil {
		return err
	}
	if err := os.MkdirAll(init.Directory, 0700); err != nil {
		return err
	}
	mygo.App.SetName("WinOne Plugin Sandbox")
	mygo.App.SetPath(mygo.PathUserData, init.Directory)
	var writes sync.Mutex
	encoder := json.NewEncoder(os.Stdout)
	emit := func(value any) {
		writes.Lock()
		defer writes.Unlock()
		if err := encoder.Encode(value); err != nil {
			mygo.App.Quit()
		}
	}
	var page *mygo.Page
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "WinOne Plugin Sandbox", URL: init.URL, Width: 80, Height: 80,
			Hidden: true, SkipTaskbar: true,
			Page: mygo.PageOptions{Untrusted: true, PreloadScript: init.SDK, Session: mygo.NewSession("winone-plugin-sandbox")},
		})
		page = win.Page()
		page.OnDidFinishLoad(func() { emit(map[string]any{"type": "loaded"}) })
		page.OnDidFailLoad(func(event *mygo.LoadError) {
			emit(map[string]any{"type": "failed", "code": event.Code})
			mygo.App.Quit()
		})
		page.OnWillNavigate(func(event *mygo.NavigateEvent) {
			if event.URL != init.URL {
				event.PreventDefault()
			}
		})
		page.SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions { return nil })
		// Browser primitives must not bypass the approved plugin slots or write
		// to the user's Downloads directory, even in an invisible logic page.
		page.OnWillDownload(func(event *mygo.DownloadEvent) { event.PreventDefault() })
		page.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
		page.SetAuthHandler(func(mygo.AuthRequest) mygo.AuthResponse { return mygo.AuthResponse{} })
		go func() {
			for reader.Scan() {
				var request incoming
				if err := json.Unmarshal(reader.Bytes(), &request); err != nil {
					mygo.App.Quit()
					return
				}
				name := ""
				switch request.Type {
				case "deliver":
					name = "__winoneGhostDeliver"
				case "reply":
					name = "__winoneGhostReply"
				default:
					mygo.App.Quit()
					return
				}
				if _, err := page.Eval("window." + name + "(" + string(request.Payload) + ")"); err != nil {
					emit(map[string]any{"type": "failed"})
					mygo.App.Quit()
					return
				}
			}
			mygo.App.Quit() // EOF also closes the Chromium descendants.
		}()
		// Plugin requests go directly to their scoped same-origin Host endpoint.
		// No 50 ms Eval polling is needed while the hidden page is idle.
	})
	return mygo.App.Run()
}

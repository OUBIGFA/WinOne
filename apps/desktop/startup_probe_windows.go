//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsOptions "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"winone/internal/catalog"
	"winone/internal/settings"
	"winone/internal/startupprobe"
	"winone/internal/transport"
)

type desktopProbeState struct {
	mu                                                         sync.Mutex
	nonce                                                      string
	snapshot, modulesRead, frameReady, surfaceReady, completed bool
}

func (s *desktopProbeState) complete(nonce string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if nonce == "" || nonce != s.nonce || !s.snapshot || !s.modulesRead || !s.frameReady || !s.surfaceReady {
		return errors.New("desktop startup probe is not ready")
	}
	s.completed = true
	return nil
}

// These endpoints remain inert in normal sessions. Only the dedicated probe
// entry can issue a nonce and exit after a real rendered-page acknowledgement.
func (a *App) StartupProbeNonce() string {
	if a.startupProbe == nil {
		return ""
	}
	return a.startupProbe.nonce
}
func (a *App) CompleteStartupProbe(nonce string) error {
	if a.startupProbe == nil {
		return errors.New("no desktop startup probe")
	}
	if err := a.startupProbe.complete(nonce); err != nil {
		return err
	}
	go runtime.Quit(a.windowContext())
	return nil
}

// The production assets and App bindings run against an isolated snapshot
// service. No user Host, worker, profile, task or shortcut is activated.
func runDesktopProbe(nonce, scratch string) error {
	if nonce == "" || len(nonce) > 128 || !filepath.IsAbs(scratch) {
		return errors.New("invalid desktop probe arguments")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	installation, err := filepath.EvalSymlinks(filepath.Dir(executable))
	if err != nil {
		return err
	}
	scratch, err = filepath.EvalSymlinks(scratch)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(installation, scratch)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("desktop probe data must be outside installation")
	}
	directory, err := os.MkdirTemp(scratch, "desktop-probe-")
	if err != nil {
		return err
	}
	store, err := settings.Open(filepath.Join(directory, "settings.json"))
	if err != nil {
		return err
	}
	service, err := desktopProbeService(directory, store)
	if err != nil {
		return err
	}
	pipe, err := transport.PipeName()
	if err != nil {
		return err
	}
	pipe += "-desktop-probe-" + rand.Text()
	listener, err := transport.Listen(pipe)
	if err != nil {
		return err
	}
	defer listener.Close()
	serviceContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	serviceDone := make(chan error, 1)
	go func() { serviceDone <- transport.Serve(serviceContext, listener, service) }()
	defer func() { cancel(); <-serviceDone }()
	state := &desktopProbeState{nonce: nonce}
	app := &App{pipe: pipe, dataDirectory: directory, startupProbe: state}
	finished := make(chan struct{})
	defer close(finished)
	group, err := ownProbeProcessGroup()
	if err != nil {
		return err
	}
	defer group.abort()
	err = wails.Run(&options.App{
		Title: "WinOne startup check", Width: 1280, Height: 840, StartHidden: true, Frameless: true,
		AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{app},
		OnStartup: func(ctx context.Context) {
			app.ctxMu.Lock()
			app.ctx = ctx
			app.ctxMu.Unlock()
			go func() {
				select {
				case <-finished:
				case <-time.After(25 * time.Second):
					runtime.Quit(ctx)
				}
			}()
		},
		OnDomReady: func(context.Context) { app.InitializeWindow() },
		Windows:    &windowsOptions.Options{WebviewUserDataPath: filepath.Join(directory, "webview"), Theme: windowsOptions.SystemDefault, WindowClassName: desktopWindowClass},
	})
	if err != nil {
		return err
	}
	state.mu.Lock()
	completed := state.completed
	state.mu.Unlock()
	if !completed {
		return errors.New("desktop startup probe did not complete")
	}
	if err = group.complete(); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(startupprobe.Report{Protocol: 1, Nonce: nonce, PID: os.Getpid(), Executable: executable, Modules: catalog.Builtins(), UIReady: true})
}

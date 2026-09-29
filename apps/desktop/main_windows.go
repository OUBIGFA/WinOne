//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsOptions "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"winone/internal/host"
	"winone/internal/settings"
	"winone/internal/transport"
)

//go:embed all:frontend/dist
var assets embed.FS

// App exposes the small settings API, never the Host's process or filesystem access.
type App struct {
	pipe         string
	sequence     atomic.Uint64
	ctxMu        sync.RWMutex
	ctx          context.Context
	startupError error
}

func (a *App) windowContext() context.Context { a.ctxMu.RLock(); defer a.ctxMu.RUnlock(); return a.ctx }

func (a *App) applyTheme(theme string) {
	ctx := a.windowContext()
	if ctx == nil {
		return
	}
	switch theme {
	case "dark":
		runtime.WindowSetDarkTheme(ctx)
	case "light":
		runtime.WindowSetLightTheme(ctx)
	default:
		runtime.WindowSetSystemDefaultTheme(ctx)
	}
}

func (a *App) request(method string, params any, result any) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	r, err := transport.Call(context.Background(), a.pipe, host.Request{Version: 1, ID: fmt.Sprint(a.sequence.Add(1)), Method: method, Params: data})
	if err != nil {
		return fmt.Errorf("无法连接后台：%w", err)
	}
	if r.Error != nil {
		return errors.New(r.Error.Message)
	}
	return json.Unmarshal(r.Result, result)
}

// Snapshot is the read-only page bootstrap API exposed through Wails.
func (a *App) Snapshot() (host.Snapshot, error) {
	var result host.Snapshot
	err := a.request("snapshot", nil, &result)
	if err != nil {
		return result, errors.Join(a.startupError, err)
	}
	a.applyTheme(result.Settings.Theme)
	return result, err
}

// SaveSettings preserves the Host's revision checks before changing the window theme.
func (a *App) SaveSettings(next settings.Settings) (settings.Settings, error) {
	var result settings.Settings
	err := a.request("settings.save", next, &result)
	if err == nil {
		a.applyTheme(result.Theme)
	}
	return result, err
}

// WinTray limits page access to the module's documented command set.
func (a *App) WinTray(method, payload string) (string, error) {
	switch method {
	case "snapshot", "settings.save", "launch", "jobs.get", "icons", "icon.activate":
	default:
		return "", errors.New("不支持的程序管理操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("wintray."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// PickProgram uses the native file picker; the selected file is not executed here.
func (a *App) PickProgram() (string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New("设置窗口未就绪")
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: "选择程序或脚本", Filters: []runtime.FileFilter{{DisplayName: "程序与脚本", Pattern: "*.exe;*.bat;*.cmd;*.ps1;*.py;*.pyw"}}})
}

// ensureHost starts only the sibling binary and waits for its actual pipe readiness.
func ensureHost(pipe, dataDir string) error {
	probe := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		response, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "ready", Method: "snapshot"})
		if err != nil {
			return err
		}
		if response.Error != nil {
			return errors.New(response.Error.Message)
		}
		return nil
	}
	if probe() == nil {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(exe), "WinOne.Host.exe"), "--pipe", pipe, "--data-dir", dataDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err = cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	for range 40 {
		if probe() == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("后台启动失败，请检查配置目录与日志")
}

func main() {
	pipeDefault, err := transport.PipeName()
	if err != nil {
		panic(err)
	}
	pipe := flag.String("pipe", pipeDefault, "local pipe name")
	data := flag.String("data-dir", filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne"), "settings directory")
	flag.Parse()
	app := &App{pipe: *pipe}
	// A failed Host is shown in the page with retry, not replaced by fabricated settings.
	app.startupError = ensureHost(*pipe, *data)
	err = wails.Run(&options.App{
		Title: "WinOne", Width: 1040, Height: 720, MinWidth: 800, MinHeight: 560,
		BackgroundColour: options.NewRGB(248, 248, 246), HideWindowOnClose: false,
		AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{app},
		OnStartup: func(ctx context.Context) { app.ctxMu.Lock(); app.ctx = ctx; app.ctxMu.Unlock() },
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: fmt.Sprintf("winone-desktop-%x", sha256.Sum256([]byte(*pipe))), OnSecondInstanceLaunch: func(options.SecondInstanceData) {
			if ctx := app.windowContext(); ctx != nil {
				runtime.WindowUnminimise(ctx)
				runtime.WindowShow(ctx)
			}
		}},
		DragAndDrop: &options.DragAndDrop{DisableWebViewDrop: true},
		Windows:     &windowsOptions.Options{WebviewUserDataPath: filepath.Join(*data, "webview"), Theme: windowsOptions.SystemDefault},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

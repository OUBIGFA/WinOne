//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"fyne.io/systray"
	"golang.org/x/sys/windows"
	"os"
	"os/signal"
	"path/filepath"
	"time"
	"winone/internal/adminstartup"
	"winone/internal/branding"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/hotkeys"
	"winone/internal/logintask"
	"winone/internal/maintenance"
	"winone/internal/processidentity"
	"winone/internal/residency"
	"winone/internal/settings"

	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/transport"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	pipeDefault, err := transport.PipeName()
	if err != nil {
		return err
	}
	pipe := flag.String("pipe", pipeDefault, "local pipe name")
	data := flag.String("data-dir", filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne"), "settings directory")
	headless := flag.Bool("headless", false, "run without tray for integration tests")
	autorun := flag.Bool("autorun", false, "run configured login tasks")
	standardSession := flag.Bool("standard-session", false, "skip remembered administrator startup for recovery")
	uninstall := flag.Bool("prepare-uninstall", false, "restore resources before uninstall; requires WinOne to be closed")
	guardDirectory := flag.String("guard-installation", "", "installation guarded for the installer lifetime")
	installerPID := flag.Uint("installer-pid", 0, "installer process owning the maintenance session")
	installerReady := flag.String("installer-ready", "", "installer-created readiness event")
	startupProbe := flag.String("check-startup", "", "read-only startup probe nonce")
	configurationProbe := flag.String("check-configuration", "", "read-only configuration probe nonce; reads stdin")
	flag.Parse()
	probeRequested := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "check-startup" || f.Name == "check-configuration" {
			probeRequested = true
		}
	})
	if probeRequested {
		// No runtime options are accepted on this maintenance-only entry.
		valid := flag.NArg() == 0
		count := 0
		flag.Visit(func(f *flag.Flag) {
			count++
			if f.Name != "check-startup" && f.Name != "check-configuration" {
				valid = false
			}
		})
		if !valid || count != 1 {
			return fmt.Errorf("启动检查参数无效")
		}
		if *configurationProbe != "" {
			return writeConfigurationProbe(os.Stdout, os.Stdin, *configurationProbe)
		}
		return writeStartupProbe(os.Stdout, *startupProbe)
	}
	if *guardDirectory != "" || *installerPID != 0 || *installerReady != "" {
		if !filepath.IsAbs(*guardDirectory) || *installerPID == 0 || uint64(*installerPID) > 0xffffffff || *installerReady == "" || *autorun || *headless {
			return fmt.Errorf("安装维护参数无效")
		}
		return guardInstallation(*guardDirectory, *data, *pipe, uint32(*installerPID), *installerReady, *uninstall)
	}
	if *uninstall && (*autorun || *headless) {
		return fmt.Errorf("卸载准备不能与登录启动或测试服务同时运行")
	}
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}
	acquire := maintenance.AcquireRuntimeForInstallation
	if *uninstall {
		acquire = maintenance.AcquireMaintenanceForInstallation
	}
	lease, err := acquire(*data, filepath.Dir(executablePath))
	if err != nil {
		return err
	}
	defer lease.Close()
	startupWarning := ""
	if !*headless && !*uninstall {
		mode := logintask.InteractiveLaunch
		if *autorun {
			mode = logintask.LoginLaunch
		}
		redirected, redirectErr := adminstartup.Try(context.Background(), *data, *pipe, filepath.Join(filepath.Dir(executablePath), "WinOne.exe"), mode, *standardSession)
		if redirected {
			return nil
		}
		if redirectErr != nil {
			startupWarning = "本次未以管理员身份启动：" + redirectErr.Error()
		}
	}
	listener, err := transport.Listen(*pipe)
	if err != nil {
		return fmt.Errorf("start Host: %w", err)
	}
	defer listener.Close()
	if *uninstall {
		return prepareUninstall(*data, *pipe)
	}
	var startupLogError error
	if !*headless {
		startupLogError = recordHostEvent(*data, "started", nil)
		if startupLogError != nil {
			fmt.Fprintln(os.Stderr, "diagnostic startup log:", startupLogError)
		}
		// Diagnostic I/O must not turn a normal user-requested exit into a crash.
		defer func() {
			if logErr := recordHostEvent(*data, "stopped", runErr); logErr != nil {
				fmt.Fprintln(os.Stderr, "diagnostic shutdown log:", logErr)
			}
		}()
	}
	var alwaysOnTop *modules.Lazy
	var capture *modules.Lazy
	var wintray *modules.Lazy
	var wintrayGate *modules.Gate
	configurationReady := false
	var store *configuration.HostStore
	adapters := []configuration.ModuleAdapter{aotConfiguration(*data, func() *modules.Lazy { return alwaysOnTop }), captureConfiguration(*data, func() *modules.Lazy { return capture }), wintrayConfiguration(*data, func() *modules.Lazy { return wintray }, func() bool { return configurationReady && wintrayGate != nil && wintrayGate.Enabled() })}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if *headless {
		store, err = configuration.OpenHost(*data, adapters...)
		if err != nil {
			return err
		}
		service := host.New(store)
		service.SetAdministrator(windows.GetCurrentProcessToken().IsElevated())
		return transport.Serve(ctx, listener, host.NewConfigurationQueue(service))
	}
	moduleState, _, err := openModuleState(*data)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	trayManifest, ok := catalog.Builtin("wintray")
	if !ok {
		return fmt.Errorf("missing WinTray manifest")
	}
	captureManifest, ok := catalog.Builtin("capture")
	if !ok {
		return fmt.Errorf("missing capture manifest")
	}
	pinManifest, ok := catalog.Builtin("always-on-top")
	if !ok {
		return fmt.Errorf("missing pin manifest")
	}
	trayRecord := moduleState.Snapshot().Modules["wintray"]
	wintray = modules.NewLogonLazy(ctx, trayManifest.ID, trayManifest.Name.Chinese, filepath.Join(filepath.Dir(executable), trayManifest.Worker), *pipe+"-wintray", filepath.Join(*data, "modules", "wintray"), *autorun && trayRecord.Enabled && !trayRecord.PendingStop)
	capture = modules.NewLazy(ctx, captureManifest.ID, captureManifest.Name.Chinese, filepath.Join(filepath.Dir(executable), captureManifest.Worker), *pipe+"-capture", filepath.Join(*data, "modules", "capture"), "--host-pipe", *pipe)
	alwaysOnTop = modules.NewLazy(ctx, pinManifest.ID, pinManifest.Name.Chinese, filepath.Join(filepath.Dir(executable), pinManifest.Worker), *pipe+"-always-on-top", filepath.Join(*data, "modules", "always-on-top"))
	captureWindows := newCaptureWindows(func(id string) (<-chan error, error) {
		return startCaptureWindow(filepath.Join(filepath.Dir(executable), "WinOne.exe"), *pipe, *data, id)
	})
	shortcuts := &shortcutRuntime{registrar: hotkeys.New(), moduleState: moduleState, openDraft: func(id string) error {
		_, err := captureWindows.Open(id)
		return err
	}, language: func() string { return applicationLanguage(store.Snapshot().Language) }}
	defer shortcuts.registrar.Close()
	gates := map[string]*modules.Gate{}
	for _, entry := range []struct {
		manifest catalog.Manifest
		runtime  *modules.Lazy
	}{{trayManifest, wintray}, {captureManifest, capture}, {pinManifest, alwaysOnTop}} {
		runtime := entry.runtime
		manifest := entry.manifest
		start := func() error { runtime.Resume(); return nil }
		stop := runtime.Close
		if manifest.ID != "wintray" {
			stop = func() error {
				runtime.Resume()
				response := runtime.Handle(host.Request{Version: 1, ID: "module-stop-probe", Method: "snapshot"})
				if response.Error != nil {
					return fmt.Errorf("%s", response.Error.Message)
				}
				return runtime.Close()
			}
		}
		if manifest.ID == "wintray" {
			start = func() error { return startWinTray(ctx, runtime) }
			stop = func() error { return stopWinTray(ctx, runtime) }
		}
		gate, gateErr := modules.NewGate(manifest.ID, manifest.Name.Chinese, moduleState, modules.LifecycleHooks{Start: start, Stop: stop, Status: runtime.Status, Handle: runtime.Handle, IntentChanged: func() error {
			if store != nil {
				shortcuts.apply(store.Snapshot())
			}
			return nil
		}})
		if gateErr != nil {
			return gateErr
		}
		gates[manifest.ID] = gate
		if manifest.ID == "wintray" {
			wintrayGate = gate
		}
	}
	store, err = configuration.OpenHostWithLifecycle(*data, nativeModuleLifecycle(moduleState, gates), adapters...)
	if store == nil {
		return err
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	configurationReady = true
	service := host.New(store)
	service.SetAdministrator(windows.GetCurrentProcessToken().IsElevated())
	service.SetLocaleResolver(func(value settings.Settings) string { return applicationLanguage(value.Language) })
	service.RegisterModule("profiles", func() host.Module { return host.Module{ID: "profiles", State: "ready"} }, profileHandler(store, moduleState))
	configurationJobs := host.NewConfigurationQueue(service)
	for _, gate := range gates {
		gate.SetActionGuard(configurationJobs.AcquireAction)
	}
	shortcuts.apply(store.Snapshot())
	trayLanguage := &trayMenuLanguage{}
	trayLanguage.Update(store.Snapshot().Language)
	if err := systray.SetMenuTheme(store.Snapshot().Theme); err != nil {
		return err
	}
	store.SetApply(func(value settings.Settings) error {
		if err := syncProductStartup(*data, executable, value); err != nil {
			return err
		}
		if err := shortcuts.applyConfiguration(value); err != nil {
			return err
		}
		trayLanguage.Update(value.Language)
		if err := systray.SetMenuTheme(value.Theme); err != nil {
			return err
		}
		return nil
	})
	// Reapply after installing the global side-effect handler so startup errors
	// use the same visible recovery path as settings changes.
	if err := store.Recover(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	service.RegisterModule("shortcuts", func() host.Module { return host.Module{ID: "shortcuts", Name: "快捷键", State: "ready"} }, shortcuts.handle)
	go shortcuts.run(ctx, service)
	for _, manifest := range catalog.Builtins() {
		gate := gates[manifest.ID]
		handle := gate.Handle
		if manifest.ID == "always-on-top" {
			handle = aotConfigurationHandler(store, handle)
		}
		if manifest.ID == "capture" {
			handle = captureConfigurationHandler(store, handle)
		}
		if manifest.ID == "wintray" {
			handle = wintrayConfigurationHandler(*data, store, func() *modules.Lazy { return wintray }, handle, gate.Enabled)
		}
		service.RegisterModule(manifest.ID, gate.Status, handle)
	}
	control := modules.NewControl(moduleState, gates, service.FeatureCatalog)
	service.RegisterModule("modules", func() host.Module { return host.Module{ID: "modules", State: "ready"} }, moduleConfigurationHandler(store, moduleState, control))
	if store.RecoveryError() == nil && gates["wintray"].Enabled() {
		_ = gates["wintray"].Handle(host.Request{Version: 1, ID: "module-startup", Method: "snapshot"})
	} else {
		wintray.SuppressAutorun()
	}
	checkResidency := func() residency.State {
		tray := observedModule{gate: gates["wintray"], worker: wintray, idle: idleTrayLifetime(store, *autorun)}
		return residencyState(*pipe, store, tray, observedModule{gate: gates["capture"], worker: capture}, observedModule{gate: gates["always-on-top"], worker: alwaysOnTop})
	}
	service.RegisterModule("residency", func() host.Module { return host.Module{ID: "residency", Name: "后台状态", State: "ready"} }, func(r host.Request) host.Response { return residencyResponse(r, checkResidency()) })
	closeModules := func() error {
		_, err := prepareModuleShutdown(configurationJobs, control.Busy, capture, alwaysOnTop, wintray)
		return err
	}
	restart := &restartPreparation{timeout: 30 * time.Second, authorize: func(pid uint32) error {
		process, err := processidentity.Open(pid, filepath.Join(filepath.Dir(executable), "WinOne.exe"))
		if err != nil {
			return err
		}
		process.Close()
		return nil
	}, authorizeHelper: func(pid uint32) error {
		process, err := processidentity.Open(pid, filepath.Join(filepath.Dir(executable), "WinOne.exe"))
		if err != nil {
			return err
		}
		defer process.Close()
		if !process.Elevated {
			return errors.New("替换进程未获得管理员权限")
		}
		return nil
	}, prepare: func() (func(), error) {
		return prepareModuleShutdown(configurationJobs, control.Busy, capture, alwaysOnTop, wintray)
	}}
	service.RegisterModule("application", func() host.Module { return host.Module{ID: "application", Name: "WinOne", State: "ready"} }, func(request host.Request) host.Response {
		if request.Method != "open" && request.Method != "capture.window.status" {
			return restart.handle(request)
		}
		fail := func(code string, err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: code, Message: err.Error()}}
		}
		if err := restart.authorize(request.PeerPID); err != nil {
			return fail("open_identity", err)
		}
		if restart.pending() {
			return fail("restart_busy", errors.New("WinOne 正在重新启动"))
		}
		if request.Method == "capture.window.status" {
			var params struct {
				WindowID string `json:"windowId"`
			}
			if err := configjson.DecodeObject(request.Params, &params); err != nil {
				return fail("window_status", err)
			}
			status, err := captureWindows.Status(params.WindowID)
			if err != nil {
				return fail("window_status", err)
			}
			data, err := json.Marshal(status)
			if err != nil {
				return fail("window_status", err)
			}
			return host.Response{ID: request.ID, Result: data}
		}
		var params struct {
			DraftID string `json:"draftId"`
		}
		if err := configjson.DecodeObject(request.Params, &params); err != nil {
			return fail("open", err)
		}
		if params.DraftID != "" {
			status, err := captureWindows.Open(params.DraftID)
			if err != nil {
				return fail("open", err)
			}
			data, err := json.Marshal(map[string]any{"accepted": true, "windowId": status.ID})
			if err != nil {
				return fail("open", err)
			}
			return host.Response{ID: request.ID, Result: data}
		}
		if err := launchDesktop(filepath.Join(filepath.Dir(executable), "WinOne.exe"), *pipe, *data, params.DraftID); err != nil {
			return fail("open", err)
		}
		return host.Response{ID: request.ID, Result: []byte(`{"accepted":true}`)}
	})
	result := make(chan error, 1)
	openWindow := func() {
		if err := launchDesktop(filepath.Join(filepath.Dir(executable), "WinOne.exe"), *pipe, *data, ""); err != nil {
			fmt.Fprintln(os.Stderr, err)
			systray.SetTooltip("设置窗口未能打开")
			return
		}
	}
	restart.watchExit = func(owner, helper uint32) (func(func() bool) error, func(), error) {
		return openRestartExitWait(owner, helper, filepath.Join(filepath.Dir(executable), "WinOne.exe"), 30*time.Second)
	}
	restart.exit = cancel
	restart.failed = func(err error) { fmt.Fprintln(os.Stderr, "restart:", err); openWindow() }
	service.SetRuntimeWarningResolver(func() string {
		value := store.Snapshot()
		administratorWarning := ""
		if value.Startup != nil && value.Startup.PreferAdministrator {
			administratorWarning = startupWarning
		}
		return hostRuntimeWarnings(restart.latestError(), administratorWarning, startupLogError, applicationLanguage(value.Language))
	})
	service.SetRestartPendingResolver(restart.pending)
	go func() { result <- transport.Serve(ctx, listener, configurationJobs); systray.Quit() }()
	systray.Run(func() {
		systray.SetIcon(branding.Icon)
		systray.SetTooltip("WinOne")
		// Route both single clicks and the first click of a double click through
		// the same serialized menu action, without blocking the tray message loop.
		trayOpen := make(chan struct{}, 1)
		systray.SetOnTapped(func() {
			select {
			case trayOpen <- struct{}{}:
			default:
			}
		})
		open := systray.AddMenuItem("打开 WinOne", "")
		collected := systray.AddMenuItem("收纳的程序", "")
		collected.Hide()
		go syncCollectedMenu(ctx, collected, observedIcons{Gate: gates["wintray"], worker: wintray}, func() string { return applicationLanguage(store.Snapshot().Language) }, func(failure error) {
			if err := reportTrayFailure(*data, applicationLanguage(store.Snapshot().Language), failure, systray.ShowWarning); err != nil {
				fmt.Fprintln(os.Stderr, "tray feedback:", err)
			}
		})
		systray.AddSeparator()
		quit := systray.AddMenuItem("退出 WinOne", "")
		trayLanguage.Bind(open, collected, quit)
		go func() {
			if *autorun {
				startup := store.Snapshot().Startup
				if startup != nil && !startup.StartMinimized {
					openWindow()
				}
			}
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if *autorun && checkResidency().CanExit() {
						if err := closeModules(); err != nil {
							systray.SetTooltip(err.Error())
							continue
						}
						cancel()
						systray.Quit()
						return
					}
				case <-open.ClickedCh:
					openWindow()
				case <-trayOpen:
					openWindow()
				case <-quit.ClickedCh:
					if err := closeModules(); err != nil {
						fmt.Fprintln(os.Stderr, err)
						systray.SetTooltip(err.Error())
						continue
					}
					cancel()
					systray.Quit()
					return
				case <-ctx.Done():
					systray.Quit()
					return
				}
			}
		}()
	}, func() { cancel() })
	return <-result
}

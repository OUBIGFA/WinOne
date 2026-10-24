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
	"strings"
	"time"
	"winone/internal/adminstartup"
	"winone/internal/branding"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/diagnostics"
	"winone/internal/hotkeys"
	"winone/internal/logintask"
	"winone/internal/maintenance"
	"winone/internal/processidentity"
	"winone/internal/processlifetime"
	"winone/internal/quickassistant"
	"winone/internal/residency"
	"winone/internal/selection"
	"winone/internal/settings"

	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/transport"
)

func main() {
	code := 0
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	processlifetime.ExitAfterCleanup(code)
}

func run() (runErr error) {
	if executable, err := os.Executable(); err == nil {
		if handled, err := directoryLinksCommand(os.Args[1:], executable); handled {
			return err
		}
		if handled, err := desktopControl(os.Args[1:], executable, os.Stdin, os.Stdout); handled {
			return err
		}
		if handled, err := desktopBootstrap(os.Args[1:], executable, os.Stdin, os.Stdout); handled {
			return err
		}
	} else {
		return err
	}
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
	installerRelease := flag.String("installer-release", "", "installer-created completion release event")
	startupProbe := flag.String("check-startup", "", "read-only startup probe nonce")
	configurationProbe := flag.String("check-configuration", "", "read-only configuration probe nonce; reads stdin")
	profileProbe := flag.String("check-profile", "", "read-only profile probe nonce; reads stdin")
	profileRoot := flag.String("profile-root", "", "explicit profile directory for read-only maintenance")
	flag.Parse()
	probeRequested := false
	profileRootSpecified := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "profile-root" {
			profileRootSpecified = true
		}
		if f.Name == "check-startup" || f.Name == "check-configuration" || f.Name == "check-profile" {
			probeRequested = true
		}
	})
	if probeRequested {
		// No runtime options are accepted on this maintenance-only entry.
		valid := flag.NArg() == 0
		count := 0
		kinds := 0
		flag.Visit(func(f *flag.Flag) {
			count++
			if f.Name == "check-startup" || f.Name == "check-configuration" || f.Name == "check-profile" {
				kinds++
			}
			if f.Name != "check-startup" && f.Name != "check-configuration" && f.Name != "check-profile" && f.Name != "profile-root" {
				valid = false
			}
		})
		expected := 1
		if *profileProbe != "" {
			expected = 2
		}
		if !valid || count != expected || kinds != 1 || (*profileProbe != "") != (*profileRoot != "") {
			return fmt.Errorf("启动检查参数无效")
		}
		if *profileProbe != "" {
			return writeProfileProbe(os.Stdout, os.Stdin, *profileProbe, *profileRoot)
		}
		if *configurationProbe != "" {
			return writeConfigurationProbe(os.Stdout, os.Stdin, *configurationProbe)
		}
		return writeStartupProbe(os.Stdout, *startupProbe)
	}
	if profileRootSpecified {
		return errors.New("profile-root is reserved for read-only checks")
	}
	if *guardDirectory != "" || *installerPID != 0 || *installerReady != "" || *installerRelease != "" {
		if !filepath.IsAbs(*guardDirectory) || *installerPID == 0 || uint64(*installerPID) > 0xffffffff || *installerReady == "" || *autorun || *headless {
			return fmt.Errorf("安装维护参数无效")
		}
		return guardInstallation(*guardDirectory, *data, *pipe, uint32(*installerPID), *installerReady, *uninstall, *installerRelease)
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
		// Log retention runs before the first host event and before any worker
		// is started, so no other process owns the log files yet. The headless
		// test service skips it on purpose: tests must not mutate user data.
		// Failures surface as a startup warning, never block the product.
		startupLogError = diagnostics.Prune(*data, time.Now())
		logContext, stopLogMigration := context.WithTimeout(context.Background(), 5*time.Second)
		startupLogError = errors.Join(startupLogError, diagnostics.ConsolidateLegacy(logContext, *data))
		stopLogMigration()
		if startupLogError != nil {
			fmt.Fprintln(os.Stderr, "diagnostic log retention:", startupLogError)
		}
		if err := recordHostEvent(*data, "started", nil); err != nil {
			startupLogError = errors.Join(startupLogError, err)
			fmt.Fprintln(os.Stderr, "diagnostic startup log:", err)
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
	var strokeWorker *modules.Lazy
	var wintrayGate *modules.Gate
	configurationReady := false
	var store *configuration.HostStore
	var selectionModule *selectionRuntime
	quickModule := &quickAssistantRuntime{preferences: quickassistant.Defaults()}
	adapters := []configuration.ModuleAdapter{aotConfiguration(*data, func() *modules.Lazy { return alwaysOnTop }), captureConfiguration(*data, func() *modules.Lazy { return capture }), wintrayConfiguration(*data, func() *modules.Lazy { return wintray }, func() bool { return configurationReady && wintrayGate != nil && wintrayGate.Enabled() }), selectionConfiguration(*data, func() *selectionRuntime { return selectionModule }), quickAssistantConfiguration(*data, func() *quickAssistantRuntime { return quickModule })}
	adapters = append(adapters, strokeConfiguration(*data, func() *modules.Lazy { return strokeWorker }))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if *headless {
		store, err = configuration.OpenHost(*data, adapters...)
		if err != nil {
			return err
		}
		service := host.New(store)
		closeUpdates := registerUpdates(service, ctx, executablePath)
		defer closeUpdates()
		registerDiagnostics(service, *data)
		service.SetAdministrator(windows.GetCurrentProcessToken().IsElevated())
		service.SetLocaleResolver(func(value settings.Settings) string { return applicationLanguage(value.Language) })
		// Headless clients exercise the same persistence/revision handlers without
		// constructing engines, registering hotkeys, or admitting feature actions.
		unavailable := func(request host.Request) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "headless", Message: "无界面测试服务不执行模块操作"}}
		}
		for _, manifest := range catalog.Builtins() {
			handle := unavailable
			switch manifest.ID {
			case "capture":
				handle = captureConfigurationHandler(store, unavailable)
			case "always-on-top":
				handle = aotConfigurationHandler(store, unavailable)
			case "wintray":
				handle = wintrayConfigurationHandler(*data, store, func() *modules.Lazy { return nil }, unavailable, func() bool { return false })
			case "selection":
				handle = selectionConfigurationHandler(store, unavailable)
			case "quick-assistant":
				handle = quickAssistantConfigurationHandler(store, unavailable)
			case "stroke":
				handle = strokeConfigurationHandler(store, unavailable)
			}
			service.RegisterModule(manifest.ID, func() host.Module {
				return host.Module{ID: manifest.ID, Name: manifest.Name.Chinese, State: "unavailable", Detail: "无界面测试服务不启动模块"}
			}, handle)
		}
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
	strokeManifest, ok := catalog.Builtin("stroke")
	if !ok {
		return fmt.Errorf("missing stroke manifest")
	}
	trayRecord := moduleState.Snapshot().Modules["wintray"]
	wintray = modules.NewLogonLazy(ctx, trayManifest.ID, trayManifest.Name.Chinese, filepath.Join(filepath.Dir(executable), trayManifest.Worker), *pipe+"-wintray", filepath.Join(*data, "modules", "wintray"), *autorun && trayRecord.Enabled && !trayRecord.PendingStop)
	capture = modules.NewLazy(ctx, captureManifest.ID, captureManifest.Name.Chinese, filepath.Join(filepath.Dir(executable), captureManifest.Worker), *pipe+"-capture", filepath.Join(*data, "modules", "capture"), "--host-pipe", *pipe, "--stay-ready")
	alwaysOnTop = modules.NewLazy(ctx, pinManifest.ID, pinManifest.Name.Chinese, filepath.Join(filepath.Dir(executable), pinManifest.Worker), *pipe+"-always-on-top", filepath.Join(*data, "modules", "always-on-top"))
	strokeWorker = modules.NewLazy(ctx, strokeManifest.ID, strokeManifest.Name.Chinese, filepath.Join(filepath.Dir(executable), strokeManifest.Worker), *pipe+"-stroke", filepath.Join(*data, "modules", "stroke"))
	defer func() { runErr = errors.Join(runErr, strokeWorker.Close()) }()
	captureWindows := newCaptureWindows(func(id string) (<-chan error, error) {
		return startCaptureWindow(filepath.Join(filepath.Dir(executable), "WinOne.exe"), *pipe, *data, id)
	})
	shortcuts := &shortcutRuntime{registrar: hotkeys.New(), moduleState: moduleState, openDraft: func(id string) error {
		_, err := captureWindows.Open(id)
		return err
	}, openWindow: func() error {
		return launchDesktop(filepath.Join(filepath.Dir(executable), "WinOne.exe"), *pipe, *data, "")
	}, language: func() string { return applicationLanguage(store.Snapshot().Language) }}
	defer shortcuts.registrar.Close()
	gates := map[string]*modules.Gate{}
	strokeChanged := make(chan struct{}, 1)
	var refreshCaptureReadiness func()
	for _, entry := range []struct {
		manifest catalog.Manifest
		runtime  *modules.Lazy
	}{{trayManifest, wintray}, {captureManifest, capture}, {pinManifest, alwaysOnTop}, {strokeManifest, strokeWorker}} {
		runtime := entry.runtime
		manifest := entry.manifest
		start := func() error { runtime.Resume(); return nil }
		stop := runtime.Close
		status := runtime.Status
		if manifest.ID != "wintray" && manifest.ID != "stroke" {
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
		if manifest.ID == "stroke" {
			start = func() error { return startStroke(runtime) }
			status = func() host.Module { return strokeStatus(runtime) }
		}
		if manifest.ID == "capture" {
			start = func() error { return prepareCapture(runtime) }
		}
		gate, gateErr := modules.NewGate(manifest.ID, manifest.Name.Chinese, moduleState, modules.LifecycleHooks{Start: start, Stop: stop, Status: status, Handle: runtime.Handle, IntentChanged: func() error {
			if manifest.ID == "stroke" {
				select {
				case strokeChanged <- struct{}{}:
				default:
				}
			}
			if store != nil {
				shortcuts.apply(store.Snapshot())
			}
			if manifest.ID == "capture" && refreshCaptureReadiness != nil {
				refreshCaptureReadiness()
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
	selectionModule = &selectionRuntime{ctx: ctx, reader: filepath.Join(filepath.Dir(executable), "selection", "WinOne.Selection.exe"), directory: filepath.Dir(executable), preferences: selection.Defaults()}
	selectionGate, gateErr := modules.NewGate("selection", "划词助手", moduleState, modules.LifecycleHooks{Start: selectionModule.start, Stop: selectionModule.stop, Status: selectionModule.status, Handle: selectionModule.handle, IntentChanged: func() error {
		if store != nil {
			shortcuts.apply(store.Snapshot())
		}
		return nil
	}})
	if gateErr != nil {
		return gateErr
	}
	gates["selection"] = selectionGate
	quickGate, gateErr := modules.NewGate("quick-assistant", "快捷助手", moduleState, modules.LifecycleHooks{Start: quickModule.start, Stop: quickModule.stop, Status: quickModule.status, Handle: quickModule.handle, IntentChanged: func() error {
		if store != nil {
			shortcuts.apply(store.Snapshot())
		}
		return nil
	}})
	if gateErr != nil {
		return gateErr
	}
	gates["quick-assistant"] = quickGate
	var configurationJobs *host.ConfigurationQueue
	packages, packageErr := openInstalledPackages(ctx, *data, *pipe, moduleState, func() (func(), bool) {
		if configurationJobs == nil {
			return nil, false
		}
		return configurationJobs.AcquireAction()
	})
	if packageErr != nil {
		return packageErr
	}
	defer func() { runErr = errors.Join(runErr, packages.Close()) }()
	packages.addGates(gates)
	adapters = append(adapters, packages.adapters()...)
	var service *host.Service
	control := modules.NewControl(moduleState, gates, func() []catalog.Manifest {
		if service == nil {
			return catalog.Builtins()
		}
		return service.FeatureCatalog()
	})
	store, err = configuration.OpenHostWithLifecycle(*data, nativeModuleLifecycle(moduleState, control.LifecycleGates, packages.isolated), adapters...)
	if store == nil {
		return err
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	configurationReady = true
	service = host.New(store)
	if err = packages.bind(service, control, store); err != nil {
		return err
	}
	resultWindows := registerResultServices(service, ctx, *data, executable, *pipe, func() (string, string) {
		value := store.Snapshot()
		return value.Theme, applicationLanguage(value.Language)
	})
	resultWindows.ai.captureEnabled = func() bool { return store.RecoveryError() == nil && gates["capture"].Enabled() }
	refreshCaptureReadiness = resultWindows.ai.refreshLocalOCRStandby
	// Start local models as soon as configuration is available, alongside the
	// rest of Host startup rather than behind WinTray/first capture startup.
	refreshCaptureReadiness()
	defer func() { runErr = errors.Join(runErr, resultWindows.dispose()) }()
	defer resultWindows.ai.stopMagpieMaintenance()
	defer selectionModule.stop()
	chatControl := registerChat(service, ctx, *data, executable, resultWindows.ai, func() string { return applicationLanguage(store.Snapshot().Language) })
	if err = registerDirectoryLinks(service, *data); err != nil {
		return err
	}
	closeInterconnect, err := registerInterconnect(service, *data, executable, *pipe, func() string { return applicationLanguage(store.Snapshot().Language) })
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, closeInterconnect()) }()
	defer func() { runErr = errors.Join(runErr, chatControl.stop()) }()
	selectionModule.results = resultWindows
	quickModule.results = resultWindows
	registerDiagnostics(service, *data)
	service.SetAdministrator(windows.GetCurrentProcessToken().IsElevated())
	service.SetLocaleResolver(func(value settings.Settings) string { return applicationLanguage(value.Language) })
	service.RegisterModule("profiles", func() host.Module { return host.Module{ID: "profiles", State: "ready"} }, profileHandler(store, moduleState))
	closeUpdates := registerUpdates(service, ctx, executable)
	defer closeUpdates()
	configurationJobs = host.NewConfigurationQueue(service)
	chatControl.guard = configurationJobs.AcquireAction
	selectionModule.actionGuard = configurationJobs.AcquireAction
	resultWindows.ai.mu.Lock()
	resultWindows.ai.gatewayGuard = configurationJobs.AcquireAction
	resultWindows.ai.mu.Unlock()
	selectionModule.suspended = func() bool { shortcuts.mu.Lock(); defer shortcuts.mu.Unlock(); return shortcuts.recording != nil }
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
		if manifest.ID == "selection" {
			handle = selectionConfigurationHandler(store, handle)
		}
		if manifest.ID == "quick-assistant" {
			handle = quickAssistantConfigurationHandler(store, handle)
		}
		if manifest.ID == "stroke" {
			handle = strokeConfigurationHandler(store, handle)
		}
		service.RegisterModule(manifest.ID, gate.Status, handle)
	}
	service.RegisterModule("modules", func() host.Module { return host.Module{ID: "modules", State: "ready"} }, moduleConfigurationHandler(store, moduleState, control, packages.moduleHandler(service, control, store)))
	if store.RecoveryError() == nil && gates["wintray"].Enabled() {
		_ = gates["wintray"].Handle(host.Request{Version: 1, ID: "module-startup", Method: "snapshot"})
	} else {
		wintray.SuppressAutorun()
	}
	if store.RecoveryError() == nil && gates["capture"].Enabled() {
		// Do not delay the main window or initialize unrelated feature engines.
		// Gate.Handle rechecks enablement if shutdown/disable races this request.
		go func() {
			response := gates["capture"].Handle(host.Request{Version: 1, ID: "capture-standby", Method: "prepare"})
			if response.Error != nil && ctx.Err() == nil {
				fmt.Fprintln(os.Stderr, "截图待命准备：", response.Error.Message)
			}
		}()
	}
	if store.RecoveryError() == nil && selectionGate.Enabled() {
		if err := selectionModule.start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	if store.RecoveryError() == nil && quickGate.Enabled() {
		_ = quickModule.start()
	}
	if store.RecoveryError() == nil && gates["stroke"].Enabled() {
		// Use the same transition as the module page: preserve startup errors
		// in its status and allow retry while the saved intent remains enabled.
		if err := gates["stroke"].SetEnabled(moduleState.Snapshot().Revision, true); err != nil {
			_ = recordHostEvent(*data, "stroke.start.failed", err)
			fmt.Fprintln(os.Stderr, err)
		}
	}
	go superviseStroke(ctx, configurationJobs, moduleState, gates["stroke"], strokeWorker, *data, func() bool { return store.RecoveryError() == nil }, strokeChanged)
	checkResidency := func() residency.State {
		tray := observedModule{gate: gates["wintray"], worker: wintray, idle: idleTrayLifetime(store, *autorun)}
		state := residencyState(*pipe, store, tray, observedModule{gate: gates["capture"], worker: capture}, observedModule{gate: gates["always-on-top"], worker: alwaysOnTop})
		if resultWindows.active() {
			state.Reasons = append(state.Reasons, "result-windows")
		}
		if chatControl.active() {
			state.Reasons = append(state.Reasons, "chat")
		}
		if resultWindows.ai.gateway.Status().Running {
			state.Reasons = append(state.Reasons, "ai-gateway")
		}
		if packages.Running() {
			state.Reasons = append(state.Reasons, "optional-modules")
		}
		if quickGate.Enabled() && store.Snapshot().QuickAssistantBinding() != "" {
			state.Reasons = append(state.Reasons, "quick-assistant-shortcut")
		}
		if selectionGate.Enabled() {
			state.Reasons = append(state.Reasons, "selection-listener")
		}
		if gates["stroke"].Enabled() {
			state.Reasons = append(state.Reasons, "mouse-gestures")
		}
		if store.Snapshot().OpenBinding() != "" {
			state.Reasons = append(state.Reasons, "open-winone-shortcut")
		}
		return state
	}
	service.RegisterModule("residency", func() host.Module { return host.Module{ID: "residency", Name: "后台状态", State: "ready"} }, func(r host.Request) host.Response { return residencyResponse(r, checkResidency()) })
	closeModules := func() error {
		if err := chatControl.stop(); err != nil {
			chatControl.resume()
			return err
		}
		defer chatControl.resume()
		if err := selectionModule.stop(); err != nil {
			return err
		}
		if err := resultWindows.stop(); err != nil {
			if selectionGate.Enabled() {
				_ = selectionModule.start()
			}
			return err
		}
		resultWindows.ai.closeRouting()
		resultWindows.ai.gateway.Close()
		_, err := prepareModuleShutdown(configurationJobs, control.Busy, capture, alwaysOnTop, wintray, packages, strokeWorker)
		if err != nil {
			if gates["stroke"].Enabled() {
				err = errors.Join(err, startStroke(strokeWorker))
			}
			if restartErr := resultWindows.ai.gateway.Resume(); restartErr != nil {
				return errors.Join(err, restartErr)
			}
		}
		if err != nil && selectionGate.Enabled() {
			_ = selectionModule.start()
		}
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
		process, err := openAdministratorHelper(pid, filepath.Join(filepath.Dir(executable), "WinOne.exe"))
		if err != nil {
			return err
		}
		defer process.Close()
		if !process.Elevated {
			return errors.New("替换进程未获得管理员权限")
		}
		return nil
	}, prepare: func() (func(), error) {
		if err := chatControl.stop(); err != nil {
			chatControl.resume()
			return nil, err
		}
		defer chatControl.resume()
		if err := selectionModule.stop(); err != nil {
			return nil, err
		}
		if err := resultWindows.stop(); err != nil {
			if selectionGate.Enabled() {
				_ = selectionModule.start()
			}
			return nil, err
		}
		resultWindows.ai.closeRouting()
		resultWindows.ai.gateway.Close()
		rollback, err := prepareModuleShutdown(configurationJobs, control.Busy, capture, alwaysOnTop, wintray, packages, strokeWorker)
		if err != nil {
			if gates["stroke"].Enabled() {
				err = errors.Join(err, startStroke(strokeWorker))
			}
			if restartErr := resultWindows.ai.gateway.Resume(); restartErr != nil {
				err = errors.Join(err, restartErr)
			}
			if selectionGate.Enabled() {
				_ = selectionModule.start()
			}
			return nil, err
		}
		return func() {
			_ = resultWindows.ai.gateway.Resume()
			rollback()
			if gates["stroke"].Enabled() {
				if err := startStroke(strokeWorker); err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			}
			if selectionGate.Enabled() {
				_ = selectionModule.start()
			}
		}, nil
	}}
	service.RegisterModule("application", func() host.Module { return host.Module{ID: "application", Name: "WinOne", State: "ready"} }, func(request host.Request) host.Response {
		if request.Method != "open" && request.Method != "capture.window.status" {
			return restart.handle(request)
		}
		fail := func(code string, err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: code, Message: err.Error()}}
		}
		// Rust delegates startup to its verified Host companion. That helper
		// may open the window, but still has no prepare/commit/cancel authority.
		origin, err := openAdministratorHelper(request.PeerPID, filepath.Join(filepath.Dir(executable), "WinOne.exe"))
		if err != nil {
			return fail("open_identity", err)
		}
		origin.Close()
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
		return strings.TrimSpace(strings.Join([]string{hostRuntimeWarnings(restart.latestError(), administratorWarning, startupLogError, applicationLanguage(value.Language)), packages.Warning(applicationLanguage(value.Language))}, "\n"))
	})
	service.SetRestartPendingResolver(restart.pending)
	go func() { result <- transport.Serve(ctx, listener, configurationJobs); systray.Quit() }()
	systray.Run(func() {
		systray.SetIcon(branding.Icon)
		systray.SetTooltip("WinOne")
		resultWindows.ai.startMagpieMaintenance(func() string { return applicationLanguage(store.Snapshot().Language) })
		// Route both single clicks and the first click of a double click through
		// the same serialized menu action, without blocking the tray message loop.
		trayOpen := make(chan struct{}, 1)
		systray.SetOnTapped(func() {
			select {
			case trayOpen <- struct{}{}:
			default:
			}
		})
		// Reserve the group divider before the footer, but keep an empty
		// collection compact: no leading separator or invisible placeholder.
		collectedSeparator := systray.NewMenuSeparator()
		open := systray.AddMenuItem("打开 WinOne", "")
		go syncCollectedMenu(ctx, observedIcons{Gate: gates["wintray"], worker: wintray}, collectedSeparator, func() string { return applicationLanguage(store.Snapshot().Language) }, func(failure error) {
			if err := reportTrayFailure(*data, applicationLanguage(store.Snapshot().Language), failure, systray.ShowWarning); err != nil {
				fmt.Fprintln(os.Stderr, "tray feedback:", err)
			}
		})
		quit := systray.AddMenuItem("退出 WinOne", "")
		if err := setTrayMenuIcons(open, quit); err != nil {
			if reportErr := reportTrayFailure(*data, applicationLanguage(store.Snapshot().Language), err, systray.ShowWarning); reportErr != nil {
				fmt.Fprintln(os.Stderr, "tray menu icons:", reportErr)
			}
		}
		trayLanguage.Bind(open, quit)
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
					if err := prepareApplicationExit(func() error { return closeSettingsForExit(executable, *pipe) }, closeModules); err != nil {
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

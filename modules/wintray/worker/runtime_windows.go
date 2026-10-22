//go:build windows

package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lxn/walk"
	"golang.org/x/sys/windows"
	"winone/internal/diagnostics"
	"winone/internal/host"
	"winone/internal/residency"
	"winone/internal/transport"
	"winone/modules/wintray/internal/config"
	"winone/modules/wintray/internal/ipc"
	"winone/modules/wintray/internal/logging"
	"winone/modules/wintray/internal/orchestrator"
	"winone/modules/wintray/internal/startup"
	"winone/modules/wintray/internal/tray"
	"winone/modules/wintray/internal/traybox"
)

type job struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Runtime owns the native message loop while Manager owns configuration transactions.
type Runtime struct {
	ctx          context.Context
	cancel       context.CancelFunc
	window       *walk.MainWindow
	box          *traybox.Box
	hosted       *tray.Host
	orchestrator *orchestrator.Service
	tasks        *startup.AppTasks
	productLogon interface {
		Sync(string, string) error
		Remove() error
	}
	productExe                string
	productStartupOwnedByHost bool
	manager                   *Manager
	logger                    *logging.Logger
	jobsMu                    sync.Mutex
	jobs                      map[string]job
	sequence                  uint64
	work                      sync.WaitGroup
	closing                   bool
	deactivating              bool
	deactivated               bool
	recoverOnly               bool
	effectsMu                 sync.Mutex
	lastError                 string
	startupError              string
	initializationState       string
	initializationError       string
	idleSince                 time.Time
	bootstrapSnapshot         Snapshot
	startupMu                 sync.Mutex
	allowSystemStartup        bool
	readyName                 string
	readyRelease              func()
	autorun                   bool
	limitsChecked             bool
	limitsRevision            string
	runningLimits             int
}

// Run keeps Walk only for its native message loop and per-program tray callbacks.
func Run(parent context.Context, pipe, dataDir string, autorun, recoverOnly bool) error {
	defaultDir, err := config.AppDirWithError()
	if err != nil {
		return err
	}
	allowSystemStartup := strings.EqualFold(filepath.Clean(defaultDir), filepath.Clean(dataDir))
	if err := config.SetWorkerDataDirectory(dataDir); err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	logOutput, err := diagnostics.NewModuleWriter(dataDir, "wintray")
	if err != nil {
		return err
	}
	logger := logging.NewOutput(logOutput)
	defer logger.Close()
	store := config.NewStore(filepath.Join(dataDir, "settings.json"))
	projectionPath := filepath.Join(dataDir, "managed-settings.json")
	_, projectionErr := os.Stat(projectionPath)
	managed := projectionErr == nil
	if projectionErr != nil && !errors.Is(projectionErr, os.ErrNotExist) {
		return projectionErr
	}
	var settings config.Settings
	if managed {
		projection, readErr := readManagedProjection(projectionPath)
		if readErr != nil {
			return readErr
		}
		settings = projection.Settings
	} else {
		settings, err = store.LoadWithError()
		if err != nil {
			return err
		}
		if _, err = os.Stat(store.Path()); errors.Is(err, os.ErrNotExist) {
			settings.RunAtLogon = false
			settings.ExitAfterManagedAppsCompleted = false
		}
	}
	window, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	defer window.Dispose()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	tasks, err := startup.NewAppTasks()
	if err != nil {
		return err
	}
	tasks.Log = logger.Info
	r := &Runtime{ctx: ctx, cancel: cancel, window: window, box: traybox.NewBox(exe, logger), hosted: tray.NewHost(settings.Language, logger, orchestrator.FindConsoleWindow), orchestrator: orchestrator.NewService(orchestrator.NewWin32WindowEnumerator(), orchestrator.NewWin32WindowManager(), logger), tasks: tasks, logger: logger, jobs: map[string]job{}}
	r.hosted.SetTrayBox(r.box)
	r.hosted.SetExternalMenu(managed)
	r.productLogon, err = startup.NewProductLogonTask()
	if err != nil {
		return err
	}
	r.productExe = filepath.Join(filepath.Dir(exe), "WinOne.Host.exe")
	r.productStartupOwnedByHost = managed
	r.allowSystemStartup = allowSystemStartup
	r.recoverOnly = recoverOnly
	if managed {
		r.manager, err = newManagedManager(store, projectionPath, r)
		if err != nil {
			return err
		}
	} else {
		r.manager = NewManager(store, settings, r)
	}
	settings = r.manager.Snapshot().Settings
	r.bootstrapSnapshot = r.manager.Snapshot()
	r.initializationState = "starting"
	if recoverOnly {
		r.initializationState = "ready"
	}
	defer r.box.Close()
	defer func() {
		if err := r.hosted.RestoreAll(); err != nil {
			logger.Warn("退出时仍有窗口未恢复：" + err.Error())
		}
	}()
	readyName := "WinOne_WinTray_Ready"
	if !allowSystemStartup {
		readyName += fmt.Sprintf("-%x", sha256.Sum256([]byte(dataDir)))
	}
	r.readyName = readyName
	r.autorun = autorun
	defer func() {
		if r.readyRelease != nil {
			r.readyRelease()
		}
	}()
	listener, err := transport.Listen(pipe)
	if err != nil {
		return err
	}
	defer listener.Close()
	go func() {
		if err := transport.Serve(ctx, listener, r); err != nil {
			logger.Error(err.Error())
			cancel()
		}
	}()
	// Reserve the lifetime timer before starting the drain waiter.
	r.work.Add(1)
	// Keep the message loop available until all background callbacks have returned.
	go func() { <-ctx.Done(); r.work.Wait(); window.Synchronize(func() { walk.App().Exit(0) }) }()
	window.Starting().Attach(func() {
		if !recoverOnly {
			r.startJob(func() (result any, resultErr error) {
				defer func() {
					r.jobsMu.Lock()
					defer r.jobsMu.Unlock()
					r.initializationState = "ready"
					if resultErr != nil {
						r.initializationState = "failed"
						r.initializationError = resultErr.Error()
					}
				}()
				if pending := r.manager.Snapshot().RecoveryError; pending != "" {
					return nil, errors.New(pending)
				}
				active, err := r.manager.Activate(false)
				if err != nil {
					return nil, err
				}
				if autorun && active.Settings.RunAtLogon {
					return r.startConfiguredApps(active.Settings)
				}
				return nil, nil
			})
		}
		// The timer does not poll on the UI thread; only host/tray operations marshal there.
		go func() {
			defer r.work.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case now := <-ticker.C:
					r.checkRunLimits(now)
					if r.tryIdleExit(now) {
						return
					}
				}
			}
		}()
	})
	window.Hide()
	window.Run()
	return nil
}

func (r *Runtime) checkRunLimits(now time.Time) {
	r.effectsMu.Lock()
	defer r.effectsMu.Unlock()
	r.jobsMu.Lock()
	inactive := r.recoverOnly || r.deactivating || r.deactivated || r.closing
	r.jobsMu.Unlock()
	if inactive {
		return
	}
	snapshot := r.manager.Snapshot()
	if snapshot.RecoveryError != "" {
		return
	}
	check := r.orchestrator.EndOverduePrograms(snapshot.Settings.ManagedApps, now)
	r.jobsMu.Lock()
	r.runningLimits = check.Running
	r.limitsChecked = true
	r.limitsRevision = snapshot.Revision
	r.jobsMu.Unlock()
}

func (r *Runtime) startConfiguredApps(settings config.Settings) (any, error) {
	r.startupMu.Lock()
	defer r.startupMu.Unlock()
	results := r.orchestrator.StartManagedApps(r.ctx, settings, func(a config.ManagedAppEntry, result orchestrator.Result) { r.adopt(a, result) })
	err := startupResultError(settings.Language, results)
	r.jobsMu.Lock()
	r.startupError = ""
	if err != nil {
		r.startupError = err.Error()
	}
	r.jobsMu.Unlock()
	return results, err
}

func (r *Runtime) onUI(action func() error) error {
	done := make(chan error, 1)
	r.window.Synchronize(func() {
		if r.ctx.Err() != nil {
			done <- r.ctx.Err()
			return
		}
		done <- action()
	})
	select {
	case err := <-done:
		return err
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

// Reconcile retains upstream task migration and icon restoration semantics.
// Independent WinTray owns its own profile and lifetime; its presence never
// blocks this Host-owned module or the installer's recovery-only worker.
func (r *Runtime) Reconcile(settings config.Settings, allowElevated bool) error {
	if err := r.syncTasks(settings, allowElevated); err != nil {
		return err
	}
	return r.onUI(func() error {
		r.hosted.SetLanguage(settings.Language)
		if err := r.box.SetPaths(config.CollectedTrayIconPaths(settings)); err != nil {
			return err
		}
		if r.readyRelease == nil {
			release, err := ipc.MarkReady(r.readyName)
			if err != nil {
				return err
			}
			r.readyRelease = release
		}
		return nil
	})
}

// Non-default profiles are test sandboxes and may never alter real login triggers.
func (r *Runtime) syncTasks(settings config.Settings, allowElevated bool) error {
	if !r.allowSystemStartup {
		if settings.RunAtLogon {
			return errors.New("独立测试配置不允许修改系统自启")
		}
		return nil
	}
	if err := r.tasks.Sync(settings, allowElevated); err != nil {
		return err
	}
	if r.productStartupOwnedByHost {
		return nil
	}
	if settings.RunAtLogon {
		return r.productLogon.Sync(r.productExe, "--autorun")
	}
	return r.productLogon.Remove()
}

func (r *Runtime) adopt(a config.ManagedAppEntry, result orchestrator.Result) {
	if result.Hidden == nil {
		return
	}
	w := tray.HostedWindow{Name: a.Name, ExePath: a.ExePath, Handle: result.Hidden.Handle, ProcessID: result.Hidden.ProcessID}
	for attempt := 0; attempt < 30; attempt++ {
		err := r.onUI(func() error { return r.hosted.TryAdd(w) })
		if err == nil { return }
		// Short scripts can finish before console ownership reaches the UI.
		// An invalid PID is not a transient icon failure; there is no window
		// left to recover. Permission/creation failures still retain retries.
		if errors.Is(err, tray.ErrHostedProcessUnavailable) && errors.Is(err, windows.ERROR_INVALID_PARAMETER) { return }
		select {
		case <-r.ctx.Done():
			tray.RestoreWindow(w, orchestrator.FindConsoleWindow, r.logger)
			return
		case <-time.After(time.Second):
		}
	}
	tray.RestoreWindow(w, orchestrator.FindConsoleWindow, r.logger)
}

func (r *Runtime) startJob(run func() (any, error)) job {
	r.jobsMu.Lock()
	defer r.jobsMu.Unlock()
	if r.closing || r.deactivating || r.deactivated || r.recoverOnly || r.ctx.Err() != nil {
		return job{State: "failed", Error: "模块正在退出"}
	}
	return r.enqueueJobLocked(run)
}

func (r *Runtime) enqueueJobLocked(run func() (any, error)) job {
	r.idleSince = time.Time{}
	r.sequence++
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), r.sequence)
	j := job{ID: id, State: "running"}
	r.jobs[id] = j
	r.work.Add(1)
	go func() {
		defer r.work.Done()
		result, err := run()
		r.jobsMu.Lock()
		defer r.jobsMu.Unlock()
		finished := job{ID: id, State: "completed", Result: result}
		if err != nil {
			finished.State = "failed"
			finished.Error = err.Error()
			r.logger.Warn(err.Error())
			r.lastError = err.Error()
		} else {
			r.lastError = ""
		}
		r.jobs[id] = finished
		if len(r.jobs) > 128 {
			for key, item := range r.jobs {
				if item.State != "running" && key != id {
					delete(r.jobs, key)
					break
				}
			}
		}
	}()
	return j
}

// Handle queues slow native mutations and returns job IDs, not premature success.
func (r *Runtime) Handle(request host.Request) host.Response {
	response := host.Response{ID: request.ID}
	fail := func(err error) host.Response {
		response.Error = &host.RPCError{Code: "wintray", Message: err.Error()}
		return response
	}
	if request.Version != 1 || request.ID == "" {
		return fail(errors.New("无效的模块请求"))
	}
	var result any
	switch request.Method {
	case "deactivate":
		result = r.startDeactivation()
	case "uninstall.prepare":
		result = r.startResourceRelease(true)
	case "lifetime":
		snapshot := r.manager.Snapshot()
		state := residency.State{Requested: r.autorun && snapshot.Settings.ExitAfterManagedAppsCompleted, Reasons: []string{}}
		r.jobsMu.Lock()
		if !r.limitsChecked || r.limitsRevision != snapshot.Revision {
			state.Reasons = append(state.Reasons, "正在检查定时任务")
		}
		if r.runningLimits > 0 {
			state.Reasons = append(state.Reasons, "有程序等待定时退出")
		}
		for _, j := range r.jobs {
			if j.State == "running" {
				state.Reasons = append(state.Reasons, "程序任务尚未完成")
				break
			}
		}
		if r.lastError != "" {
			state.Reasons = append(state.Reasons, "程序任务需要处理错误")
		}
		if r.startupError != "" {
			state.Reasons = append(state.Reasons, "登录启动存在失败程序")
		}
		r.jobsMu.Unlock()
		if snapshot.RecoveryError != "" {
			state.Reasons = append(state.Reasons, "程序设置需要恢复")
		}
		if len(config.CollectedTrayIconPaths(snapshot.Settings)) > 0 {
			state.Reasons = append(state.Reasons, "已启用托盘图标收纳")
		}
		if err := r.onUI(func() error {
			if r.hosted.Count() > 0 {
				state.Reasons = append(state.Reasons, "有托管程序窗口")
			}
			return nil
		}); err != nil {
			return fail(err)
		}
		result = state
	case "import.preview":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil {
			return fail(err)
		}
		preview, err := PreviewImport(args.Path)
		if err != nil {
			return fail(err)
		}
		result = preview
	case "snapshot":
		r.jobsMu.Lock()
		initializationState, initializationError := r.initializationState, r.initializationError
		r.jobsMu.Unlock()
		// Readiness polling must stay responsive while native startup owns the
		// manager lock. Its immutable bootstrap snapshot remains valid until then.
		snapshot := r.bootstrapSnapshot
		if initializationState != "starting" {
			snapshot = r.manager.Snapshot()
		}
		r.jobsMu.Lock()
		snapshot.InitializationState = initializationState
		snapshot.InitializationError = initializationError
		snapshot.ProcessID = uint32(os.Getpid())
		snapshot.BackgroundError = r.lastError
		snapshot.StartupError = r.startupError
		r.jobsMu.Unlock()
		result = snapshot
	case "startup.retry":
		snapshot := r.manager.Snapshot()
		if snapshot.RecoveryError != "" {
			return fail(errors.New(snapshot.RecoveryError))
		}
		result = r.startJob(func() (any, error) {
			return r.startConfiguredApps(snapshot.Settings)
		})
	case "settings.save":
		var args struct {
			Revision string          `json:"revision"`
			Settings config.Settings `json:"settings"`
			History  json.RawMessage `json:"history,omitempty"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil {
			return fail(err)
		}
		result = r.startJob(func() (any, error) {
			return r.manager.SaveImported(args.Revision, args.Settings, args.History)
		})
	case "settings.reload":
		result = r.startJob(func() (any, error) {
			snapshot, err := r.manager.ReloadManaged(true)
			if err == nil {
				r.markReconciled()
			}
			return snapshot, err
		})
	case "history.merge":
		var args struct {
			History json.RawMessage `json:"history"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil {
			return fail(err)
		}
		result = r.startJob(func() (any, error) {
			if err := r.manager.MergeHistory(args.History); err != nil {
				return nil, err
			}
			return map[string]bool{"merged": true}, nil
		})
	case "recover":
		result = r.startJob(func() (any, error) {
			snapshot, err := r.manager.Recover(true)
			if err == nil {
				r.markReconciled()
			}
			return snapshot, err
		})
	case "launch":
		var args struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil {
			return fail(err)
		}
		snapshot := r.manager.Snapshot()
		if snapshot.RecoveryError != "" {
			return fail(errors.New(snapshot.RecoveryError))
		}
		settings := snapshot.Settings
		var entry *config.ManagedAppEntry
		for _, a := range settings.ManagedApps {
			if a.ID == args.ID {
				copy := a
				entry = &copy
				break
			}
		}
		if entry == nil {
			return fail(errors.New("程序不存在"))
		}
		result = r.startJob(func() (any, error) {
			out := r.orchestrator.StartNow(r.ctx, *entry, settings.CloseWindowRetrySeconds)
			r.adopt(*entry, out)
			if !out.Managed {
				return out, errors.New(out.Message)
			}
			return out, nil
		})
	case "program.state", "program.window", "program.stop":
		value, err := r.programAction(request.Method, request.Params)
		if err != nil {
			return fail(err)
		}
		result = value
	case "jobs.get":
		var args struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil {
			return fail(err)
		}
		r.jobsMu.Lock()
		j, ok := r.jobs[args.ID]
		r.jobsMu.Unlock()
		if !ok {
			return fail(errors.New("任务不存在或已过期"))
		}
		result = j
	case "icons":
		type iconView struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Image   []byte `json:"image,omitempty"`
			ExePath string `json:"exePath"`
			Hosted  bool   `json:"hosted,omitempty"`
		}
		icons := []iconView{}
		if err := r.onUI(func() error {
			for _, icon := range r.box.Icons() {
				var image bytes.Buffer
				if icon.Image != nil {
					if err := png.Encode(&image, icon.Image); err != nil {
						return err
					}
				}
				icons = append(icons, iconView{icon.Identity(), icon.DisplayName(), image.Bytes(), icon.ExePath, false})
			}
			for _, item := range r.hosted.MenuEntries() {
				icons = append(icons, iconView{item.ID, item.Name, item.Image, item.ExePath, true})
			}
			return nil
		}); err != nil {
			return fail(err)
		}
		result = icons
	case "icon.activate":
		var args struct {
			ID     string `json:"id"`
			Right  bool   `json:"right"`
			Action string `json:"action,omitempty"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil {
			return fail(err)
		}
		var automatic *traybox.Icon
		if err := r.onUI(func() error {
			if strings.HasPrefix(args.ID, "hosted:") {
				if args.Right {
					return errors.New("请使用 WinOne 的程序操作菜单")
				}
				return r.hosted.ActivateMenu(args.ID, args.Action)
			}
			if args.Action != "" && args.Action != "double" && args.Action != "open" {
				return errors.New("不支持的托盘图标操作")
			}
			if args.Action == "double" && args.Right {
				return errors.New("托盘单击和双击操作冲突")
			}
			for _, icon := range r.box.Icons() {
				if icon.Identity() == args.ID {
					if args.Action == "open" && !args.Right {
						copy := icon
						automatic = &copy
						return nil
					}
					action := traybox.ActionClick
					if args.Right {
						action = traybox.ActionRightClick
					} else if args.Action == "double" {
						action = traybox.ActionDoubleClick
					}
					return traybox.Activate(icon, action)
				}
			}
			return traybox.ErrIconGone
		}); err != nil {
			return fail(err)
		}
		// Observe outside the UI thread so collection and native callbacks
		// keep responding while the target application opens.
		if automatic != nil {
			if err := traybox.OpenIcon(r.ctx, *automatic, func() (traybox.Icon, error) {
				var current traybox.Icon
				err := r.onUI(func() error {
					for _, icon := range r.box.Icons() {
						if icon.Identity() == args.ID {
							current = icon
							return nil
						}
					}
					return traybox.ErrIconGone
				})
				return current, err
			}); err != nil {
				return fail(err)
			}
		}
		result = map[string]bool{"delivered": true}
	case "shutdown":
		r.jobsMu.Lock()
		r.closing = true
		r.jobsMu.Unlock()
		// Closing the collector may fail; keep the worker available for recovery in that case.
		if err := r.onUI(func() error {
			if err := r.box.Close(); err != nil {
				return err
			}
			return nil
		}); err != nil {
			r.jobsMu.Lock()
			r.closing = false
			r.jobsMu.Unlock()
			return fail(err)
		}
		if err := r.restoreHostedWindows(); err != nil {
			r.jobsMu.Lock()
			r.closing = false
			r.jobsMu.Unlock()
			return fail(err)
		}
		result = map[string]bool{"stopping": true}
		go func() { time.Sleep(100 * time.Millisecond); r.cancel() }()
	default:
		return fail(errors.New("不支持的模块操作"))
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fail(err)
	}
	response.Result = data
	return response
}

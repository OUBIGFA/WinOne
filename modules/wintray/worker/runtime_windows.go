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
	"winone/internal/host"
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
	ctx                context.Context
	cancel             context.CancelFunc
	window             *walk.MainWindow
	box                *traybox.Box
	hosted             *tray.Host
	orchestrator       *orchestrator.Service
	tasks              *startup.AppTasks
	productLogon       *startup.LogonTask
	productExe         string
	manager            *Manager
	logger             *logging.Logger
	jobsMu             sync.Mutex
	jobs               map[string]job
	sequence           uint64
	work               sync.WaitGroup
	closing            bool
	lastError          string
	allowSystemStartup bool
}

// Run keeps Walk only for its native message loop and per-program tray callbacks.
func Run(parent context.Context, pipe, dataDir string, autorun bool) error {
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
	logger, err := logging.New(dataDir)
	if err != nil {
		return err
	}
	defer logger.Close()
	store := config.NewStore(filepath.Join(dataDir, "settings.json"))
	settings, err := store.LoadWithError()
	if err != nil {
		return err
	}
	if _, err = os.Stat(filepath.Join(dataDir, "settings.json")); errors.Is(err, os.ErrNotExist) {
		settings.RunAtLogon = false
		settings.ExitAfterManagedAppsCompleted = false
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
	r.productLogon, err = startup.NewProductLogonTask()
	if err != nil {
		return err
	}
	r.productExe = filepath.Join(filepath.Dir(exe), "WinOne.Host.exe")
	r.allowSystemStartup = allowSystemStartup
	r.manager = NewManager(store, settings, r)
	if err = r.box.SetPaths(config.CollectedTrayIconPaths(settings)); err != nil {
		return err
	}
	defer r.box.Close()
	defer r.hosted.RestoreAll()
	readyName := "WinOne_WinTray_Ready"
	if !allowSystemStartup {
		readyName += fmt.Sprintf("-%x", sha256.Sum256([]byte(dataDir)))
	}
	release, err := ipc.MarkReady(readyName)
	if err != nil {
		return err
	}
	defer release()
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
		r.startJob(func() (any, error) {
			if err := r.syncTasks(settings, false); err != nil {
				return nil, err
			}
			if autorun && settings.RunAtLogon {
				return r.orchestrator.StartManagedApps(ctx, settings, func(a config.ManagedAppEntry, result orchestrator.Result) { r.adopt(a, result) }), nil
			}
			return nil, nil
		})
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
					r.orchestrator.EndOverduePrograms(r.manager.Snapshot().Settings.ManagedApps, now)
				}
			}
		}()
	})
	window.Hide()
	window.Run()
	return nil
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
func (r *Runtime) Reconcile(settings config.Settings, allowElevated bool) error {
	if err := r.syncTasks(settings, allowElevated); err != nil {
		return err
	}
	return r.onUI(func() error {
		r.hosted.SetLanguage(settings.Language)
		return r.box.SetPaths(config.CollectedTrayIconPaths(settings))
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
		if err := r.onUI(func() error { return r.hosted.TryAdd(w) }); err == nil {
			return
		}
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
	if r.closing || r.ctx.Err() != nil {
		return job{State: "failed", Error: "模块正在退出"}
	}
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
	case "snapshot":
		snapshot := r.manager.Snapshot()
		r.jobsMu.Lock()
		snapshot.BackgroundError = r.lastError
		r.jobsMu.Unlock()
		result = snapshot
	case "settings.save":
		var args struct {
			Revision string          `json:"revision"`
			Settings config.Settings `json:"settings"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil {
			return fail(err)
		}
		result = r.startJob(func() (any, error) { return r.manager.Save(args.Revision, args.Settings) })
	case "launch":
		var args struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil {
			return fail(err)
		}
		settings := r.manager.Snapshot().Settings
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
			ID    string `json:"id"`
			Name  string `json:"name"`
			Image []byte `json:"image,omitempty"`
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
				icons = append(icons, iconView{icon.Identity(), icon.DisplayName(), image.Bytes()})
			}
			return nil
		}); err != nil {
			return fail(err)
		}
		result = icons
	case "icon.activate":
		var args struct {
			ID    string `json:"id"`
			Right bool   `json:"right"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil {
			return fail(err)
		}
		if err := r.onUI(func() error {
			for _, icon := range r.box.Icons() {
				if icon.Identity() == args.ID {
					action := traybox.ActionClick
					if args.Right {
						action = traybox.ActionRightClick
					}
					return traybox.Activate(icon, action)
				}
			}
			return traybox.ErrIconGone
		}); err != nil {
			return fail(err)
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
			r.hosted.RestoreAll()
			return nil
		}); err != nil {
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

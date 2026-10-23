//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"path/filepath"
	"time"
	"unsafe"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/internal/settings"
	"winone/modules/stroke"
)

// Gesture input cannot make an RPC to restart a dead worker. Supervise only
// this enabled, resident engine; maintenance and saved disablement win over
// recovery. No gesture/action is replayed when its listener is restarted.
func superviseStroke(ctx context.Context, queue *host.ConfigurationQueue, state *modulestate.Store, gate *modules.Gate, worker *modules.Lazy, directory string, configurationReady func() bool, changed <-chan struct{}) {
	var retryAt time.Time
	for ctx.Err() == nil {
		var exited <-chan struct{}
		var timer *time.Timer
		var retry <-chan time.Time
		current := state.Snapshot()
		record := current.Modules["stroke"]
		if record.Enabled && !record.PendingStop {
			if worker.Running() {
				exited = worker.ExitSignal()
			} else {
				delay := time.Until(retryAt)
				if delay <= 0 {
					delay = time.Second
					if release, admitted := queue.AcquireAction(); admitted {
						func() {
							defer release()
							if ctx.Err() != nil || !configurationReady() || worker.Status().State == "unavailable" {
								return
							}
							retryAt = time.Now().Add(10 * time.Second) // bound repeated immediate exits
							_ = recordHostEvent(directory, "stroke.listener.unavailable", errors.New(worker.Status().Detail))
							// The captured revision rejects a disable that raced the exit event.
							if err := gate.SetEnabled(current.Revision, true); err != nil {
								delay = 10 * time.Second
								_ = recordHostEvent(directory, "stroke.recovery.failed", err)
							} else {
								_ = recordHostEvent(directory, "stroke.listener.restored", nil)
							}
						}()
					}
				}
				if worker.Running() {
					exited = worker.ExitSignal()
				} else {
					timer = time.NewTimer(delay)
					retry = timer.C
				}
			}
		}
		// A healthy listener is observed through its process exit event. Disabled
		// modules have no timer and wake only for an explicit lifecycle change.
		select {
		case <-ctx.Done():
		case <-changed:
		case <-exited:
		case <-retry:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}
func strokeConfiguration(directory string, worker func() *modules.Lazy) configuration.ModuleAdapter {
	return configuration.ModuleAdapter{ID: "stroke", Load: func() (json.RawMessage, error) { return json.Marshal(stroke.Defaults()) }, Validate: func(raw json.RawMessage) error {
		var value stroke.Preferences
		if err := configjson.DecodeObject(raw, &value); err != nil {
			return err
		}
		if value.Revision != 0 {
			return errors.New("鼠标手势修订号由 Host 管理")
		}
		return value.Validate()
	}, Apply: func(raw json.RawMessage, revision uint64) error {
		var value stroke.Preferences
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		value.Revision = revision
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err = settings.WriteAtomic(filepath.Join(directory, "modules", "stroke", "managed-settings.json"), data); err != nil {
			return err
		}
		if worker != nil {
			if current := worker(); current != nil {
				response, called := current.HandleRunning(host.Request{Version: 1, ID: "stroke-configuration", Method: "settings.reload"})
				if called && response.Error != nil {
					return errors.New(response.Error.Message)
				}
			}
		}
		return nil
	}}
}

// Preference reads and target inspection never activate the global mouse hook.
func strokeConfigurationHandler(store *configuration.HostStore, next func(host.Request) host.Response) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "stroke", Message: err.Error()}}
		}
		result := func(value any) host.Response {
			data, err := json.Marshal(value)
			if err != nil {
				return fail(err)
			}
			return host.Response{ID: request.ID, Result: data}
		}
		if request.Method == "inspect.target" {
			target, err := strokeTargetAtCursor()
			if err != nil {
				return fail(err)
			}
			return result(target)
		}
		if request.Method == "windows.list" {
			return result(strokeTargetWindows())
		}
		if request.Method != "settings.get" && request.Method != "settings.save" {
			return next(request)
		}
		raw, revision, err := store.Module("stroke")
		if err != nil {
			return fail(err)
		}
		if request.Method == "settings.save" {
			var value stroke.Preferences
			if err = configjson.DecodeObject(request.Params, &value); err != nil {
				return fail(err)
			}
			expected := value.Revision
			value.Revision = 0
			if err = value.Validate(); err != nil {
				return fail(err)
			}
			raw, err = json.Marshal(value)
			if err != nil {
				return fail(err)
			}
			raw, revision, err = store.SaveModule("stroke", expected, raw)
			if err != nil {
				return fail(err)
			}
		}
		var value stroke.Preferences
		if err = json.Unmarshal(raw, &value); err != nil {
			return fail(err)
		}
		value.Revision = revision
		return result(value)
	}
}

func startStroke(worker *modules.Lazy) error {
	worker.Resume()
	response := worker.Handle(host.Request{Version: 1, ID: "stroke-activate", Method: "activate"})
	if response.Error != nil {
		return errors.Join(errors.New(response.Error.Message), worker.Close())
	}
	var state struct {
		Running bool `json:"running"`
	}
	if err := json.Unmarshal(response.Result, &state); err != nil {
		return err
	}
	if !state.Running {
		return errors.New("鼠标手势引擎未确认启用")
	}
	return nil
}

// Report execution failures through WinOne's module status without starting an
// idle worker merely because the settings or dashboard asks for status.
func strokeStatus(worker *modules.Lazy) host.Module {
	status := worker.Status()
	response, called := worker.HandleRunning(host.Request{Version: 1, ID: "stroke-status", Method: "snapshot"})
	if !called {
		// Unlike an on-demand tool, an enabled gesture engine must keep its
		// listener alive. Gate.Status handles disabled modules before here.
		if status.State == "available" {
			status.State = "faulted"
			status.Detail = "鼠标手势监听未运行，请重新启用模块"
		}
		return status
	}
	if response.Error != nil {
		status.State = "faulted"
		status.Detail = response.Error.Message
		return status
	}
	var runtime struct {
		Diagnostic string `json:"diagnostic"`
		Running    bool   `json:"running"`
	}
	if err := json.Unmarshal(response.Result, &runtime); err != nil {
		status.State = "faulted"
		status.Detail = "鼠标手势运行状态无效"
		return status
	}
	if runtime.Diagnostic != "" {
		status.State = "faulted"
		status.Detail = runtime.Diagnostic
	} else if !runtime.Running {
		status.State = "faulted"
		status.Detail = "鼠标手势监听未启用，请重新启用模块"
	}
	return status
}

type strokeTarget struct {
	Window      uint64 `json:"window"`
	ProcessID   uint32 `json:"processId"`
	ProcessPath string `json:"processPath"`
	WindowClass string `json:"windowClass"`
	Title       string `json:"title"`
}

var strokeUser32 = windows.NewLazySystemDLL("user32.dll")

func strokeWindowTarget(window uintptr) (strokeTarget, error) {
	var pid uint32
	strokeUser32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return strokeTarget{}, errors.New("无法识别目标窗口")
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return strokeTarget{}, errors.New("无法读取目标程序路径")
	}
	defer windows.CloseHandle(process)
	path := make([]uint16, 32768)
	size := uint32(len(path))
	if err = windows.QueryFullProcessImageName(process, 0, &path[0], &size); err != nil {
		return strokeTarget{}, err
	}
	class, title := make([]uint16, 256), make([]uint16, 512)
	strokeUser32.NewProc("GetClassNameW").Call(window, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	strokeUser32.NewProc("GetWindowTextW").Call(window, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
	return strokeTarget{Window: uint64(window), ProcessID: pid, ProcessPath: windows.UTF16ToString(path[:size]), WindowClass: windows.UTF16ToString(class), Title: windows.UTF16ToString(title)}, nil
}
func strokeTargetAtCursor() (strokeTarget, error) {
	var point struct{ X, Y int32 }
	ok, _, _ := strokeUser32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point)))
	if ok == 0 {
		return strokeTarget{}, errors.New("无法读取鼠标位置")
	}
	packed := uint64(uint32(point.X)) | uint64(uint32(point.Y))<<32
	window, _, _ := strokeUser32.NewProc("WindowFromPoint").Call(uintptr(packed))
	root, _, _ := strokeUser32.NewProc("GetAncestor").Call(window, 2)
	return strokeWindowTarget(root)
}
func strokeTargetWindows() []strokeTarget {
	result := []strokeTarget{}
	callback := windows.NewCallback(func(window, unused uintptr) uintptr {
		visible, _, _ := strokeUser32.NewProc("IsWindowVisible").Call(window)
		if visible != 0 && len(result) < 256 {
			if target, err := strokeWindowTarget(window); err == nil && target.Title != "" {
				result = append(result, target)
			}
		}
		return 1
	})
	strokeUser32.NewProc("EnumWindows").Call(callback, 0)
	return result
}

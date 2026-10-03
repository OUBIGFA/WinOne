//go:build windows

// Package worker exposes a small local command surface over the PowerToys adapter.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
	"winone/internal/host"
	"winone/internal/transport"
	"winone/modules/always-on-top/engine"
)

type Runtime struct {
	mu       sync.Mutex
	native   *engine.Native
	store    *engine.Store
	cancel   context.CancelFunc
	stopping bool
}
type Snapshot struct {
	Settings             engine.Settings `json:"settings"`
	Windows              []engine.Window `json:"windows"`
	OwnedWindows         []engine.Window `json:"ownedWindows"`
	Administrator        bool            `json:"administrator"`
	ProcessID            uint32          `json:"processId"`
	ConfigurationManaged bool            `json:"configurationManaged"`
}

func Run(parent context.Context, pipe, directory string) error {
	if !filepath.IsAbs(directory) {
		return errors.New("数据目录必须为绝对路径")
	}
	// Claim the pipe before creating native hooks or recovering existing pins.
	listener, err := transport.Listen(pipe)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	store, err := engine.OpenSettings(filepath.Join(directory, "settings.json"))
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	native, err := engine.OpenNative(filepath.Join(filepath.Dir(exe), "WinOne.AlwaysOnTop.Native.dll"), filepath.Join(directory, "native"))
	if err != nil {
		return err
	}
	defer native.Close()
	if err = native.Apply(store.Snapshot()); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	r := &Runtime{native: native, store: store, cancel: cancel}
	return transport.Serve(ctx, listener, r)
}

// Handle serializes native mutations and rejects stale window identities in the DLL.
func (r *Runtime) Handle(request host.Request) host.Response {
	response := host.Response{ID: request.ID}
	fail := func(err error) host.Response {
		response.Error = &host.RPCError{Code: "always_on_top", Message: err.Error()}
		return response
	}
	if request.Version != 1 || request.ID == "" {
		return fail(errors.New("无效的模块请求"))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		return fail(errors.New("置顶模块正在退出"))
	}
	var result any
	switch request.Method {
	case "snapshot":
		result = Snapshot{Settings: r.store.Snapshot(), Windows: r.native.Windows(), OwnedWindows: r.native.OwnedWindows(), Administrator: windows.GetCurrentProcessToken().IsElevated(), ProcessID: uint32(os.Getpid()), ConfigurationManaged: r.store.IsManaged()}
	case "settings.reload":
		value, err := r.store.ReloadManaged(r.native.Apply)
		if err != nil {
			return fail(err)
		}
		result = value
	case "settings.save":
		var next engine.Settings
		if err := json.Unmarshal(request.Params, &next); err != nil {
			return fail(err)
		}
		var legacy struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.Unmarshal(request.Params, &legacy); err != nil {
			return fail(err)
		}
		if legacy.Enabled != nil && !*legacy.Enabled {
			return fail(errors.New("请通过模块管理关闭窗口置顶"))
		}
		before := r.store.Snapshot()
		saved, err := r.store.Save(next)
		if err != nil {
			return fail(err)
		}
		if err = r.native.Apply(saved); err != nil {
			before.Revision = saved.Revision
			_, rollback := r.store.Save(before)
			return fail(errors.Join(err, rollback, r.native.Apply(before)))
		}
		result = saved
	case "window.set":
		var args struct {
			Window engine.Window `json:"window"`
			Pinned bool          `json:"pinned"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil {
			return fail(err)
		}
		if err := r.native.Pin(args.Window, args.Pinned); err != nil {
			return fail(err)
		}
		result = map[string]bool{"applied": true}
	case "window.toggle":
		var target engine.Window
		if err := json.Unmarshal(request.Params, &target); err != nil {
			return fail(err)
		}
		h, err := strconv.ParseUint(target.Handle, 10, 64)
		if err != nil {
			return fail(err)
		}
		current, err := r.native.Inspect(uintptr(h))
		if err != nil {
			return fail(err)
		}
		if current.PID != target.PID || current.Created != target.Created {
			return fail(errors.New("窗口已关闭或发生变化"))
		}
		if err = r.native.Pin(target, !current.Pinned); err != nil {
			return fail(err)
		}
		result = map[string]bool{"applied": true}
	case "toggle":
		window, err := r.native.Foreground()
		if err != nil {
			return fail(err)
		}
		if err = r.native.Pin(window, !window.Pinned); err != nil {
			return fail(err)
		}
		result = map[string]bool{"applied": true}
	case "clear":
		if err := r.native.Clear(); err != nil {
			return fail(err)
		}
		result = map[string]bool{"cleared": true}
	case "shutdown":
		if err := r.native.Close(); err != nil {
			return fail(err)
		}
		r.stopping = true
		result = map[string]bool{"stopping": true}
		go func() { time.Sleep(100 * time.Millisecond); r.cancel() }()
	default:
		return fail(fmt.Errorf("不支持的置顶操作"))
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fail(err)
	}
	response.Result = data
	return response
}

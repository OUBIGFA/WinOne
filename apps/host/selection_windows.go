//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"winone/internal/host"
	"winone/internal/selection"
)

type selectionRuntime struct {
	mu                sync.Mutex
	ctx               context.Context
	reader, directory string
	results           *resultWindows
	preferences       selection.Preferences
	watcher           *selection.Watcher
	cancel            context.CancelFunc
	moduleContext     context.Context
	done              chan struct{}
	lastError         string
	actionGuard       func() (func(), bool)
	suspended         func() bool
}

func (r *selectionRuntime) configure(value selection.Preferences) {
	r.mu.Lock()
	value.ExcludedApps = append([]string{}, value.ExcludedApps...)
	value.Actions = value.EffectiveActions()
	value.ToolbarVersion = 1
	r.preferences = value
	r.mu.Unlock()
}
func (r *selectionRuntime) status() host.Module {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, detail := "ready", ""
	if r.lastError != "" {
		state, detail = "faulted", r.lastError
	}
	return host.Module{ID: "selection", Name: "划词助手", State: state, Detail: detail}
}
func (r *selectionRuntime) start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.watcher != nil || r.results == nil {
		return nil
	}
	if _, err := os.Stat(r.reader); err != nil {
		r.lastError = "划词读取组件不可用，请修复安装"
		return errors.New(r.lastError)
	}
	watcher, err := selection.StartWatcher()
	if err != nil {
		r.lastError = err.Error()
		return err
	}
	ctx, cancel := context.WithCancel(r.ctx)
	r.watcher, r.cancel, r.done = watcher, cancel, make(chan struct{})
	r.moduleContext = ctx
	r.lastError = ""
	r.results.allowSelection(true)
	go r.run(ctx, watcher, r.done)
	return nil
}
func (r *selectionRuntime) stop() error {
	r.mu.Lock()
	watcher, cancel, done, results := r.watcher, r.cancel, r.done, r.results
	if results != nil {
		results.allowSelection(false)
	}
	if cancel != nil {
		cancel()
	}
	r.mu.Unlock()
	if results != nil {
		if err := results.closeSelection(); err != nil {
			return err
		}
	}
	if watcher != nil {
		if err := watcher.Close(); err != nil {
			return err
		}
		<-done
	}
	r.mu.Lock()
	r.watcher, r.cancel, r.done = nil, nil, nil
	r.mu.Unlock()
	return nil
}
func (r *selectionRuntime) record(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastError = ""
	if err != nil && !errors.Is(err, context.Canceled) {
		r.lastError = err.Error()
	}
}
func (r *selectionRuntime) admitted(event selection.Event) bool {
	if event.ProcessID == 0 || event.ProcessID == uint32(os.Getpid()) {
		return false
	}
	r.mu.Lock()
	prefs := r.preferences
	suspended := r.suspended
	r.mu.Unlock()
	if suspended != nil && suspended() {
		return false
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, event.ProcessID)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if windows.QueryFullProcessImageName(process, 0, &buffer[0], &size) != nil {
		return false
	}
	executable := windows.UTF16ToString(buffer[:size])
	// Settings and result text are valid sources. Result toolbars return an
	// empty selection; worker processes remain excluded.
	if !selectionSourceAllowed(r.directory, executable) {
		return false
	}
	return !prefs.Excludes(filepath.Base(executable))
}

func selectionSourceAllowed(directory, executable string) bool {
	if real, err := filepath.EvalSymlinks(directory); err == nil {
		directory = real
	}
	if real, err := filepath.EvalSymlinks(executable); err == nil {
		executable = real
	}
	relative, err := filepath.Rel(directory, executable)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return true
	}
	return strings.EqualFold(relative, "WinOne.exe") || strings.EqualFold(relative, filepath.Join("shared", "ui", "WinOne.Results.exe"))
}
func (r *selectionRuntime) read(ctx context.Context, event selection.Event, current func() bool) error {
	if !r.admitted(event) {
		return nil
	}
	value, handled, err := r.results.readSelection(ctx, event)
	if !handled {
		value, err = selection.ReadSelection(ctx, r.reader, event)
	}
	if err != nil {
		return err
	}
	if !current() || ctx.Err() != nil {
		return context.Canceled
	}
	if value.Status != "selected" {
		if event.Kind == "manual" {
			return errors.New("未能读取选中文字，请在支持的文本区域重新选择")
		}
		return nil
	}
	anchor := event.Point
	if event.Keyboard && len(value.Rectangles) > 0 {
		last := value.Rectangles[len(value.Rectangles)-1]
		anchor = [2]int32{int32(last[0]), int32(last[1] + last[3])}
	}
	window, pid := selection.Foreground()
	if window != event.Window || pid != event.ProcessID {
		return context.Canceled
	}
	if r.actionGuard != nil {
		release, allowed := r.actionGuard()
		if !allowed {
			return context.Canceled
		}
		defer release()
	}
	if err := r.results.dismissSelection([2]int32{}, true, 0); err != nil {
		return err
	}
	r.mu.Lock()
	actions := r.preferences.EffectiveActions()
	r.mu.Unlock()
	return r.results.openSelection(value.Text, anchor, actions)
}
func (r *selectionRuntime) run(ctx context.Context, watcher *selection.Watcher, done chan struct{}) {
	defer close(done)
	var cancel context.CancelFunc
	var reads sync.WaitGroup
	defer reads.Wait()
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-watcher.Events():
			if cancel != nil {
				cancel()
			}
			if err := r.results.dismissSelection(event.Point, event.Keyboard || event.Kind == "focus", event.Window); err != nil {
				r.record(err)
			}
			r.mu.Lock()
			prefs := r.preferences
			r.mu.Unlock()
			if event.Kind != "selection" || !prefs.Automatic || event.Keyboard && !prefs.Keyboard {
				continue
			}
			request, requestCancel := context.WithCancel(ctx)
			cancel = requestCancel
			reads.Add(1)
			go func() {
				defer reads.Done()
				defer requestCancel()
				err := r.read(request, event, func() bool { return watcher.Current(event.Sequence) })
				if err != nil && request.Err() == nil {
					r.record(err)
				}
			}()
		}
	}
}
func (r *selectionRuntime) handle(request host.Request) host.Response {
	if request.Method == "snapshot" {
		data, _ := json.Marshal(r.status())
		return host.Response{ID: request.ID, Result: data}
	}
	if request.Method != "show" {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "selection", Message: "不支持的划词操作"}}
	}
	window, pid := selection.Foreground()
	r.mu.Lock()
	parent := r.moduleContext
	r.mu.Unlock()
	if parent == nil {
		parent = r.ctx
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	err := r.read(ctx, selection.Event{Kind: "manual", Window: window, ProcessID: pid, Keyboard: true}, func() bool { return true })
	r.record(err)
	if err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "selection", Message: err.Error()}}
	}
	return host.Response{ID: request.ID, Result: json.RawMessage(`{"shown":true}`)}
}

//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"winone/internal/diagnostics"
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
	lastError         string // startup / hook failure, not a one-shot read
	operationError    string
	operation         uint64 // only the newest admitted operation may publish
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
	return host.Module{ID: "selection", Name: "划词助手", State: state, Detail: detail, Warning: r.operationError}
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
	r.lastError, r.operationError = "", ""
	r.operation++
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
	r.operation++
	r.operationError = ""
	r.mu.Unlock()
	return nil
}

// beginOperation / finishOperation separate a transient request result from
// lifecycle health. Cancelled/obsolete reads cannot overwrite a newer success;
// a successful automatic read clears the same warning as a manual one.
func (r *selectionRuntime) beginOperation() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operation++
	return r.operation
}
func (r *selectionRuntime) finishOperation(ticket uint64, current func() bool, err error) {
	r.mu.Lock()
	if ticket != r.operation || !current() || errors.Is(err, context.Canceled) {
		r.mu.Unlock()
		return
	}
	previous := r.operationError
	r.operationError = ""
	if err != nil {
		r.operationError = "上次划词操作未完成，功能仍可重试：" + err.Error()
	}
	changed := previous != r.operationError
	results := r.results
	r.mu.Unlock()
	// Record only a changed diagnostic, never selection text, credentials or
	// request arguments. A cancelled gesture is not a fault/recovery event.
	if changed && results != nil && results.logDirectory != "" {
		event := diagnostics.Event{Source: "selection", Event: "operation-recovered"}
		if err != nil {
			event.Event, event.Error = "operation-failed", err.Error()
		}
		if logErr := diagnostics.Record(results.logDirectory, event); logErr != nil {
			fmt.Fprintln(os.Stderr, "Selection diagnostics:", logErr)
		}
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
func (r *selectionRuntime) read(ctx context.Context, event selection.Event, current func() bool) (failure error) {
	if !r.admitted(event) {
		return nil
	}
	ticket := r.beginOperation()
	defer func() { r.finishOperation(ticket, current, failure) }()
	value, handled, err := r.results.readSelection(ctx, event)
	if !handled {
		value, err = selection.ReadSelection(ctx, r.reader, event)
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("读取选中文字超时，请在目标程序响应后重新选择（%w）", err)
		}
		return fmt.Errorf("读取选中文字失败：%w", err)
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
		return fmt.Errorf("关闭上次划词工具栏失败：%w", err)
	}
	r.mu.Lock()
	actions := r.preferences.EffectiveActions()
	r.mu.Unlock()
	if err := r.results.openSelection(value.Text, anchor, actions); err != nil {
		return fmt.Errorf("显示划词工具栏失败：%w", err)
	}
	return nil
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
			ticket := r.beginOperation()
			if err := r.results.dismissSelection(event.Point, event.Keyboard || event.Kind == "focus", event.Window); err != nil {
				r.finishOperation(ticket, func() bool { return watcher.Current(event.Sequence) }, fmt.Errorf("收起划词工具栏失败：%w", err))
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
				_ = r.read(request, event, func() bool { return watcher.Current(event.Sequence) })
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
	if err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "selection", Message: err.Error()}}
	}
	return host.Response{ID: request.ID, Result: json.RawMessage(`{"shown":true}`)}
}

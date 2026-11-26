//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"sort"
	"strconv"
	"sync"
	"time"
	"unsafe"
	"winone/internal/capturehandoff"
	"winone/internal/diagnostics"
	"winone/internal/host"
	"winone/internal/hotkeys"
	"winone/internal/modulestate"
	"winone/internal/settings"
)

type shortcutRegistrar interface {
	Update([]hotkeys.Binding) ([]hotkeys.Status, error)
	Events() <-chan string
	Close() error
}
type shortcutRuntime struct {
	mu                  sync.Mutex
	registrar           shortcutRegistrar
	bindings            []hotkeys.Binding
	recording           *shortcutRecording
	statuses            []hotkeys.Status
	lastError           string
	actionErrors        map[string]string
	moduleState         *modulestate.Store
	openDraft           func(string) error
	openWindow          func() error
	defaultOpenConflict bool
	language            func() string
	logDirectory        string // Product log for action timing and failures; empty disables it.
}

func (r *shortcutRuntime) apply(s settings.Settings) {
	_ = r.applyConfiguration(s)
}

// Registration conflicts remain visible per action; malformed registrations are
// application failures and must prevent a configuration transaction committing.
func (r *shortcutRuntime) applyConfiguration(s settings.Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Resolve defaults before masking disabled modules, so disabling capture
	// cannot silently claim a shortcut reserved by its saved preferences.
	r.defaultOpenConflict = s.OpenShortcut == nil && s.OpenBinding() == ""
	s = s.ResolvedShortcuts()
	if r.moduleState != nil {
		state := r.moduleState.Snapshot()
		if record := state.Modules["capture"]; !record.Enabled || record.PendingStop {
			s.CaptureShortcut = ""
			empty := ""
			s.OCRShortcut = &empty
			s.TranslateShortcut = &empty
		}
		if record := state.Modules["always-on-top"]; !record.Enabled || record.PendingStop {
			s.PinShortcut = ""
		}
		if record := state.Modules["quick-assistant"]; !record.Enabled || record.PendingStop {
			empty := ""
			s.QuickAssistantShortcut = &empty
		}
		if record := state.Modules["selection"]; !record.Enabled || record.PendingStop {
			empty := ""
			s.SelectionShortcut = &empty
		}
	}
	bindings := []hotkeys.Binding{{ID: "capture", Shortcut: s.CaptureShortcut}, {ID: "pin", Shortcut: s.PinShortcut}, {ID: "ocr", Shortcut: s.OCRBinding()}, {ID: "translate", Shortcut: s.TranslateBinding()}, {ID: "selection", Shortcut: s.SelectionBinding()}, {ID: "quick-assistant", Shortcut: s.QuickAssistantBinding()}, {ID: "open-winone", Shortcut: s.OpenBinding()}}
	if err := hotkeys.ValidateBindings(bindings); err != nil {
		return err
	}
	r.bindings = bindings
	if r.recording != nil {
		return nil
	}
	statuses, err := r.registrar.Update(bindings)
	if err != nil {
		r.lastError = err.Error()
		return err
	}
	r.lastError = ""
	r.actionErrors = nil
	r.statuses = statuses
	return nil
}
func (r *shortcutRuntime) handle(req host.Request) host.Response {
	if req.Method == "record.start" || req.Method == "record.stop" {
		return r.handleRecording(req)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if req.Method != "snapshot" {
		return host.Response{ID: req.ID, Error: &host.RPCError{Code: "invalid", Message: "不支持的快捷键操作"}}
	}
	statuses := append([]hotkeys.Status(nil), r.statuses...)
	// Preserve older bindings and explain why their new default was not claimed.
	if r.defaultOpenConflict {
		for i := range statuses {
			if statuses[i].ID == "open-winone" {
				statuses[i].Error = "默认 Alt+A 已被其他 WinOne 操作使用，请设置其他快捷键"
			}
		}
	}
	data, _ := json.Marshal(struct {
		Bindings     []hotkeys.Status  `json:"bindings"`
		LastError    string            `json:"lastError"`
		ActionErrors map[string]string `json:"actionErrors"`
	}{statuses, r.lastError, r.actionErrors})
	return host.Response{ID: req.ID, Result: data}
}
func (r *shortcutRuntime) report(action string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastError = ""
	if r.actionErrors == nil {
		r.actionErrors = map[string]string{}
	}
	delete(r.actionErrors, action)
	if err != nil {
		r.lastError = err.Error()
		r.actionErrors[action] = err.Error()
	}
}

// Capture identity before a cold worker startup can let foreground focus move.
func foregroundIdentity() (json.RawMessage, error) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	h, _, _ := user32.NewProc("GetForegroundWindow").Call()
	if h == 0 {
		return nil, fmt.Errorf("没有前台窗口")
	}
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(h, uintptr(unsafe.Pointer(&pid)))
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(p)
	var created, exit, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(p, &created, &exit, &kernel, &user); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"handle": strconv.FormatUint(uint64(h), 10), "pid": pid, "created": strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10)})
}

// capturing reports whether an action uses the capture worker's single job.
func capturing(id string) bool { return id != "pin" && id != "selection" && id != "quick-assistant" }

func (r *shortcutRuntime) run(ctx context.Context, service *host.Service) {
	// Keep one capture-family operation in flight. A press while it runs first
	// focuses its visible session; when there is none (silent OCR, closing),
	// the latest press is kept and runs as soon as the operation ends.
	type outcome struct {
		err     error
		timings map[string]float64
	}
	type activation struct {
		active bool
		err    error
	}
	busy, activating, opening, pinning := false, false, false, false
	activeAction := ""
	var activePressed, activeStarted time.Time
	queued := ""
	var queuedAt time.Time
	finished := make(chan outcome, 1)
	activationFinished := make(chan activation, 1)
	openFinished := make(chan error, 1)
	pinFinished := make(chan error, 1)
	start := func(id string, pressed time.Time) {
		locale := "zh-CN"
		if r.language != nil {
			locale = r.language()
		}
		mode := "window"
		if id == "ocr" || id == "translate" {
			mode = id
		}
		captureParams, _ := json.Marshal(map[string]string{"mode": mode, "language": locale})
		req := host.Request{Version: 1, ID: fmt.Sprintf("shortcut-%d", time.Now().UnixNano()), Method: "capture.capture.unified", Params: captureParams}
		if !capturing(id) {
			req.Method = id + ".show"
			req.Params = json.RawMessage(`{}`)
		}
		busy, activeAction, activePressed, activeStarted = true, id, pressed, time.Now()
		go func() {
			// A new shortcut captures the current screen. Recovery is an explicit
			// draft.resume action; restoring old pixels here hides the current app.
			response := service.Handle(req)
			if response.Error != nil {
				finished <- outcome{err: fmt.Errorf("%s", response.Error.Message)}
				return
			}
			if capturing(id) {
				timings, err := waitCaptureJob(ctx, service, response, r.openDraft)
				finished <- outcome{err: err, timings: timings}
				return
			}
			finished <- outcome{}
		}()
	}
	runQueued := func() {
		if !busy && !activating && queued != "" {
			id, pressed := queued, queuedAt
			queued = ""
			start(id, pressed)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-openFinished:
			opening = false
			r.report("open-winone", err)
			r.logAction("open-winone", err)
		case err := <-pinFinished:
			pinning = false
			r.report("pin", err)
			r.logAction("pin", err)
		case result := <-finished:
			r.report(activeAction, result.err)
			r.logAction(activeAction, result.err, map[string]float64{
				"queued": float64(activeStarted.Sub(activePressed).Microseconds()) / 1000,
				"total":  float64(time.Since(activePressed).Microseconds()) / 1000,
			}, result.timings)
			busy = false
			runQueued()
		case result := <-activationFinished:
			activating = false
			if result.err != nil {
				r.report("capture", result.err)
			}
			// A focused session consumed the press; otherwise keep it queued.
			if result.err != nil || result.active {
				queued = ""
			}
			runQueued()
		case id, ok := <-r.registrar.Events():
			if !ok {
				return
			}
			pressed := time.Now()
			r.mu.Lock()
			recording := r.recording != nil
			r.mu.Unlock()
			if recording {
				continue
			}
			// Opening the application is independent of capture sessions and
			// module enablement. Coalesce repeated presses during a cold launch.
			if id == "open-winone" {
				if !opening {
					opening = true
					go func() {
						if r.openWindow == nil {
							openFinished <- fmt.Errorf("WinOne 窗口打开入口未配置")
							return
						}
						openFinished <- r.openWindow()
					}()
				}
				continue
			}
			// Pinning targets the foreground window at press time and takes
			// milliseconds; it never waits behind a capture.
			if id == "pin" {
				if !pinning {
					params, err := foregroundIdentity()
					if err != nil {
						r.report(id, err)
						r.logAction(id, err)
						continue
					}
					pinning = true
					go func() {
						response := service.Handle(host.Request{Version: 1, ID: fmt.Sprintf("shortcut-%d", time.Now().UnixNano()), Method: "always-on-top.window.toggle", Params: params})
						if response.Error != nil {
							pinFinished <- fmt.Errorf("%s", response.Error.Message)
							return
						}
						pinFinished <- nil
					}()
				}
				continue
			}
			if busy {
				queued, queuedAt = id, pressed
				if !activating {
					activating = true
					go func() {
						response := service.Handle(host.Request{Version: 1, ID: "shortcut-activate-capture", Method: "capture.capture.activate", Params: json.RawMessage(`{}`)})
						if response.Error != nil {
							activationFinished <- activation{err: fmt.Errorf("%s", response.Error.Message)}
							return
						}
						var result struct {
							Active bool `json:"active"`
						}
						err := json.Unmarshal(response.Result, &result)
						activationFinished <- activation{active: result.Active, err: err}
					}()
				}
				continue
			}
			start(id, pressed)
		}
	}
}

// logAction writes one bounded product-log line per shortcut action: its
// failure, or press-to-finish timing plus the capture worker's own stages.
// Cancellation is a normal outcome. Recognized text or images never appear.
// The fsync runs off the dispatch loop so the next press is not delayed.
func (r *shortcutRuntime) logAction(action string, failure error, timings ...map[string]float64) {
	if r.logDirectory == "" {
		return
	}
	if failure != nil {
		go func() { _ = recordHostEvent(r.logDirectory, "shortcut.failed", fmt.Errorf("%s: %w", action, failure)) }()
		return
	}
	merged := map[string]float64{}
	for _, values := range timings {
		for key, value := range values {
			merged[key] = value
		}
	}
	if len(merged) == 0 {
		return
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	message := "action=" + action
	for _, key := range keys {
		message += fmt.Sprintf(" %s=%.0fms", key, merged[key])
	}
	go func() {
		_ = diagnostics.Record(r.logDirectory, diagnostics.Event{Source: "host", Event: "shortcut.timing", Message: message})
	}()
}

// Capture creation returns a job; output/clipboard failures may arrive only at
// completion. The worker holds each jobs.get until the job changes, bounded by
// waitMs inside the 5 s pipe budget, so completion is seen at once instead of
// on a fixed 250 ms poll. Timings come from the worker's own stage clock.
func waitCaptureJob(ctx context.Context, service *host.Service, response host.Response, openDraft func(string) error) (map[string]float64, error) {
	var job struct {
		ID      string             `json:"id"`
		State   string             `json:"state"`
		Error   string             `json:"error"`
		Timings map[string]float64 `json:"timings"`
		Result  struct {
			DraftID string `json:"draftId"`
		} `json:"result"`
	}
	for {
		if response.Error != nil {
			return nil, fmt.Errorf("%s", response.Error.Message)
		}
		job.Timings = nil
		if err := json.Unmarshal(response.Result, &job); err != nil {
			return nil, err
		}
		switch job.State {
		case "failed":
			return job.Timings, fmt.Errorf("%s", job.Error)
		case "completed":
			if err := ctx.Err(); err != nil {
				return job.Timings, err
			}
			if job.Result.DraftID != "" {
				if err := capturehandoff.ValidateDraftID(job.Result.DraftID); err != nil {
					return job.Timings, err
				}
				if openDraft == nil {
					return job.Timings, fmt.Errorf("截图编辑窗口未配置")
				}
				return job.Timings, openDraft(job.Result.DraftID)
			}
			return job.Timings, nil
		case "cancelled":
			return job.Timings, nil
		case "running":
		default:
			return nil, fmt.Errorf("无法确认截图任务状态")
		}
		if job.ID == "" {
			return nil, fmt.Errorf("截图任务未返回标识")
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		params, _ := json.Marshal(map[string]any{"id": job.ID, "waitMs": 2000})
		requested := time.Now()
		response = service.Handle(host.Request{Version: 1, ID: "shortcut-capture-job", Method: "capture.jobs.get", Params: params})
		// A worker that answers "running" without waiting must not turn this
		// loop into a busy spin. Completion leaves the loop above without delay.
		if spacing := 25*time.Millisecond - time.Since(requested); spacing > 0 && response.Error == nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(spacing):
			}
		}
	}
}

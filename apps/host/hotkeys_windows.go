//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"strconv"
	"sync"
	"time"
	"unsafe"
	"winone/internal/capturehandoff"
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
	mu           sync.Mutex
	registrar    shortcutRegistrar
	bindings     []hotkeys.Binding
	recording    *shortcutRecording
	statuses     []hotkeys.Status
	lastError    string
	actionErrors map[string]string
	moduleState  *modulestate.Store
	openDraft    func(string) error
	language     func() string
}

func (r *shortcutRuntime) apply(s settings.Settings) {
	_ = r.applyConfiguration(s)
}

// Registration conflicts remain visible per action; malformed registrations are
// application failures and must prevent a configuration transaction committing.
func (r *shortcutRuntime) applyConfiguration(s settings.Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.moduleState != nil {
		state := r.moduleState.Snapshot()
		if record := state.Modules["capture"]; !record.Enabled || record.PendingStop {
			s.CaptureShortcut = ""
		}
		if record := state.Modules["always-on-top"]; !record.Enabled || record.PendingStop {
			s.PinShortcut = ""
		}
	}
	bindings := []hotkeys.Binding{{ID: "capture", Shortcut: s.CaptureShortcut}, {ID: "pin", Shortcut: s.PinShortcut}}
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
	data, _ := json.Marshal(struct {
		Bindings     []hotkeys.Status  `json:"bindings"`
		LastError    string            `json:"lastError"`
		ActionErrors map[string]string `json:"actionErrors"`
	}{r.statuses, r.lastError, r.actionErrors})
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
func (r *shortcutRuntime) run(ctx context.Context, service *host.Service) {
	// Keep one operation in flight. Repeated capture presses only activate that
	// session; they cannot queue a capture or toggle after it finishes.
	busy := false
	activeAction := ""
	finished := make(chan error, 1)
	activationFinished := make(chan error, 1)
	activating := false
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-finished:
			r.report(activeAction, err)
			busy = false
		case err := <-activationFinished:
			activating = false
			if err != nil {
				r.report("capture", err)
			}
		case id, ok := <-r.registrar.Events():
			if !ok {
				return
			}
			r.mu.Lock()
			recording := r.recording != nil
			r.mu.Unlock()
			if recording {
				continue
			}
			if busy {
				if id == "capture" && activeAction == "capture" && !activating {
					activating = true
					go func() {
						response := service.Handle(host.Request{Version: 1, ID: "shortcut-activate-capture", Method: "capture.capture.activate", Params: json.RawMessage(`{}`)})
						var err error
						if response.Error != nil {
							err = fmt.Errorf("%s", response.Error.Message)
						}
						activationFinished <- err
					}()
				}
				continue
			}
			locale := "zh-CN"
			if r.language != nil {
				locale = r.language()
			}
			captureParams, _ := json.Marshal(map[string]string{"mode": "window", "language": locale})
			req := host.Request{Version: 1, ID: fmt.Sprintf("shortcut-%d", time.Now().UnixNano()), Method: "capture.capture.unified", Params: captureParams}
			if id == "pin" {
				params, err := foregroundIdentity()
				if err != nil {
					r.report(id, err)
					continue
				}
				req.Method = "always-on-top.window.toggle"
				req.Params = params
			}
			busy = true
			activeAction = id
			go func() {
				if id == "capture" {
					resumed, err := resumeCaptureDraft(ctx, service, r.openDraft)
					if resumed || err != nil {
						finished <- err
						return
					}
				}
				response := service.Handle(req)
				if response.Error != nil {
					finished <- fmt.Errorf("%s", response.Error.Message)
					return
				}
				if id == "capture" {
					finished <- waitCaptureJob(ctx, service, response, r.openDraft)
					return
				}
				finished <- nil
			}()
		}
	}
}

func resumeCaptureDraft(ctx context.Context, service *host.Service, openDraft func(string) error) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	response := service.Handle(host.Request{Version: 1, ID: "shortcut-resume-draft", Method: "capture.draft.get", Params: json.RawMessage(`{}`)})
	if response.Error != nil {
		return false, fmt.Errorf("%s", response.Error.Message)
	}
	var result struct {
		Draft *struct {
			ID string `json:"id"`
		} `json:"draft"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return false, err
	}
	if result.Draft == nil {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := capturehandoff.ValidateDraftID(result.Draft.ID); err != nil {
		return false, err
	}
	if openDraft == nil {
		return false, fmt.Errorf("截图编辑窗口未配置")
	}
	return true, openDraft(result.Draft.ID)
}

// Capture creation returns a job; output/clipboard failures may arrive only at completion.
func waitCaptureJob(ctx context.Context, service *host.Service, response host.Response, openDraft func(string) error) error {
	var job struct {
		ID     string `json:"id"`
		State  string `json:"state"`
		Error  string `json:"error"`
		Result struct {
			DraftID string `json:"draftId"`
		} `json:"result"`
	}
	for {
		if response.Error != nil {
			return fmt.Errorf("%s", response.Error.Message)
		}
		if err := json.Unmarshal(response.Result, &job); err != nil {
			return err
		}
		switch job.State {
		case "failed":
			return fmt.Errorf("%s", job.Error)
		case "completed":
			if err := ctx.Err(); err != nil {
				return err
			}
			if job.Result.DraftID != "" {
				if err := capturehandoff.ValidateDraftID(job.Result.DraftID); err != nil {
					return err
				}
				if openDraft == nil {
					return fmt.Errorf("截图编辑窗口未配置")
				}
				return openDraft(job.Result.DraftID)
			}
			return nil
		case "cancelled":
			return nil
		case "running":
		default:
			return fmt.Errorf("无法确认截图任务状态")
		}
		if job.ID == "" {
			return fmt.Errorf("截图任务未返回标识")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		params, _ := json.Marshal(map[string]string{"id": job.ID})
		response = service.Handle(host.Request{Version: 1, ID: "shortcut-capture-job", Method: "capture.jobs.get", Params: params})
	}
}

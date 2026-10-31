//go:build windows

package desktop

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"winone/desktopmygo/host"
	"winone/desktopmygo/i18n"
	"winone/internal/protocol"
)

// PermissionAction is what the administrator helper does for the
// permission settings: remember whether WinOne starts as administrator
// (Enabled), or restart this window as administrator now (Restart).
type PermissionAction struct {
	Restart bool
	Enabled bool
}

// PermissionResult is the helper's outcome after this window checked it:
// the settings it saved or read, or, for a restart, that the window must
// close so the administrator instance takes over (Closing, with the
// confirmed handoff Token).
type PermissionResult struct {
	Closing  bool            `json:"closing"`
	Token    string          `json:"token,omitempty"`
	Settings json.RawMessage `json:"settings,omitempty"`
}

// permissionLimit bounds the whole operation, consent prompt included.
const permissionLimit = 300 * time.Second

// RunPermission performs action through WinOne.Host.exe --desktop-control,
// the Host's short-lived consent helper beside this program. The helper
// asks this window to relay its Host requests, so the restart's owner is
// the real settings process, and asks guard whether the window holds no
// unsaved edit before it changes anything. Only the requests the action
// needs are relayed. It blocks: call it off the UI thread.
//
// Port of apps/desktop-rust/src/permissions.rs.
func RunPermission(client *host.Client, directory string, action PermissionAction, guard func() bool) (PermissionResult, error) {
	executable, err := os.Executable()
	if err != nil {
		return PermissionResult{}, err
	}
	command := exec.Command(filepath.Join(filepath.Dir(executable), "WinOne.Host.exe"), "--desktop-control")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	operation := "preference"
	if action.Restart {
		operation = "restart"
	}
	request := map[string]any{"version": 1, "operation": operation, "pipe": client.Pipe(), "dataDirectory": directory, "ownerPID": os.Getpid()}
	if !action.Restart {
		request["enabled"] = action.Enabled
	}
	data, err := json.Marshal(request)
	if err != nil {
		return PermissionResult{}, err
	}
	call := func(method string, params json.RawMessage) (json.RawMessage, error) {
		var value any
		if len(params) > 0 && string(params) != "null" {
			value = params
		}
		// The settings document can come back as a configuration job.
		if method == "settings.save" {
			return client.CallCompleting(method, value)
		}
		return client.Call(method, value)
	}
	return runHelper(command, data, action, call, guard, client.Cancelled, permissionLimit)
}

func permissionError(code, key string, args ...any) error {
	return &host.Error{Code: code, Message: i18n.T(key, args...)}
}

func protocolError() error { return permissionError("protocol", "permission.protocol") }

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procCancelSynchronousIo = kernel32.NewProc("CancelSynchronousIo")
)

// runHelper starts the helper, exchanges its messages on a thread of its
// own, and ends it on cancellation or after limit. A read blocked on the
// helper's pipe is interrupted even when a descendant kept the pipe open.
func runHelper(command *exec.Cmd, request []byte, action PermissionAction, call func(string, json.RawMessage) (json.RawMessage, error), guard func() bool, cancelled func() bool, limit time.Duration) (PermissionResult, error) {
	if cancelled() {
		return PermissionResult{}, permissionError("cancelled", "permission.cancelled")
	}
	input, err := command.StdinPipe()
	if err != nil {
		return PermissionResult{}, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return PermissionResult{}, err
	}
	if err := command.Start(); err != nil {
		return PermissionResult{}, err
	}
	type outcome struct {
		raw json.RawMessage
		err error
	}
	done := make(chan outcome, 1)
	thread := make(chan windows.Handle, 1)
	go func() {
		// Locked, so CancelSynchronousIo reaches the blocked read.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		handle, _ := windows.OpenThread(windows.THREAD_TERMINATE, false, windows.GetCurrentThreadId())
		thread <- handle
		raw, err := func() (json.RawMessage, error) {
			if err := protocol.WriteFrame(input, request); err != nil {
				return nil, err
			}
			return exchange(output, input, action, call, guard, cancelled)
		}()
		done <- outcome{raw, err}
	}()
	ioThread := <-thread
	if ioThread != 0 {
		defer windows.CloseHandle(ioThread)
	}
	deadline := time.Now().Add(limit)
	var stopped error
	var result outcome
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
wait:
	for {
		select {
		case result = <-done:
			break wait
		case <-ticker.C:
			if stopped == nil && (cancelled() || !time.Now().Before(deadline)) {
				stopped = permissionError("timeout", "permission.timeout")
				if cancelled() {
					stopped = permissionError("cancelled", "permission.cancelled")
				}
				if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					stopped = permissionError("permission", "permission.killFailed", "error", err.Error())
				}
			}
			if stopped != nil && ioThread != 0 {
				procCancelSynchronousIo.Call(uintptr(ioThread))
			}
		}
	}
	_ = input.Close()
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	var exit error
	select {
	case exit = <-waited:
	case <-time.After(3 * time.Second):
		_ = command.Process.Kill()
		<-waited
		return PermissionResult{}, permissionError("permission", "permission.helperStuck")
	}
	if stopped != nil {
		return PermissionResult{}, stopped
	}
	if result.err != nil {
		return PermissionResult{}, result.err
	}
	if exit != nil {
		return PermissionResult{}, permissionError("permission", "permission.helperExit", "status", exit.Error())
	}
	var value PermissionResult
	if err := json.Unmarshal(result.raw, &value); err != nil {
		return PermissionResult{}, protocolError()
	}
	return value, nil
}

// permissionMessage is one message of the helper.
type permissionMessage struct {
	Version  int             `json:"version"`
	Sequence uint64          `json:"sequence"`
	Kind     string          `json:"kind"`
	Method   string          `json:"method"`
	Params   json.RawMessage `json:"params"`
	Result   json.RawMessage `json:"result"`
	Error    json.RawMessage `json:"error"`
}

// exchange answers the helper's guard questions and Host requests until it
// reports its result, which is accepted only when it matches what this
// window relayed: a preference saved exactly as returned, a restart whose
// handoff the Host committed and acknowledged with the same token, or an
// administrator window that needs no restart.
func exchange(reader io.Reader, writer io.Writer, action PermissionAction, call func(string, json.RawMessage) (json.RawMessage, error), guard func() bool, cancelled func() bool) (json.RawMessage, error) {
	var saved json.RawMessage
	saveAttempted, committed := false, false
	var snapshot json.RawMessage
	prepared, acknowledged := "", ""
	for sequence := uint64(1); sequence <= 8192; sequence++ {
		if cancelled() {
			return nil, permissionError("cancelled", "permission.cancelled")
		}
		frame, err := protocol.ReadFrame(reader)
		if err != nil {
			return nil, err
		}
		var message permissionMessage
		if json.Unmarshal(frame, &message) != nil || message.Version != 1 || message.Sequence != sequence {
			return nil, protocolError()
		}
		var result json.RawMessage
		var failure error
		switch message.Kind {
		case "guard":
			result, _ = json.Marshal(guard())
		case "call":
			method := message.Method
			allowed := method == "snapshot"
			if action.Restart {
				switch method {
				case "application.restart.prepare", "application.restart.status", "application.restart.commit", "application.restart.cancel":
					allowed = true
				}
			} else if method == "settings.save" && !saveAttempted {
				allowed = true
			}
			if method == "" || !allowed {
				return nil, protocolError()
			}
			if method == "settings.save" {
				saveAttempted = true
			}
			if method == "application.restart.prepare" && prepared != "" || method == "application.restart.commit" && committed {
				return nil, protocolError()
			}
			result, failure = call(method, message.Params)
			if failure == nil {
				var job struct {
					Token              json.RawMessage `json:"token"`
					State              string          `json:"state"`
					HelperPID          uint64          `json:"helperPid"`
					HelperAcknowledged bool            `json:"helperAcknowledged"`
				}
				_ = json.Unmarshal(result, &job)
				switch method {
				case "snapshot":
					snapshot = result
				case "settings.save":
					saved = result
				case "application.restart.prepare":
					prepared = token(job.Token)
				}
				if method == "application.restart.commit" && job.State == "committed" {
					committed = true
				}
				if (method == "application.restart.commit" || method == "application.restart.status") && job.State == "committed" &&
					job.HelperAcknowledged && job.HelperPID > 0 && prepared != "" && token(job.Token) == prepared {
					acknowledged = prepared
				}
			}
		case "done":
			if (len(message.Error) > 0) == (len(message.Result) > 0) {
				return nil, protocolError()
			}
			if len(message.Error) > 0 {
				var reported host.Error
				if json.Unmarshal(message.Error, &reported) != nil || reported.Message == "" {
					return nil, protocolError()
				}
				return nil, &reported
			}
			var outcome struct {
				Closing  *bool           `json:"closing"`
				Token    json.RawMessage `json:"token"`
				Settings json.RawMessage `json:"settings"`
			}
			if json.Unmarshal(message.Result, &outcome) != nil || outcome.Closing == nil {
				return nil, protocolError()
			}
			switch {
			case !action.Restart && !*outcome.Closing && saved != nil && sameJSON(saved, outcome.Settings) && preferAdministratorIs(outcome.Settings, action.Enabled):
			case action.Restart && *outcome.Closing && committed && acknowledged != "" && token(outcome.Token) == acknowledged:
			case action.Restart && !*outcome.Closing && administrator(snapshot) && sameJSON(settingsOf(snapshot), outcome.Settings):
			default:
				return nil, protocolError()
			}
			return message.Result, nil
		default:
			return nil, protocolError()
		}
		reply := map[string]any{"version": 1, "sequence": sequence}
		if failure != nil {
			reply["error"] = rpcError(failure)
		} else {
			reply["result"] = result
		}
		data, err := json.Marshal(reply)
		if err != nil {
			return nil, err
		}
		if err := protocol.WriteFrame(writer, data); err != nil {
			return nil, err
		}
	}
	return nil, protocolError()
}

// preferAdministratorIs reports whether the saved settings remember the
// requested administrator start. The Host writes the field only when true
// (omitempty), so an absent field means false. permissions.rs compared it
// with the request literally and so rejected every successful "turn off".
func preferAdministratorIs(settings json.RawMessage, enabled bool) bool {
	var value struct {
		Startup *struct {
			PreferAdministrator *bool `json:"preferAdministrator"`
		} `json:"startup"`
	}
	if json.Unmarshal(settings, &value) != nil || value.Startup == nil {
		return false
	}
	if value.Startup.PreferAdministrator == nil {
		return !enabled
	}
	return *value.Startup.PreferAdministrator == enabled
}

func administrator(snapshot json.RawMessage) bool {
	var value struct {
		Administrator bool `json:"administrator"`
	}
	return json.Unmarshal(snapshot, &value) == nil && value.Administrator
}

func settingsOf(snapshot json.RawMessage) json.RawMessage {
	var value struct {
		Settings json.RawMessage `json:"settings"`
	}
	_ = json.Unmarshal(snapshot, &value)
	return value.Settings
}

// token returns a 32-digit hexadecimal handoff token, or "".
func token(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil || len(value) != 32 {
		return ""
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return ""
		}
	}
	return value
}

// sameJSON compares two JSON values by their content, as serde_json's
// Value equality did.
func sameJSON(a, b json.RawMessage) bool {
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

// rpcError is the Host's error shape for a relayed failure.
func rpcError(err error) map[string]string {
	var reported *host.Error
	if errors.As(err, &reported) {
		return map[string]string{"code": reported.Code, "message": reported.Message}
	}
	return map[string]string{"code": "failed", "message": err.Error()}
}

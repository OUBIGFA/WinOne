//go:build windows

package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"winone/internal/host"
)

type PrivilegeState struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// Privilege transitions are asynchronous because UAC must remain under the user's control.
func (l *Lazy) handlePrivilege(req host.Request) host.Response {
	l.mu.Lock()
	defer l.mu.Unlock()
	if req.Method == "privilege.start" {
		if l.closing {
			return host.Response{ID: req.ID, Error: &host.RPCError{Code: "stopping", Message: "模块正在退出"}}
		}
		var args struct {
			Administrator bool `json:"administrator"`
		}
		if err := json.Unmarshal(req.Params, &args); err != nil {
			return host.Response{ID: req.ID, Error: &host.RPCError{Code: "invalid", Message: "权限设置无效"}}
		}
		if l.privilege.State != "running" {
			l.privilege = PrivilegeState{State: "running"}
			go l.changePrivilege(args.Administrator)
		}
	}
	state := l.privilege
	if state.State == "" {
		state.State = "idle"
	}
	data, _ := json.Marshal(state)
	return host.Response{ID: req.ID, Result: data}
}

func (l *Lazy) changePrivilege(administrator bool) {
	l.inflight.Wait()
	l.mu.Lock()
	previous := l.process
	l.mu.Unlock()
	var pins []json.RawMessage
	finish := func(process *Process, err error) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.process = process
		l.privilege = PrivilegeState{State: "completed"}
		if err != nil {
			l.privilege = PrivilegeState{State: "failed", Error: err.Error()}
		}
	}
	if previous != nil {
		select {
		case <-previous.exited:
			previous = nil
		default:
		}
	}
	if previous != nil {
		response := previous.Handle(host.Request{Version: 1, ID: "privilege-snapshot", Method: "snapshot"})
		if response.Error != nil {
			finish(previous, errors.New(response.Error.Message))
			return
		}
		var snapshot struct {
			OwnedWindows []json.RawMessage `json:"ownedWindows"`
		}
		if err := json.Unmarshal(response.Result, &snapshot); err != nil {
			finish(previous, err)
			return
		}
		if snapshot.OwnedWindows == nil {
			finish(previous, errors.New("置顶组件版本不匹配，请重新构建全部组件"))
			return
		}
		pins = snapshot.OwnedWindows
		if err := previous.Close(); err != nil {
			finish(previous, err)
			return
		}
	}
	var next *Process
	if administrator {
		next = StartElevated(l.ctx, l.id, l.name, l.exe, l.pipe, l.data)
	} else {
		next = Start(l.ctx, l.id, l.name, l.exe, l.pipe, l.data, false)
	}
	if next.Status().State != "ready" {
		cause := errors.New(next.Status().Detail)
		// A timed-out worker can still be alive. Never launch a duplicate against its pipe.
		if err := next.Close(); err != nil {
			finish(next, errors.Join(cause, err))
			return
		}
		if !administrator {
			finish(next, cause)
			return
		}
		restored := Start(l.ctx, l.id, l.name, l.exe, l.pipe, l.data, false)
		if restored.Status().State != "ready" {
			finish(restored, errors.Join(cause, errors.New("普通模式恢复失败："+restored.Status().Detail)))
			return
		}
		finish(restored, errors.Join(cause, restorePins(restored, pins)))
		return
	}
	finish(next, restorePins(next, pins))
}

func restorePins(process *Process, pins []json.RawMessage) error {
	var failures []error
	for _, pin := range pins {
		params, err := json.Marshal(struct {
			Window json.RawMessage `json:"window"`
			Pinned bool            `json:"pinned"`
		}{pin, true})
		if err != nil {
			failures = append(failures, err)
			continue
		}
		response := process.Handle(host.Request{Version: 1, ID: "privilege-restore", Method: "window.set", Params: params})
		if response.Error != nil {
			failures = append(failures, fmt.Errorf("恢复置顶失败：%s", response.Error.Message))
		}
	}
	return errors.Join(failures...)
}

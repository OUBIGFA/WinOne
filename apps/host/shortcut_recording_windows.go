//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"time"
	"winone/internal/host"
)

type shortcutRecording struct {
	token  string
	cancel context.CancelFunc
}

// Temporarily unregister WinOne's bindings so the foreground recorder receives
// the keys without launching a feature. Ownership and a deadline bound failures.
func (r *shortcutRuntime) beginRecording(owner uint32) (string, error) {
	if owner == 0 {
		return "", errors.New("录入窗口无效")
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, owner)
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recording != nil {
		windows.CloseHandle(process)
		return "", errors.New("已有快捷键正在录入")
	}
	if _, err = r.registrar.Update(nil); err != nil {
		windows.CloseHandle(process)
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	token := rand.Text()
	r.recording = &shortcutRecording{token: token, cancel: cancel}
	for i := range r.statuses {
		r.statuses[i].Registered = false
	}
	go func() {
		defer windows.CloseHandle(process)
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				r.endRecording(token)
				return
			case <-ticker.C:
				state, err := windows.WaitForSingleObject(process, 0)
				if err != nil || state == windows.WAIT_OBJECT_0 {
					r.endRecording(token)
					return
				}
			}
		}
	}()
	return token, nil
}

func (r *shortcutRuntime) endRecording(token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recording == nil {
		return nil
	}
	if token == "" || r.recording.token != token {
		return errors.New("快捷键录入已变更")
	}
	r.recording.cancel()
	r.recording = nil
	statuses, err := r.registrar.Update(r.bindings)
	if err != nil {
		r.lastError = err.Error()
		return err
	}
	r.statuses = statuses
	return nil
}

func (r *shortcutRuntime) handleRecording(request host.Request) host.Response {
	var args struct {
		ProcessID uint32 `json:"processId"`
		Token     string `json:"token"`
	}
	var result any
	err := json.Unmarshal(request.Params, &args)
	if err == nil {
		if request.Method == "record.start" {
			var token string
			token, err = r.beginRecording(args.ProcessID)
			result = map[string]string{"token": token}
		} else {
			err = r.endRecording(args.Token)
			result = map[string]bool{"stopped": err == nil}
		}
	}
	if err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "shortcut_recording", Message: err.Error()}}
	}
	data, _ := json.Marshal(result)
	return host.Response{ID: request.ID, Result: data}
}

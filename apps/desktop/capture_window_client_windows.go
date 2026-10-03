//go:build windows

package main

import (
	"context"
	"errors"
	"time"
	"winone/internal/capturehandoff"
)

type captureWindowStatus struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Error string `json:"error"`
}

// Settings remains hidden until the actual screenshot window exits. A launch
// acknowledgement, a saved image or a transient notifier exit is not completion.
func (a *App) OpenCaptureWindow(id string) error {
	if err := capturehandoff.ValidateDraftID(id); err != nil {
		return err
	}
	ctx := a.windowContext()
	if ctx == nil {
		return errors.New("窗口尚未就绪")
	}
	var opened struct {
		Accepted bool   `json:"accepted"`
		WindowID string `json:"windowId"`
	}
	if err := a.request("application.open", map[string]string{"draftId": id}, &opened); err != nil {
		return err
	}
	if !opened.Accepted || opened.WindowID == "" {
		return errors.New("截图窗口未能启动")
	}
	return waitCaptureWindow(ctx, opened.WindowID, func() (captureWindowStatus, error) {
		var status captureWindowStatus
		err := a.request("application.capture.window.status", map[string]string{"windowId": opened.WindowID}, &status)
		return status, err
	})
}

func waitCaptureWindow(ctx context.Context, id string, read func() (captureWindowStatus, error)) error {
	timer := time.NewTicker(200 * time.Millisecond)
	defer timer.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		status, err := read()
		if err != nil {
			return err
		}
		if status.ID != id {
			return errors.New("截图窗口状态不匹配")
		}
		switch status.State {
		case "completed":
			return nil
		case "failed":
			if status.Error != "" {
				return errors.New(status.Error)
			}
			return errors.New("截图窗口异常退出")
		case "running":
		default:
			return errors.New("截图窗口状态无效")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

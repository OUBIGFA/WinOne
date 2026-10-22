//go:build windows

package worker

import (
	"encoding/json"
	"errors"
	"winone/internal/configjson"
	"winone/modules/wintray/internal/config"
	"winone/modules/wintray/internal/orchestrator"
)

// Resolve every action against committed settings, never a caller-supplied
// executable/PID or a cached HWND. The UI's state only chooses its next label.
func (r *Runtime) programAction(method string, raw json.RawMessage) (any, error) {
	var args struct {
		ID        string `json:"id"`
		Visible   *bool  `json:"visible"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := configjson.DecodeObject(raw, &args); err != nil {
		return nil, err
	}
	snapshot := r.manager.Snapshot()
	if snapshot.RecoveryError != "" {
		return nil, errors.New(snapshot.RecoveryError)
	}
	var entry *config.ManagedAppEntry
	for _, value := range snapshot.Settings.ManagedApps {
		if value.ID == args.ID {
			copy := value
			entry = &copy
			break
		}
	}
	if entry == nil {
		return nil, errors.New("程序不存在，请刷新列表")
	}
	switch method {
	case "program.state":
		return r.orchestrator.ProgramState(*entry), nil
	case "program.stop":
		// The upstream exit action can terminate a remaining process tree. Require
		// an explicit confirmation instead of bypassing unsaved-document protection.
		if !args.Confirmed {
			return nil, errors.New("请先保存程序中的内容并确认退出")
		}
		return r.startJob(func() (any, error) { return nil, r.orchestrator.StopNow(r.ctx, *entry) }), nil
	case "program.window":
		if args.Visible == nil {
			return nil, errors.New("缺少窗口显示状态")
		}
		return r.startJob(func() (any, error) {
			window, err := r.orchestrator.SetProgramWindowVisible(r.ctx, *entry, *args.Visible)
			if err != nil {
				return window, err
			}
			if !*args.Visible {
				// Keep WinOne's existing recovery owner and unified tray entry. This also
				// restores the hidden native window when the module is disabled/exits.
				r.adopt(*entry, orchestrator.Result{Managed: true, Hidden: &window})
				if err := r.ctx.Err(); err != nil {
					return window, err
				}
				if window.IsVisible() {
					return window, errors.New("窗口恢复入口未能建立，已保留程序窗口，请重试")
				}
			}
			return window, nil
		}), nil
	}
	return nil, errors.New("不支持的程序操作")
}

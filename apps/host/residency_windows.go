//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"winone/internal/host"
	"winone/internal/residency"
)

// Residency probes never cold-start modules and treat unavailable state as a reason to stay.
func residencyState(pipe string, store host.SettingsStore, tray, capture, pin moduleEndpoint) residency.State {
	state := residency.State{Reasons: []string{}}
	if !tray.Enabled() {
		state.Reasons = append(state.Reasons, "启动与托盘已关闭")
		return state
	}
	response := tray.Handle(host.Request{Version: 1, ID: "residency", Method: "lifetime"})
	if response.Error != nil || json.Unmarshal(response.Result, &state) != nil {
		return residency.State{Reasons: []string{"无法确认程序任务状态"}}
	}
	s := store.Snapshot()
	if s.CaptureShortcut != "" && capture.Enabled() {
		state.Reasons = append(state.Reasons, "截图快捷键已启用")
	}
	if s.PinShortcut != "" && pin.Enabled() {
		state.Reasons = append(state.Reasons, "置顶快捷键已启用")
	}
	if open, err := residency.WindowOpen(pipe); open || err != nil {
		state.Reasons = append(state.Reasons, "设置窗口仍在使用")
	}
	if capture.Status().State == "ready" {
		r := capture.Handle(host.Request{Version: 1, ID: "residency-capture", Method: "snapshot"})
		var status struct {
			Busy bool `json:"busy"`
			Pins int  `json:"pins"`
		}
		if r.Error != nil || json.Unmarshal(r.Result, &status) != nil {
			state.Reasons = append(state.Reasons, "无法确认截图状态")
		} else {
			if status.Busy {
				state.Reasons = append(state.Reasons, "截图或编辑尚未完成")
			}
			if status.Pins > 0 {
				state.Reasons = append(state.Reasons, "仍有贴图窗口")
			}
		}
	} else if status := capture.Status().State; status != "available" && status != "disabled" {
		state.Reasons = append(state.Reasons, "截图模块需要处理")
	}
	if pin.Status().State == "ready" {
		r := pin.Handle(host.Request{Version: 1, ID: "residency-pin", Method: "snapshot"})
		var status struct {
			Windows []struct {
				Pinned bool `json:"pinned"`
			} `json:"ownedWindows"`
		}
		if r.Error != nil || json.Unmarshal(r.Result, &status) != nil || status.Windows == nil {
			state.Reasons = append(state.Reasons, "无法确认置顶状态")
		} else {
			for _, w := range status.Windows {
				if w.Pinned {
					state.Reasons = append(state.Reasons, "仍有置顶窗口")
					break
				}
			}
		}
	} else if status := pin.Status().State; status != "available" && status != "disabled" {
		state.Reasons = append(state.Reasons, "置顶模块需要处理")
	}
	return state
}
func residencyResponse(req host.Request, state residency.State) host.Response {
	if req.Method != "snapshot" {
		return host.Response{ID: req.ID, Error: &host.RPCError{Code: "invalid", Message: "不支持的后台状态操作"}}
	}
	data, err := json.Marshal(state)
	if err != nil {
		return host.Response{ID: req.ID, Error: &host.RPCError{Code: "internal", Message: fmt.Sprint(err)}}
	}
	return host.Response{ID: req.ID, Result: data}
}

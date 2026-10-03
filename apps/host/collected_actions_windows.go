//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"winone/internal/host"
)

func performCollectedAction(worker moduleEndpoint, id string, hosted bool, action collectedAction) error {
	payload := map[string]any{"id": id, "right": action.Right}
	if hosted {
		payload["action"] = action.ID
	}
	params, _ := json.Marshal(payload)
	response := worker.Handle(host.Request{Version: 1, ID: "tray-action", Method: "icon.activate", Params: params})
	if response.Error != nil {
		return errors.New(response.Error.Message)
	}
	var result struct {
		Delivered bool `json:"delivered"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil || !result.Delivered {
		return errors.New("无法确认程序操作结果")
	}
	return nil
}

// Native icons keep their application's menu. Hosted programs have no native
// menu, so WinOne presents the complete set of supported window actions.
type collectedAction struct {
	ID, Title string
	Right     bool
}

func collectedActions(hosted bool, locale string) []collectedAction {
	if locale == "en-US" {
		if hosted {
			return []collectedAction{{"open", "Show window", false}, {"hide", "Hide window", false}, {"release", "Stop managing", false}, {"quit", "Exit program", false}}
		}
		return []collectedAction{{"open", "Open", false}, {"menu", "Original menu", true}}
	}
	if hosted {
		return []collectedAction{{"open", "显示窗口", false}, {"hide", "隐藏窗口", false}, {"release", "取消托管", false}, {"quit", "退出程序", false}}
	}
	return []collectedAction{{"open", "打开", false}, {"menu", "原程序菜单", true}}
}

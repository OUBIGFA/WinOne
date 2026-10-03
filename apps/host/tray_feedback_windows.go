//go:build windows

package main

import "errors"

// Keep the reason in WinOne diagnostics even if Windows suppresses notifications.
// The notifier is injected so ordinary tests never display desktop notifications.
func reportTrayFailure(directory, locale string, failure error, notify func(string, string) error) error {
	if failure == nil {
		return nil
	}
	logErr := recordHostEvent(directory, "tray-action-failed", failure)
	message := "托盘操作遇到问题。详情见设置 → 诊断日志。"
	if locale == "en-US" {
		message = "Tray action needs attention. See Settings → Diagnostic logs."
	}
	if logErr != nil {
		message = failure.Error()
	}
	return errors.Join(logErr, notify("WinOne", message))
}

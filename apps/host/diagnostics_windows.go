//go:build windows

package main

import (
	"strings"
	"winone/internal/diagnostics"
)

// Preserve independent runtime warnings; log failures are diagnostic, not a
// reason to hide a restart/permission failure or prevent the Host from running.
func hostRuntimeWarnings(restartWarning, administratorWarning string, logFailure error, locale string) string {
	messages := []string{}
	for _, message := range []string{restartWarning, administratorWarning} {
		if message != "" {
			messages = append(messages, message)
		}
	}
	if logFailure != nil {
		prefix := "启动诊断日志未能写入："
		if locale == "en-US" {
			prefix = "The startup diagnostic log could not be written: "
		}
		messages = append(messages, prefix+logFailure.Error())
	}
	return strings.Join(messages, "\n")
}

// Host events retain bounded error details, not serialized settings.
// The shared writer owns serialization, size limits and rotation for every module.
func recordHostEvent(directory, event string, failure error) error {
	detail := ""
	if failure != nil {
		detail = failure.Error()
	}
	return diagnostics.Record(directory, diagnostics.Event{Source: "host", Event: event, Error: detail})
}

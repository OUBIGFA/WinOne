//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"fyne.io/systray"
)

type automationRunNotice struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	ResultText string `json:"resultText"`
	Error      string `json:"errorMsg"`
}

func (a *chatAutomations) notifyRun(ctx context.Context, schedule scheduledChat, run automationRunNotice) error {
	text := schedule.Name + "\n" + run.ResultText
	if run.Error != "" {
		text = schedule.Name + "\n" + run.Error
	}
	if len([]rune(text)) > 1000 {
		text = string([]rune(text)[:1000])
	}
	var failures []error
	if schedule.Notify.Desktop {
		notify := a.desktopNotify
		if notify == nil {
			notify = systray.ShowWarning
		}
		if err := notify("WinOne · "+run.Status, text); err != nil {
			failures = append(failures, err)
		}
	}
	for channel, enabled := range map[string]bool{"feishu": schedule.Notify.Feishu, "wecom": schedule.Notify.Wecom} {
		if !enabled {
			continue
		}
		schedule.NotificationChannel = channel
		schedule.NotificationText = text
		raw, err := a.command(ctx, schedule, "notification", run.ID, 0)
		if err == nil {
			var result struct {
				Error string `json:"error"`
			}
			err = json.Unmarshal(raw, &result)
			if err == nil && result.Error != "" {
				err = errors.New(result.Error)
			}
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", channel, err))
		}
	}
	return errors.Join(failures...)
}

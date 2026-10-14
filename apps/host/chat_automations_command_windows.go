//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
	"winone/internal/chat"
)

// Each command gets the existing runner's Job Object. EOF closes the worker,
// and Host job cleanup completes before a successful result is published.
func (a *chatAutomations) command(ctx context.Context, schedule scheduledChat, mode, runID string, firedAt int64) (json.RawMessage, error) {
	directory := schedule.WorkingDir
	if mode == "notification" {
		directory = filepath.Join(a.control.directory, "chat")
	}
	if schedule.TargetSessionID != "" && mode == "hook" {
		c, err := a.control.store.Conversation(schedule.TargetSessionID)
		if err != nil || c.Archived {
			return nil, errAutomationTargetUnavailable
		}
		if mode == "hook" {
			directory = c.Directory
		}
	}
	if directory == "" {
		directory = filepath.Join(a.control.directory, "chat")
		if err := os.MkdirAll(directory, 0700); err != nil {
			return nil, err
		}
	}
	if !filepath.IsAbs(directory) {
		return nil, errors.New("脚本工作目录必须是绝对路径")
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("脚本工作位置不是文件夹")
	}
	base := filepath.Dir(a.control.desktop)
	var hook json.RawMessage
	var hookMu sync.Mutex
	runner, err := chat.StartRunner(ctx, filepath.Join(base, "shared", "ai", "runtime", "bun.exe"), filepath.Join(base, "shared", "chat", "script-runner.js"), directory, func(event json.RawMessage) {
		var value struct {
			Type   string          `json:"type"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(event, &value) == nil && value.Type == "pre_run_hook" {
			hookMu.Lock()
			hook = append(json.RawMessage(nil), value.Result...)
			hookMu.Unlock()
		}
	})
	if err != nil {
		return nil, err
	}
	finishJob, err := runner.RetainCommandJob()
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return nil, errors.Join(err, runner.Close(cleanup))
	}
	closeRunner := func() error {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		closeErr := runner.Close(cleanup)
		wait, cancelWait := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelWait()
		return errors.Join(closeErr, finishJob(wait))
	}
	connections, err := a.connections()
	if err != nil {
		return nil, errors.Join(err, closeRunner())
	}
	if _, err = runner.Call(ctx, "init", map[string]any{"baseUrl": a.bridge.BaseURL, "key": a.bridge.Key, "settings": connections, "mcpServers": a.control.store.Preferences().MCPServers, "mcpAuthDirectory": filepath.Join(a.control.directory, "chat", "mcp")}); err != nil {
		return nil, errors.Join(err, closeRunner())
	}
	value, runErr := runner.Call(ctx, "execute", map[string]any{"schedule": schedule, "mode": mode, "runId": runID, "firedAt": firedAt, "workingDir": directory})
	closeErr := closeRunner()
	hookMu.Lock()
	defer hookMu.Unlock()
	// Preserve a failed pre-run diagnostic even when the script never starts.
	return json.Marshal(map[string]any{"value": value, "error": errorString(errors.Join(runErr, closeErr)), "preRunHook": hook})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

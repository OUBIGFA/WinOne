//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"path/filepath"
	"winone/internal/chat"
	"winone/internal/configjson"
)

func (a *App) exportChatArchive(payload string) (string, error) {
	var options struct {
		ID           string `json:"id"`
		Password     string `json:"password,omitempty"`
		ExcludeMedia bool   `json:"excludeMedia,omitempty"`
	}
	if err := configjson.DecodeObject([]byte(payload), &options); err != nil {
		return "", err
	}
	if options.ID == "" || len(options.Password) > 16384 {
		return "", errors.New("任务导出参数无效")
	}
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	target, err := runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{Title: a.dialogText("导出完整任务", "Export complete task"), DefaultFilename: "WinOne-task.cshare", CanCreateDirectories: true, Filters: []runtime.FileFilter{{DisplayName: a.dialogText("Cindy 任务文件", "Cindy task archive"), Pattern: "*.cshare"}}})
	if err != nil {
		return "", err
	}
	if target == "" {
		return `{"status":"canceled"}`, nil
	}
	if filepath.Ext(target) == "" {
		target += ".cshare"
	}
	var outcome chat.ArchiveOutcome
	if err = a.request("chat.export", chat.ArchiveOptions{ID: options.ID, Password: options.Password, ExcludeMedia: options.ExcludeMedia, TargetPath: target}, &outcome); err != nil {
		return "", err
	}
	if outcome.Status != "oversize" && (outcome.Status != "ok" || outcome.FilePath != target) {
		return "", errors.New(a.dialogText("无法确认任务已导出", "Unable to confirm that the task was exported"))
	}
	raw, err := json.Marshal(outcome)
	return string(raw), err
}

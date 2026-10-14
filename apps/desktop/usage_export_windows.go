//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"path/filepath"
	"winone/internal/ai"
	"winone/internal/configjson"
)

// Large CSV contents stay in the Host. The renderer supplies only filters;
// the native dialog supplies the destination and cancellation performs no I/O.
func (a *App) exportUsage(payload string) (string, error) {
	var query ai.UsageQuery
	if err := configjson.DecodeObject([]byte(payload), &query); err != nil {
		return "", err
	}
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	path, err := runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{Title: a.dialogText("导出用量 CSV", "Export usage CSV"), DefaultFilename: "WinOne-usage.csv", CanCreateDirectories: true, Filters: []runtime.FileFilter{{DisplayName: a.dialogText("CSV 表格", "CSV spreadsheet"), Pattern: "*.csv"}}})
	if err != nil {
		return "", err
	}
	if path == "" {
		return `{"state":"cancelled"}`, nil
	}
	if filepath.Ext(path) == "" {
		path += ".csv"
	}
	var result json.RawMessage
	if err = a.request("capture.ai.usage.export.save", map[string]any{"query": query, "path": path}, &result); err != nil {
		return "", err
	}
	var receipt struct {
		State string `json:"state"`
		Path  string `json:"path"`
		Rows  *int   `json:"rows"`
	}
	if err = json.Unmarshal(result, &receipt); err != nil {
		return "", err
	}
	if receipt.State != "saved" || receipt.Path != path || receipt.Rows == nil {
		return "", errors.New(a.dialogText("无法确认 CSV 已保存", "Unable to confirm that the CSV was saved"))
	}
	return string(result), nil
}

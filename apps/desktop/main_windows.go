//go:build windows

package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsOptions "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"winone/internal/adminstartup"
	"winone/internal/capturehandoff"
	"winone/internal/host"
	"winone/internal/logintask"
	"winone/internal/maintenance"
	"winone/internal/previewtransfer"
	"winone/internal/residency"
	"winone/internal/resultwindow"
	"winone/internal/settings"
	"winone/internal/transport"
)

//go:embed all:frontend/dist
var assets embed.FS

// App exposes the small settings API, never the Host's process or filesystem access.
type App struct {
	updateSession          updateSession
	authorizationMu        sync.Mutex
	captureSurface         *captureSurfaceState
	captureWindowBefore    *captureSurfaceState
	dedicatedCapture       bool
	captureOnly            bool
	settingsNavigation     uint64
	frameInitMu            sync.Mutex
	frameInitialized       bool
	settingsDirty          bool
	settingsBusy           bool
	settingsClosePending   bool
	settingsCloseApproved  bool
	startupProbe           *desktopProbeState
	pipe                   string
	sequence               atomic.Uint64
	ctxMu                  sync.RWMutex
	ctx                    context.Context
	startupError           error
	startupWarning         string
	locale                 string
	captureLaunches        capturehandoff.Queue
	captureEditing         bool
	captureClosePending    bool
	captureDirectory       string
	dataDirectory          string
	captureRecoveryChecked bool
}

func (a *App) dialogText(chinese, english string) string {
	a.ctxMu.RLock()
	locale := a.locale
	a.ctxMu.RUnlock()
	if locale == "en-US" {
		return english
	}
	return chinese
}

func (a *App) windowContext() context.Context { a.ctxMu.RLock(); defer a.ctxMu.RUnlock(); return a.ctx }

func (a *App) applyTheme(theme string) {
	ctx := a.windowContext()
	if ctx == nil {
		return
	}
	switch theme {
	case "dark":
		runtime.WindowSetDarkTheme(ctx)
	case "light":
		runtime.WindowSetLightTheme(ctx)
	default:
		runtime.WindowSetSystemDefaultTheme(ctx)
	}
}

func (a *App) request(method string, params any, result any) error {
	if a.startupProbe != nil && method != "snapshot" && method != "modules.snapshot" {
		return errors.New("startup probe permits snapshot only")
	}
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	timeout := 5 * time.Second
	if strings.HasPrefix(method, "chat.") {
		timeout = 30 * time.Second
	}
	if strings.HasPrefix(method, "capture.") || strings.HasPrefix(method, "always-on-top.") {
		timeout = 15 * time.Second
	}
	if method == "capture.ai.usage.export.save" {
		timeout = 30 * time.Second
	}
	if method == "capture.ai.archive.export.save" || method == "capture.ai.archive.read" {
		timeout = 150 * time.Second
	}
	if method == "capture.ai.magpie.catalog" {
		timeout = 30 * time.Second
	}
	if method == "capture.ai.models.test" {
		timeout = 100 * time.Second
	}
	if method == "chat.export" {
		timeout = 150 * time.Second
	}
	r, err := transport.CallCompleting(context.Background(), a.pipe, host.Request{Version: 1, ID: fmt.Sprint(a.sequence.Add(1)), Method: method, Params: data}, timeout)
	if err != nil {
		return fmt.Errorf("无法连接后台：%w", err)
	}
	if r.Error != nil {
		return errors.New(r.Error.Message)
	}
	err = json.Unmarshal(r.Result, result)
	if err == nil && a.startupProbe != nil && method == "modules.snapshot" {
		a.startupProbe.mu.Lock()
		a.startupProbe.modulesRead = true
		a.startupProbe.mu.Unlock()
	}
	return err
}

// Snapshot is the read-only page bootstrap API exposed through Wails.
func (a *App) Snapshot() (host.Snapshot, error) {
	var result host.Snapshot
	err := a.request("snapshot", nil, &result)
	if err != nil {
		return result, errors.Join(a.startupError, err)
	}
	if a.startupProbe != nil {
		a.startupProbe.mu.Lock()
		a.startupProbe.snapshot = true
		a.startupProbe.mu.Unlock()
	}
	a.applyTheme(result.Settings.Theme)
	a.ctxMu.Lock()
	a.locale = result.Locale
	if a.locale == "" && result.Settings.Language == "en-US" {
		a.locale = "en-US"
	}
	a.ctxMu.Unlock()
	if a.startupWarning != "" && result.Settings.Startup != nil && result.Settings.Startup.PreferAdministrator && !result.Administrator {
		if result.RuntimeWarning != "" {
			result.RuntimeWarning += "\n"
		}
		result.RuntimeWarning += a.startupWarning
	}
	return result, err
}

// SaveSettings preserves the Host's revision checks before changing the window theme.
func (a *App) SaveSettings(next settings.Settings) (settings.Settings, error) {
	var result settings.Settings
	err := a.request("settings.save", next, &result)
	if err == nil {
		a.applyTheme(result.Theme)
	}
	return result, err
}

// WinTray limits page access to the module's documented command set.
func (a *App) WinTray(method, payload string) (string, error) {
	switch method {
	case "snapshot", "settings.get", "settings.save", "launch", "jobs.get", "icons", "icon.activate", "recover", "import.preview", "startup.retry":
	default:
		return "", errors.New("不支持的程序管理操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("wintray."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// AlwaysOnTop exposes only supported commands, retaining native identity checks.
func (a *App) Shortcuts() (string, error) {
	var result json.RawMessage
	if err := a.request("shortcuts.snapshot", nil, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// Residency reports live blockers for login-session automatic exit.
func (a *App) Modules(method, payload string) (string, error) {
	switch method {
	case "snapshot", "catalog", "setEnabled", "jobs.get", "recoverConfiguration", "resetWinTray.preview", "resetWinTray":
	default:
		return "", errors.New("不支持的模块管理操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("modules."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// Residency reports live blockers for login-session automatic exit.
func (a *App) Residency() (string, error) {
	var result json.RawMessage
	if err := a.request("residency.snapshot", nil, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// AlwaysOnTop exposes only supported commands, retaining native identity checks.
func (a *App) AlwaysOnTop(method, payload string) (string, error) {
	switch method {
	case "snapshot", "settings.get", "settings.save", "window.set", "clear", "privilege.start", "privilege.status":
	default:
		return "", errors.New("不支持的置顶操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("always-on-top."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// Selection shares the Host module contract and never owns AI configuration.
func (a *App) Selection(method, payload string) (string, error) {
	switch method {
	case "snapshot", "settings.get", "settings.save":
	default:
		return "", errors.New("不支持的划词操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("selection."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

func (a *App) QuickAssistant(method, payload string) (string, error) {
	switch method {
	case "snapshot", "settings.get", "settings.save", "show":
	default:
		return "", errors.New("不支持的快捷助手操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("quick-assistant."+method, params, &result); err != nil {
		return "", err
	}
	if method == "show" {
		var target resultwindow.WindowTarget
		if err := json.Unmarshal(result, &target); err != nil {
			return "", err
		}
		if err := target.Activate(); err != nil {
			return "", err
		}
	}
	return string(result), nil
}

// Chat remains in the shared Host; closing this window does not own an agent.
func (a *App) Chat(method, payload string) (string, error) {
	if method == "export" {
		return a.exportChatArchive(payload)
	}
	switch strings.TrimPrefix(method, "onebot.chat.") {
	case "onebot.tasks.pr.refs", "onebot.tasks.pr.statuses", "onebot.tasks.list", "onebot.tasks.get", "onebot.tasks.stop", "onebot.tasks.message", "onebot.tasks.trace", "onebot.capabilities.get", "onebot.events", "onebot.projects.list", "onebot.projects.set", "onebot.history.search", "onebot.lifecycle.action", "onebot.session.open", "onebot.profiles.list", "onebot.profiles.get", "onebot.profiles.versions", "onebot.profile.content.read", "onebot.memory.list", "onebot.memory.read", "onebot.memory.index", "onebot.memory.search", "onebot.skills.list", "onebot.skills.query", "onebot.skills.read", "onebot.skills.readSource", "onebot.skills.mounts", "onebot.todos.read",
		"onebot.profiles.create", "onebot.profiles.update", "onebot.profile.content.write", "onebot.memory.update", "onebot.memory.delete", "onebot.memory.write", "onebot.memory.consolidate", "onebot.memory.importDocument", "onebot.skills.save", "onebot.skills.saveSource", "onebot.skills.delete", "onebot.todos.patch":
	case "usage", "project.git", "automations.deleted", "automations.restore", "automations.project.read", "automations.project.write", "automations.project.reconcile", "automations.project.promote", "automations.project.clone", "automations.project.demote", "automations.silence", "automations.notify", "automations.connections.get", "automations.connections.save", "automations.templates", "automations.template.apply", "automations.template.save", "automations.template.delete", "automations.list", "automations.get", "automations.create", "automations.update", "automations.pause", "automations.resume", "automations.delete", "automations.run", "automations.runs", "automations.deleteRun", "automations.markRead", "automations.runtime", "automations.preview", "automations.sidebarIndexRuns", "automations.inflightCount", "automations.detectCwd", "automations.channels", "automations.capabilityStatus", "automations.hook.test", "automations.hook.generate", "automations.markAllRead", "automations.project.upsertSchedule", "automations.project.removeSchedule", "automations.open", "automations.events", "catalog", "settings.get", "settings.save", "resources", "files", "attachments.add", "automation.run", "project.add", "project.create", "project.update", "project.remove", "conversation.new", "conversation.update", "conversation.move", "conversation.delete", "title.generate", "title.regenerate", "configure", "location", "fork", "draft.get", "draft.save", "open", "poll", "prompt", "state", "permission", "stop", "compact", "release", "detach":
	default:
		return "", errors.New("不支持的 Chat 操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("chat."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// PickProgram uses the native file picker; the selected file is not executed here.
func (a *App) PickWinTrayConfig() (string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: a.dialogText("选择 WinTray 配置", "Choose WinTray settings"), Filters: []runtime.FileFilter{{DisplayName: a.dialogText("WinTray JSON 配置", "WinTray JSON settings"), Pattern: "*.json"}}})
}

// PickProgram uses the native file picker; the selected file is not executed here.
func (a *App) PickProgram() (string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: a.dialogText("选择程序或脚本", "Choose a program or script"), Filters: []runtime.FileFilter{{DisplayName: a.dialogText("程序与脚本", "Programs and scripts"), Pattern: "*.exe;*.bat;*.cmd;*.ps1;*.py;*.pyw"}}})
}

// PickPrograms adds several entries without launching or persisting them.
func (a *App) PickPrograms() ([]string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return nil, errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	return runtime.OpenMultipleFilesDialog(ctx, runtime.OpenDialogOptions{Title: a.dialogText("选择程序或脚本", "Choose programs or scripts"), Filters: []runtime.FileFilter{{DisplayName: a.dialogText("程序与脚本", "Programs and scripts"), Pattern: "*.exe;*.bat;*.cmd;*.ps1;*.py;*.pyw"}}})
}

// Capture forwards only supported image commands; it hides settings before starting capture.
func (a *App) Capture(method, payload string) (string, error) {
	if method == "ai.archive.export" {
		return a.exportRequestArchive(payload)
	}
	if method == "ai.archive.read" {
		var result json.RawMessage
		if err := a.request("capture.ai.archive.read", json.RawMessage(payload), &result); err != nil {
			return "", err
		}
		return string(result), nil
	}
	if method == "ai.usage.export" {
		return a.exportUsage(payload)
	}
	switch method {
	case "snapshot", "settings.get", "settings.save", "filename.preview", "capture.start", "capture.unified", "scroll.stop", "scroll.retake", "handoff.release", "draft.begin", "draft.get", "draft.update", "draft.cancel", "draft.complete", "draft.confirm", "selection.candidates", "selection.commit", "draft.copy", "draft.pin", "edit.last", "edit.apply", "edit.preview", "copy.last", "save.last", "pin.last", "pins.close", "jobs.get":
	default:
		return "", errors.New("不支持的截图操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	hide := method == "capture.start" || method == "capture.unified" || method == "scroll.retake" || method == "edit.last"
	if method == "save.last" || method == "edit.apply" || method == "draft.complete" {
		var identity struct {
			ExpectedID string `json:"expectedId"`
			Revision   int64  `json:"revision"`
			SaveAs     bool   `json:"saveAs"`
			SaveCopy   bool   `json:"saveCopy"`
		}
		if err := json.Unmarshal([]byte(payload), &identity); err != nil {
			return "", err
		}
		var values map[string]any
		if err := json.Unmarshal([]byte(payload), &values); err != nil {
			return "", err
		}
		if values == nil {
			values = map[string]any{}
		}
		if method == "edit.apply" && values["document"] == nil {
			return "", errors.New(a.dialogText("标注为空", "No annotations were provided"))
		}
		choosePath := true
		if method == "draft.complete" {
			var response struct {
				Draft *struct {
					ID       string `json:"id"`
					Revision int64  `json:"revision"`
					Settings struct {
						Ask bool `json:"askWhereToSave"`
					} `json:"settings"`
				} `json:"draft"`
			}
			if err := a.request("capture.draft.get", map[string]string{"expectedId": identity.ExpectedID}, &response); err != nil {
				return "", err
			}
			if response.Draft == nil || response.Draft.ID != identity.ExpectedID || response.Draft.Revision != identity.Revision {
				return "", errors.New(a.dialogText("截图草稿已变化，请重新加载", "Draft changed. Reload before saving."))
			}
			choosePath = !identity.SaveCopy && (response.Draft.Settings.Ask || identity.SaveAs)
			delete(values, "path")
		}
		if choosePath {
			ctx := a.windowContext()
			if ctx == nil {
				return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
			}
			var suggested struct {
				Name string `json:"name"`
			}
			if err := a.request("capture.filename.preview", map[string]string{}, &suggested); err != nil {
				return "", err
			}
			path, err := runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{Title: a.dialogText("截图另存为", "Save capture as"), DefaultFilename: suggested.Name, CanCreateDirectories: true, Filters: []runtime.FileFilter{{DisplayName: a.dialogText("PNG 图像", "PNG image"), Pattern: "*.png"}}})
			if err != nil {
				return "", err
			}
			if path == "" {
				return `{"state":"cancelled"}`, nil
			}
			if filepath.Ext(path) == "" {
				path += ".png"
			}
			values["path"] = path
		}
		if identity.ExpectedID != "" {
			values["expectedId"] = identity.ExpectedID
		}
		params = values
	}
	// Attach locale after save-dialog parameters are rebuilt, including pins
	// created automatically when a draft is completed.
	if method == "capture.unified" || method == "scroll.retake" || method == "pin.last" || method == "draft.pin" || method == "draft.complete" {
		if params == nil && method != "capture.unified" {
			params = map[string]any{}
		}
		values, ok := params.(map[string]any)
		if !ok {
			return "", errors.New("invalid capture parameters")
		}
		a.ctxMu.RLock()
		locale := a.locale
		a.ctxMu.RUnlock()
		if locale != "en-US" {
			locale = "zh-CN"
		}
		values["language"] = locale
	}
	if hide {
		if ctx := a.windowContext(); ctx != nil {
			if err := a.rememberCaptureWindow(); err != nil {
				return "", err
			}
			values, _ := params.(map[string]any)
			if method == "scroll.retake" || method == "capture.unified" && values["mode"] == "scroll" {
				// Keep a taskbar entry so returning to WinOne can stop scrolling.
				runtime.WindowMinimise(ctx)
				time.Sleep(180 * time.Millisecond)
			} else {
				runtime.WindowHide(ctx)
				if err := a.captureHiddenWindowReady(); err != nil {
					return "", errors.Join(err, a.Reveal())
				}
			}
		}
	}
	var result json.RawMessage
	if err := a.request("capture."+method, params, &result); err != nil {
		if hide {
			if restoreErr := a.Reveal(); restoreErr != nil {
				return "", errors.Join(err, restoreErr)
			}
		}
		return "", err
	}
	return string(result), nil
}

// Reveal restores the settings window after a UI-initiated capture finishes or is cancelled.
func (a *App) Reveal() error {
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	if a.captureSurface != nil {
		return nil
	}
	if a.captureWindowBefore != nil {
		if err := a.captureWindowBefore.restore(); err != nil {
			return err
		}
		a.captureWindowBefore = nil
		return nil
	}
	if ctx := a.windowContext(); ctx != nil {
		runtime.WindowShow(ctx)
	}
	return nil
}

// A failed in-place session must expose its retained draft even when the
// original settings window was minimized. Normal completion still uses Reveal.
func (a *App) RevealCaptureEditor() error {
	if err := a.Reveal(); err != nil {
		return err
	}
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	if a.captureSurface != nil {
		return errors.New("请先恢复截图窗口")
	}
	ctx := a.windowContext()
	if ctx == nil {
		return errors.New("窗口尚未就绪")
	}
	runtime.WindowUnminimise(ctx)
	runtime.WindowShow(ctx)
	return nil
}

func (a *App) PickDirectory() (string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{Title: a.dialogText("选择文件夹", "Choose a folder")})
}
func (a *App) PickChatFiles() ([]string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return nil, errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	return runtime.OpenMultipleFilesDialog(ctx, runtime.OpenDialogOptions{Title: a.dialogText("添加附件", "Add attachments")})
}
func (a *App) PickFile() (string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: a.dialogText("选择文件", "Choose a file")})
}
func (a *App) DirectoryLinks(method, payload string) (string, error) {
	switch method {
	case "browse", "preview", "previewBatch", "create", "create.elevated", "createBatch", "createBatch.elevated", "list", "inspect", "inspectBatch", "scan", "scan.cancel", "remove", "forget", "forgetInvalid", "exportPlan", "history.export", "history.preview", "history.import", "migration.preview", "migration.execute", "migration.status", "migration.cancel", "migration.recover", "migration.cleanup", "menu.get", "menu.set", "reveal", "open", "pending":
	default:
		return "", errors.New("不支持的目录链接操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("directory-links."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// Interconnect exposes the same restricted adapter in the legacy desktop.
func (a *App) Interconnect(method, payload string) (string, error) {
	switch method {
	case "transfer_text", "transfer_text_open", "pending", "menu.get", "menu.set", "general.get", "general.save", "entry.get", "entry.show", "entry.hide", "get", "start", "stop", "add_files", "add_text", "remove", "item_path", "download_name", "save_as", "preferences.save", "share_code_restart", "share_code_stop", "share_code_answer", "share_add_text", "share_save_as":
	case "transfer_list", "transfer_start", "transfer_stop", "transfer_answer", "transfer_remove", "share_revoke", "transfer_restart", "share_add", "share_remove", "transfer_files":
	default:
		return "", errors.New("不支持的互联助手操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("interconnect."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// TimedPower retains the restricted bridge for the legacy desktop.
func (a *App) TimedPower(method, payload string) (string, error) {
	switch method {
	case "snapshot", "settings.get", "settings.save", "start", "cancel":
	default:
		return "", errors.New("不支持的定时关机操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("timed-power."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

func (a *App) Stroke(method, payload string) (string, error) {
	switch method {
	case "snapshot", "settings.get", "settings.save", "record.start", "record.read", "record.cancel", "inspect.target", "windows.list":
	default:
		return "", errors.New("不支持的鼠标手势操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	var result json.RawMessage
	if err := a.request("stroke."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// ExportProfile uses the native save dialog; cancellation performs no write.
func (a *App) ExportProfile() (string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	path, err := runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{Title: a.dialogText("导出配置", "Export configuration"), DefaultFilename: "WinOne.winone", CanCreateDirectories: true, Filters: []runtime.FileFilter{{DisplayName: a.dialogText("WinOne 配置", "WinOne configuration"), Pattern: "*.winone"}}})
	if err != nil || path == "" {
		return "", err
	}
	if filepath.Ext(path) == "" {
		path += ".winone"
	}
	var result struct {
		Path string `json:"path"`
	}
	if err = a.request("profiles.export", map[string]string{"path": path, "name": a.dialogText("我的配置", "My configuration")}, &result); err != nil {
		return "", err
	}
	return result.Path, nil
}

func (a *App) PickProfile() (string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: a.dialogText("选择配置文件", "Choose a configuration file"), Filters: []runtime.FileFilter{{DisplayName: a.dialogText("WinOne 配置", "WinOne configuration"), Pattern: "*.winone"}}})
}

func (a *App) PickModulePackage() (string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: a.dialogText("选择模块包", "Choose a module package"), Filters: []runtime.FileFilter{{DisplayName: a.dialogText("WinOne 模块包", "WinOne module package"), Pattern: "*.winone-module;*.winone-runtime;*.zip"}}})
}

func (a *App) PickProfileLocation(kind string) (string, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return "", errors.New(a.dialogText("设置窗口未就绪", "Settings window is not ready"))
	}
	switch kind {
	case "directory":
		return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{Title: a.dialogText("选择文件夹", "Choose a folder")})
	case "program":
		return a.PickProgram()
	case "file":
		return a.PickFile()
	default:
		return "", errors.New("不支持的路径类型")
	}
}

func (a *App) Profiles(method, payload string) (string, error) {
	if method != "import.preview" && method != "import.apply" {
		return "", errors.New("不支持的配置操作")
	}
	var params any
	if err := json.Unmarshal([]byte(payload), &params); err != nil {
		return "", err
	}
	if method == "import.preview" {
		var descriptor previewtransfer.Descriptor
		if err := a.request("profiles.import.preview.start", params, &descriptor); err != nil {
			return "", err
		}
		defer func() {
			var result any
			if err := a.request("profiles.import.preview.release", map[string]string{"token": descriptor.Token}, &result); err != nil {
				fmt.Fprintln(os.Stderr, "preview cleanup:", err)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		data, err := previewtransfer.Receive(ctx, descriptor, func(offset int) (previewtransfer.Chunk, error) {
			var chunk previewtransfer.Chunk
			err := a.request("profiles.import.preview.read", map[string]any{"token": descriptor.Token, "offset": offset}, &chunk)
			return chunk, err
		})
		if err != nil {
			return "", err
		}
		if !json.Valid(data) {
			return "", errors.New(a.dialogText("配置预览格式无效", "Invalid profile preview"))
		}
		return string(data), nil
	}
	var result json.RawMessage
	if err := a.request("profiles."+method, params, &result); err != nil {
		return "", err
	}
	return string(result), nil
}

// ensureHost starts only the sibling binary and waits for its actual pipe readiness.
func ensureHost(pipe, dataDir string, standard ...bool) error {
	probe := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		response, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "ready", Method: "snapshot"})
		if err != nil {
			return err
		}
		if response.Error != nil {
			return errors.New(response.Error.Message)
		}
		return nil
	}
	if probe() == nil {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(exe), "WinOne.Host.exe"), "--pipe", pipe, "--data-dir", dataDir)
	if len(standard) > 0 && standard[0] {
		cmd.Args = append(cmd.Args, "--standard-session")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err = cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	for range 40 {
		if probe() == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("后台启动失败，请检查配置目录与日志")
}

func main() {
	if executable, err := os.Executable(); err == nil {
		if handled, err := runAdministratorAuthorizationHelper(os.Args[1:], executable); handled {
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
		if handled, err := runAuthorizedStartHelper(os.Args[1:], executable); handled {
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
	}
	pipeDefault, err := transport.PipeName()
	if err != nil {
		panic(err)
	}
	pipe := flag.String("pipe", pipeDefault, "local pipe name")
	data := flag.String("data-dir", filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne"), "settings directory")
	initialCapture := flag.String("capture-draft", "", "capture draft to open")
	openSettings := flag.Bool("open-settings", false, "open global settings")
	standardSession := flag.Bool("standard-session", false, "skip remembered administrator startup for recovery")
	uiProbe := flag.String("check-ui", "", "desktop probe nonce")
	probeRoot := flag.String("probe-root", "", "existing probe scratch directory")
	flag.Parse()
	probeRequested := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "check-ui" || f.Name == "probe-root" {
			probeRequested = true
		}
	})
	if probeRequested {
		valid := flag.NArg() == 0
		flag.Visit(func(f *flag.Flag) {
			if f.Name != "check-ui" && f.Name != "probe-root" {
				valid = false
			}
		})
		if !valid {
			fmt.Fprintln(os.Stderr, "invalid desktop probe arguments")
			os.Exit(1)
		}
		if err := runDesktopProbe(*uiProbe, *probeRoot); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	executablePath, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	lease, err := maintenance.AcquireRuntimeForInstallation(*data, filepath.Dir(executablePath))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer lease.Close()
	redirected, redirectErr := adminstartup.Try(context.Background(), *data, *pipe, executablePath, logintask.InteractiveLaunch, *standardSession, *initialCapture)
	if redirected {
		return
	}
	app := &App{pipe: *pipe, dataDirectory: *data, captureDirectory: filepath.Join(*data, "modules", "capture"), captureOnly: *initialCapture != "", dedicatedCapture: *initialCapture != ""}
	if redirectErr != nil {
		app.startupWarning = "本次未以管理员身份启动：" + redirectErr.Error()
	}
	if *openSettings {
		app.requestSettingsNavigation()
	}
	draftID, err := capturehandoff.DraftFromArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if draftID != "" {
		if err = app.queueCaptureDraft(draftID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	releaseWindow, err := residency.MarkWindow(*pipe)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer releaseWindow()
	// A failed Host is shown in the page with retry, not replaced by fabricated settings.
	app.startupError = ensureHost(*pipe, *data, *standardSession || redirectErr != nil)
	role := desktopRoleFor(*pipe, app.dedicatedCapture)
	instanceID := role.InstanceID
	assetRoot, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := grantExistingWindowForeground(instanceID, executablePath); err != nil {
		fmt.Fprintln(os.Stderr, "Window activation:", err)
	}
	err = wails.Run(&options.App{
		Title: "WinOne", Width: 1280, Height: 840, MinWidth: 800, MinHeight: 560, Frameless: true, StartHidden: true,
		BackgroundColour: options.NewRGB(248, 248, 246), HideWindowOnClose: false,
		AssetServer: &assetserver.Options{Assets: desktopPageAssets(assetRoot, app.dedicatedCapture), Handler: captureAssets(filepath.Join(*data, "modules", "capture"))}, Bind: []interface{}{app},
		OnStartup: func(ctx context.Context) { app.ctxMu.Lock(); app.ctx = ctx; app.ctxMu.Unlock() },
		OnDomReady: func(context.Context) {
			if app.dedicatedCapture {
				if err := app.prepareDedicatedCaptureWindow(); err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			} else if _, err := app.InitializeWindow(); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			app.notifyCaptureLaunch()
		},
		OnBeforeClose: func(context.Context) bool { return app.blockCaptureClose() },
		OnShutdown:    func(context.Context) { app.updateSession.close() },
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: instanceID, OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
			for _, arg := range data.Args {
				if arg == "--open-settings" {
					app.requestSettingsNavigation()
					break
				}
			}
			id, err := capturehandoff.DraftFromArgs(data.Args)
			if err == nil && id != "" {
				err = app.queueCaptureDraft(id)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			if app.dedicatedCapture {
				return
			}
			if ctx := app.windowContext(); ctx != nil {
				app.frameInitMu.Lock()
				if app.captureSurface == nil {
					runtime.WindowUnminimise(ctx)
					runtime.WindowShow(ctx)
				}
				app.frameInitMu.Unlock()
			}
		}},
		DragAndDrop: &options.DragAndDrop{DisableWebViewDrop: true},
		Windows:     &windowsOptions.Options{WebviewUserDataPath: filepath.Join(*data, role.WebviewDirectory), Theme: windowsOptions.SystemDefault, WindowClassName: role.WindowClass},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

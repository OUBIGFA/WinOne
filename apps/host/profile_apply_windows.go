//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"winone/internal/configuration"
	"winone/internal/modulestate"
	"winone/internal/profiles"
	"winone/internal/settings"
	"winone/modules/wintray/preferences"
)

type profileApplyRequest struct {
	Path                  string            `json:"path"`
	Bindings              map[string]string `json:"bindings"`
	SourceHash            string            `json:"sourceHash"`
	ConfigurationRevision *uint64           `json:"configurationRevision"`
	ModuleRevision        *uint64           `json:"moduleRevision"`
	Modules               []string          `json:"modules"`
	Global                bool              `json:"global"`
}
type deferredProfileModule struct {
	Module    profiles.ModuleSettings           `json:"module"`
	Paths     map[string]profiles.PathReference `json:"paths"`
	Enabled   *bool                             `json:"enabled,omitempty"`
	Shortcuts map[string]string                 `json:"shortcuts"`
}
type profileApplyResult struct {
	Revision uint64   `json:"revision"`
	Modules  []string `json:"modules"`
}

// applyProfile rebuilds the candidate from the bytes it just verified. The UI's
// preview contains no trusted patch, and all native changes occur inside Commit.
func applyProfile(request profileApplyRequest, store *configuration.HostStore, states *modulestate.Store) (profileApplyResult, error) {
	empty := profileApplyResult{}
	if request.SourceHash == "" || request.ConfigurationRevision == nil || request.ModuleRevision == nil {
		return empty, errors.New("请先预览配置")
	}
	if len(request.Modules) == 0 && !request.Global {
		return empty, errors.New("请选择要导入的设置")
	}
	preview, err := previewProfileFile(request.Path, request.Bindings, store, states)
	if err != nil {
		return empty, err
	}
	if preview.SourceHash != request.SourceHash {
		return empty, errors.New("配置文件已变化，请重新预览")
	}
	if preview.ConfigurationRevision != *request.ConfigurationRevision || preview.ModuleRevision != *request.ModuleRevision {
		return empty, configuration.ErrConflict
	}
	document := store.CommittedDocument()
	if document.Revision != preview.ConfigurationRevision {
		return empty, configuration.ErrConflict
	}
	if document.ModuleEnabled == nil {
		return empty, errors.New("模块开关尚未接入统一配置")
	}
	var global settings.Settings
	if err = json.Unmarshal(document.Values["host"], &global); err != nil {
		return empty, err
	}
	if request.Global {
		global.Theme = preview.Preferences.Theme
		global.Language = preview.Preferences.Language
	}
	plans := map[string]profiles.ModulePlan{}
	for _, item := range preview.Plan.Modules {
		plans[item.ID] = item
	}
	selected := map[string]bool{}
	for _, id := range request.Modules {
		item, exists := plans[id]
		if !exists || selected[id] {
			return empty, errors.New("模块选择无效或重复")
		}
		selected[id] = true
		if item.State == "missing_module" {
			if document.DeferredModules == nil {
				document.DeferredModules = map[string]json.RawMessage{}
			}
			deferred := deferredProfileModule{Module: preview.archive.Modules[id], Paths: preview.archive.Paths, Shortcuts: map[string]string{}}
			if enabled, exists := preview.Preferences.Enabled[id]; exists {
				deferred.Enabled = &enabled
			}
			for action, key := range preview.Preferences.Shortcuts {
				if strings.HasPrefix(action, id+".") {
					deferred.Shortcuts[action] = key
				}
			}
			raw, err := json.Marshal(deferred)
			if err != nil {
				return empty, err
			}
			document.DeferredModules[id] = raw
			continue
		}
		if item.State != "ready" || len(item.Patch) == 0 {
			return empty, fmt.Errorf("%s：请先处理配置或路径问题", id)
		}
		merged, err := mergeProfilePatch(document.Values[id], item.Patch)
		if err != nil {
			return empty, err
		}
		document.Values[id] = merged
		if enabled, exists := preview.Preferences.Enabled[id]; exists {
			document.ModuleEnabled[id] = enabled
		}
	}
	// Only actions implemented by the current Host can be registered. Other
	// installed-module shortcuts must not be silently discarded during import.
	for action, key := range preview.Preferences.Shortcuts {
		if selected["capture"] && strings.HasPrefix(action, "capture.") {
			if action != "capture.region" {
				return empty, fmt.Errorf("当前版本尚不支持导入快捷键 %s", action)
			}
			global.CaptureShortcut = key
		}
		if selected["always-on-top"] && strings.HasPrefix(action, "always-on-top.") {
			if action != "always-on-top.toggle" {
				return empty, fmt.Errorf("当前版本尚不支持导入快捷键 %s", action)
			}
			global.PinShortcut = key
		}
		if selected["wintray"] && strings.HasPrefix(action, "wintray.") {
			return empty, errors.New("启动与托盘不支持快捷键设置")
		}
	}
	if request.Global {
		preferAdministrator := global.Startup != nil && global.Startup.PreferAdministrator
		global.Startup = &settings.StartupPreferences{RunAtLogon: preview.Preferences.RunAtLogon, StartMinimized: preview.Preferences.StartMinimized, PreferAdministrator: preferAdministrator}
	}
	if err = settings.Validate(global); err != nil {
		return empty, err
	}
	document.Values["host"], err = json.Marshal(global)
	if err != nil {
		return empty, err
	}
	if request.Global {
		tray, err := preferences.Decode(document.Values["wintray"])
		if err != nil {
			return empty, err
		}
		tray.RunAtLogon = preview.Preferences.RunAtLogon
		tray.StartMinimizedToTray = preview.Preferences.StartMinimized
		if global.Language != "system" {
			tray.Language = global.Language
		}
		document.Values["wintray"], err = json.Marshal(tray)
		if err != nil {
			return empty, err
		}
	}
	if selected["capture"] {
		var capture capturePreferences
		if err = json.Unmarshal(document.Values["capture"], &capture); err != nil {
			return empty, err
		}
		capture.Theme = global.Theme
		document.Values["capture"], err = json.Marshal(capture)
		if err != nil {
			return empty, err
		}
	}
	saved, err := store.CommitDocument(document)
	if err != nil {
		return empty, err
	}
	return profileApplyResult{Revision: saved.Revision, Modules: append([]string{}, request.Modules...)}, nil
}

func mergeProfilePatch(current, patch json.RawMessage) (json.RawMessage, error) {
	var values, changes map[string]json.RawMessage
	if err := json.Unmarshal(current, &values); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(patch, &changes); err != nil {
		return nil, err
	}
	if values == nil || changes == nil {
		return nil, errors.New("配置合并内容无效")
	}
	for key, value := range changes {
		values[key] = value
	}
	return json.Marshal(values)
}

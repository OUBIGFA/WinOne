//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"math"
	"os"
	"path/filepath"
	"time"
	"winone/internal/captureprefs"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/settings"
)

// Mirrors the C# worker's versioned preferences, without image/history metadata.
type capturePreferences struct {
	SchemaVersion    int      `json:"schemaVersion"`
	Revision         uint64   `json:"revision"`
	OutputDirectory  string   `json:"outputDirectory"`
	CopyToClipboard  bool     `json:"copyToClipboard"`
	AskWhereToSave   bool     `json:"askWhereToSave"`
	OpenEditor       bool     `json:"openEditor"`
	PinAfterCapture  bool     `json:"pinAfterCapture"`
	Theme            string   `json:"theme"`
	ScrollDelay      int      `json:"scrollDelay"`
	ScrollFromTop    bool     `json:"scrollFromTop"`
	ToolbarTools     []string `json:"toolbarTools"`
	ToolbarVersion   int      `json:"toolbarVersion"`
	FileNameTemplate string   `json:"fileNameTemplate"`
}

func (value capturePreferences) validate() error {
	if value.ToolbarVersion != 0 && value.ToolbarVersion != 2 {
		return errors.New("不支持的截图工具栏版本")
	}
	if _, err := captureprefs.FileName(value.FileNameTemplate, time.Now(), 1); err != nil {
		return err
	}
	if value.SchemaVersion != 1 || value.Revision > math.MaxInt64 {
		return errors.New("不支持的截图配置版本")
	}
	if !filepath.IsAbs(value.OutputDirectory) {
		return errors.New("保存目录必须为绝对路径")
	}
	if value.Theme != "system" && value.Theme != "light" && value.Theme != "dark" {
		return errors.New("无效的主题")
	}
	if value.ScrollDelay < 100 || value.ScrollDelay > 2000 {
		return errors.New("滚动间隔须为 100–2000 毫秒")
	}
	return captureprefs.ValidateToolbar(value.ToolbarTools)
}

// Old custom layouts adopt the two new common actions once. Version 2 then
// preserves any explicit choice to move them into the overflow menu.
func (value capturePreferences) upgradeToolbar() capturePreferences {
	if value.ToolbarVersion == 0 {
		if value.ToolbarTools != nil {
			value.ToolbarTools = append([]string{}, value.ToolbarTools...)
			for _, action := range []string{"ocr", "translate"} {
				found := false
				for _, tool := range value.ToolbarTools {
					if tool == action {
						found = true
					}
				}
				if !found {
					value.ToolbarTools = append(value.ToolbarTools, action)
				}
			}
		}
		value.ToolbarVersion = 2
	}
	return value
}

func captureConfiguration(directory string, worker func() *modules.Lazy) configuration.ModuleAdapter {
	moduleDirectory := filepath.Join(directory, "modules", "capture")
	return configuration.ModuleAdapter{
		ID: "capture",
		Prepare: func(raw json.RawMessage, global settings.Settings) (json.RawMessage, error) {
			var value capturePreferences
			if err := configjson.DecodeObject(raw, &value); err != nil {
				return nil, err
			}
			value.Theme = global.Theme
			value = value.upgradeToolbar()
			return json.Marshal(value)
		},
		Load: func() (json.RawMessage, error) {
			// Known Folders follows Windows folder redirection, including relocated Pictures.
			pictures, err := windows.KnownFolderPath(windows.FOLDERID_Pictures, windows.KF_FLAG_DONT_VERIFY)
			if err != nil {
				return nil, err
			}
			value := capturePreferences{SchemaVersion: 1, OutputDirectory: filepath.Join(pictures, "WinOne"), CopyToClipboard: true, OpenEditor: true, Theme: "system", ScrollDelay: 300}
			data, err := os.ReadFile(filepath.Join(moduleDirectory, "settings.json"))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			if err == nil {
				if err = configjson.DecodeObject(data, &value); err != nil {
					return nil, err
				}
			}
			value.Revision = 0
			value = value.upgradeToolbar()
			if err = value.validate(); err != nil {
				return nil, err
			}
			return json.Marshal(value)
		},
		Validate: func(raw json.RawMessage) error {
			var value capturePreferences
			if err := configjson.DecodeObject(raw, &value); err != nil {
				return err
			}
			if value.Revision != 0 {
				return errors.New("截图修订号须由 Host 管理")
			}
			return value.validate()
		},
		Apply: func(raw json.RawMessage, revision uint64) error {
			var value capturePreferences
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}
			value.Revision = revision
			value = value.upgradeToolbar()
			if err := value.validate(); err != nil {
				return err
			}
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err = settings.WriteAtomic(filepath.Join(moduleDirectory, "managed-settings.json"), data); err != nil {
				return err
			}
			if runtime := worker(); runtime != nil {
				response, called := runtime.HandleRunning(host.Request{Version: 1, ID: "configuration-refresh", Method: "settings.reload"})
				if called && response.Error != nil {
					return errors.New(response.Error.Message)
				}
			}
			return nil
		},
	}
}

func captureConfigurationHandler(store *configuration.HostStore, next func(host.Request) host.Response) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		if request.Method == "filename.preview" {
			var params struct {
				Template *string `json:"template"`
			}
			var name string
			err := configjson.DecodeObject(request.Params, &params)
			if err == nil && params.Template == nil {
				var raw json.RawMessage
				raw, _, err = store.Module("capture")
				var value capturePreferences
				if err == nil {
					err = json.Unmarshal(raw, &value)
					params.Template = &value.FileNameTemplate
				}
			}
			if err == nil {
				name, err = captureprefs.FileName(*params.Template, time.Now(), 1)
			}
			if err != nil {
				return host.Response{ID: request.ID, Error: &host.RPCError{Code: "configuration", Message: err.Error()}}
			}
			result, _ := json.Marshal(map[string]string{"name": name})
			return host.Response{ID: request.ID, Result: result}
		}
		if request.Method != "settings.save" && request.Method != "settings.get" {
			return next(request)
		}
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "configuration", Message: err.Error()}}
		}
		raw, revision, err := store.Module("capture")
		if err != nil {
			return fail(err)
		}
		if request.Method == "settings.save" {
			var value capturePreferences
			if err = configjson.DecodeObject(request.Params, &value); err != nil {
				return fail(err)
			}
			expected := value.Revision
			value.Revision = 0
			raw, err = json.Marshal(value)
			if err != nil {
				return fail(err)
			}
			raw, revision, err = store.SaveModule("capture", expected, raw)
			if err != nil {
				return fail(err)
			}
		}
		var value capturePreferences
		if err = json.Unmarshal(raw, &value); err != nil {
			return fail(err)
		}
		value.Revision = revision
		value = value.upgradeToolbar()
		data, err := json.Marshal(value)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: data}
	}
}

//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulestate"
	"winone/internal/previewtransfer"
	"winone/internal/profiles"
	"winone/internal/protocol"
	"winone/internal/settings"
	"winone/modules/wintray/preferences"
)

// collectProfile reads a single committed configuration, with stable module
// intent on either side. It never queries or activates feature processes.
func collectProfile(store *configuration.HostStore, states *modulestate.Store, name string) (profiles.Archive, error) {
	before := states.Snapshot()
	document := store.CommittedDocument()
	after := states.Snapshot()
	if before.Revision != after.Revision {
		return profiles.Archive{}, errors.New("模块状态已变更，请重新导出")
	}
	var global settings.Settings
	if err := json.Unmarshal(document.Values["host"], &global); err != nil {
		return profiles.Archive{}, err
	}
	tray, err := preferences.Decode(document.Values["wintray"])
	if err != nil {
		return profiles.Archive{}, err
	}
	archive := profiles.Archive{
		CreatedBy:   "0.1.0-alpha.1",
		Preferences: profiles.Preferences{Name: name, Theme: global.Theme, Language: tray.Language, RunAtLogon: tray.RunAtLogon, StartMinimized: tray.StartMinimizedToTray, Enabled: map[string]bool{}, Shortcuts: map[string]string{"capture.region": global.CaptureShortcut, "always-on-top.toggle": global.PinShortcut}},
		Modules:     map[string]profiles.ModuleSettings{}, Paths: map[string]profiles.PathReference{},
	}
	if global.Language != "" {
		archive.Preferences.Language = global.Language
	}
	for _, manifest := range catalog.Builtins() {
		raw, exists := document.Values[manifest.ID]
		if !exists {
			return profiles.Archive{}, errors.New("基础模块配置尚未迁入，无法完整导出")
		}
		exported, err := profiles.ExportBuiltinSettings(manifest.ID, raw, profiles.ExportOptions{})
		if err != nil {
			return profiles.Archive{}, err
		}
		archive.Modules[manifest.ID] = exported.Module
		for key, ref := range exported.Paths {
			archive.Paths[key] = ref
		}
		if document.ModuleEnabled != nil {
			archive.Preferences.Enabled[manifest.ID] = document.ModuleEnabled[manifest.ID]
		} else {
			archive.Preferences.Enabled[manifest.ID] = before.Modules[manifest.ID].Enabled
		}
	}
	for id, raw := range document.DeferredModules {
		var deferred deferredProfileModule
		if err := configjson.DecodeObject(raw, &deferred); err != nil {
			return profiles.Archive{}, err
		}
		if _, exists := archive.Modules[id]; exists {
			return profiles.Archive{}, errors.New("待安装模块与当前模块配置冲突")
		}
		archive.Modules[id] = deferred.Module
		if deferred.Enabled != nil {
			archive.Preferences.Enabled[id] = *deferred.Enabled
		}
		for action, key := range deferred.Shortcuts {
			archive.Preferences.Shortcuts[action] = key
		}
		for key, ref := range deferred.Paths {
			if existing, exists := archive.Paths[key]; exists {
				if existing.Kind != ref.Kind || existing.Hint != ref.Hint {
					return profiles.Archive{}, errors.New("待安装模块的路径引用与当前配置冲突")
				}
			} else {
				archive.Paths[key] = ref
			}
		}
	}
	return archive, nil
}

// limitProfilePreviewGeneration bounds decompression and serialization to one
// request. Reads and cleanup remain available while another preview is built.
func limitProfilePreviewGeneration(next func(host.Request) host.Response) func(host.Request) host.Response {
	var generation sync.Mutex
	return func(request host.Request) host.Response {
		if request.Method == "import.preview" || request.Method == "import.preview.start" {
			if !generation.TryLock() {
				return host.Response{ID: request.ID, Error: &host.RPCError{Code: "profile_busy", Message: "正在生成配置预览，请稍后重试"}}
			}
			defer generation.Unlock()
		}
		return next(request)
	}
}

func profileHandler(store *configuration.HostStore, states *modulestate.Store) func(host.Request) host.Response {
	previews := previewtransfer.NewStore(90 * time.Second)
	return limitProfilePreviewGeneration(func(request host.Request) host.Response {
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "profile", Message: err.Error()}}
		}
		if request.Method == "import.preview.read" || request.Method == "import.preview.release" {
			var args struct {
				Token  string `json:"token"`
				Offset int    `json:"offset"`
			}
			if err := configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			if request.Method == "import.preview.release" {
				previews.Release(args.Token)
				return host.Response{ID: request.ID, Result: json.RawMessage(`{"released":true}`)}
			}
			chunk, err := previews.Read(args.Token, args.Offset)
			if err != nil {
				return fail(err)
			}
			data, err := json.Marshal(chunk)
			if err != nil {
				return fail(err)
			}
			return host.Response{ID: request.ID, Result: data}
		}
		if request.Method == "import.apply" {
			var args profileApplyRequest
			if err := configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			result, err := applyProfile(args, store, states)
			if err != nil {
				return fail(err)
			}
			data, err := json.Marshal(result)
			if err != nil {
				return fail(err)
			}
			return host.Response{ID: request.ID, Result: data}
		}
		if request.Method == "import.preview" || request.Method == "import.preview.start" {
			var args struct {
				Path     string            `json:"path"`
				Bindings map[string]string `json:"bindings"`
			}
			if err := configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			preview, err := previewProfileFile(args.Path, args.Bindings, store, states)
			if err != nil {
				return fail(err)
			}
			data, err := json.Marshal(preview)
			if err != nil {
				return fail(err)
			}
			if request.Method == "import.preview.start" {
				descriptor, err := previews.Own(data)
				if err != nil {
					return fail(err)
				}
				data, err = json.Marshal(descriptor)
				if err != nil {
					return fail(err)
				}
			} else if len(data) > protocol.MaxFrameSize/2 {
				return fail(errors.New("预览内容较大，请使用分块预览接口"))
			}
			return host.Response{ID: request.ID, Result: data}
		}
		if request.Method != "export" {
			return fail(errors.New("不支持的配置档案操作"))
		}
		var args struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if err := configjson.DecodeObject(request.Params, &args); err != nil {
			return fail(err)
		}
		if !filepath.IsAbs(args.Path) || !strings.EqualFold(filepath.Ext(args.Path), ".winone") {
			return fail(errors.New("请选择 .winone 配置文件保存位置"))
		}
		archive, err := collectProfile(store, states, args.Name)
		if err != nil {
			return fail(err)
		}
		data, err := profiles.Encode(archive)
		if err != nil {
			return fail(err)
		}
		if err = settings.WriteAtomic(args.Path, data); err != nil {
			return fail(err)
		}
		result, _ := json.Marshal(map[string]any{"path": args.Path, "bytes": len(data)})
		return host.Response{ID: request.ID, Result: result}
	})
}

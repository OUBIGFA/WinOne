//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/settings"
	"winone/modules/wintray/preferences"
)

type trayConfigurationSnapshot struct {
	Revision             string               `json:"revision"`
	Settings             preferences.Settings `json:"settings"`
	RecoveryError        string               `json:"recoveryError"`
	BackgroundError      string               `json:"backgroundError"`
	StartupError         string               `json:"startupError"`
	ProcessID            uint32               `json:"processId"`
	ConfigurationManaged bool                 `json:"configurationManaged"`
	InitializationState  string               `json:"initializationState"`
}

// A native reload is an asynchronous job. Never equate an accepted request with
// completion, and never cold-start a disabled module while saving preferences.
func runExistingTrayJob(runtime *modules.Lazy, method string, params json.RawMessage) (bool, error) {
	response, called := runtime.HandleRunning(host.Request{Version: 1, ID: "configuration-native", Method: method, Params: params})
	if !called {
		return false, nil
	}
	if response.Error != nil {
		return true, errors.New(response.Error.Message)
	}
	var operation struct {
		ID    string `json:"id"`
		State string `json:"state"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Result, &operation); err != nil {
		return true, err
	}
	for operation.State == "running" {
		if operation.ID == "" {
			return true, errors.New("程序配置任务缺少标识")
		}
		time.Sleep(150 * time.Millisecond)
		data, _ := json.Marshal(map[string]string{"id": operation.ID})
		response, called = runtime.HandleRunning(host.Request{Version: 1, ID: "configuration-native-poll", Method: "jobs.get", Params: data})
		if !called {
			return true, errors.New("程序配置任务尚未确认，Worker 已退出")
		}
		if response.Error != nil {
			return true, errors.New(response.Error.Message)
		}
		if err := json.Unmarshal(response.Result, &operation); err != nil {
			return true, err
		}
	}
	if operation.State != "completed" {
		if operation.Error != "" {
			return true, errors.New(operation.Error)
		}
		return true, errors.New("程序配置任务未完成")
	}
	return true, nil
}

func wintrayConfiguration(directory string, get func() *modules.Lazy, mayStart ...func() bool) configuration.ModuleAdapter {
	moduleDirectory := filepath.Join(directory, "modules", "wintray")
	pendingNative := false // Apply calls are serialized by the configuration store.
	return configuration.ModuleAdapter{
		ID: "wintray",
		Prepare: func(raw json.RawMessage, global settings.Settings) (json.RawMessage, error) {
			value, err := preferences.Decode(raw)
			if err != nil {
				return nil, err
			}
			if global.Language != "" {
				value.Language = applicationLanguage(global.Language)
			}
			if global.Startup != nil {
				value.RunAtLogon = global.Startup.RunAtLogon
				value.StartMinimizedToTray = global.Startup.StartMinimized
			}
			return json.Marshal(value)
		},
		Load: func() (json.RawMessage, error) {
			source := filepath.Join(moduleDirectory, "settings.json")
			if _, err := os.Stat(source + ".transaction.json"); !errors.Is(err, os.ErrNotExist) {
				if err != nil {
					return nil, err
				}
				return nil, errors.New("请先恢复旧版程序设置事务")
			}
			value := preferences.Defaults()
			data, err := os.ReadFile(source)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			if err == nil {
				value, err = preferences.DecodeLegacy(data)
				if err != nil {
					return nil, err
				}
			}
			return json.Marshal(value)
		},
		Validate: func(raw json.RawMessage) error { _, err := preferences.Decode(raw); return err },
		Apply: func(raw json.RawMessage, revision uint64) error {
			value, err := preferences.Decode(raw)
			if err != nil {
				return err
			}
			previous, readErr := readWinTrayProjection(filepath.Join(moduleDirectory, "managed-settings.json"))
			needsNative := readErr != nil || previous.RunAtLogon != value.RunAtLogon || ((len(previous.ManagedApps) > 0 || len(value.ManagedApps) > 0) && !reflect.DeepEqual(previous, value))
			data, err := json.Marshal(struct {
				Revision uint64               `json:"revision"`
				Settings preferences.Settings `json:"settings"`
			}{revision, value})
			if err != nil {
				return err
			}
			if err = settings.WriteAtomic(filepath.Join(moduleDirectory, "managed-settings.json"), data); err != nil {
				return err
			}
			runtime := get()
			if runtime == nil {
				if pendingNative {
					return errors.New("程序配置仍需恢复")
				}
				return nil
			}
			if !runtime.Running() && len(mayStart) > 0 && mayStart[0]() && (needsNative || pendingNative) {
				pendingNative = true
				runtime.SuppressAutorun()
				if err = startWinTray(context.Background(), runtime); err != nil {
					return err
				}
			}
			called, err := runExistingTrayJob(runtime, "settings.reload", nil)
			if called {
				pendingNative = err != nil
			} else if pendingNative {
				return errors.New("程序配置仍需恢复")
			}
			return err
		},
	}
}

func wintrayConfigurationHandler(directory string, store *configuration.HostStore, get func() *modules.Lazy, next func(host.Request) host.Response, enabled ...func() bool) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		if request.Method == "import.preview" {
			var args struct {
				Path string `json:"path"`
			}
			if err := configjson.DecodeObject(request.Params, &args); err != nil {
				return host.Response{ID: request.ID, Error: &host.RPCError{Code: "configuration", Message: err.Error()}}
			}
			preview, err := preferences.PreviewImport(args.Path)
			if err != nil {
				return host.Response{ID: request.ID, Error: &host.RPCError{Code: "configuration", Message: err.Error()}}
			}
			data, err := json.Marshal(preview)
			if err != nil {
				return host.Response{ID: request.ID, Error: &host.RPCError{Code: "configuration", Message: err.Error()}}
			}
			return host.Response{ID: request.ID, Result: data}
		}
		if request.Method == "icons" {
			if len(enabled) > 0 && !enabled[0]() {
				return next(request)
			}
			if runtime := get(); runtime != nil {
				if response, called := runtime.HandleRunning(request); called {
					return response
				}
			}
			return host.Response{ID: request.ID, Result: json.RawMessage(`[]`)}
		}
		isSnapshot := request.Method == "snapshot"
		if request.Method != "settings.save" && request.Method != "settings.get" && !isSnapshot {
			return next(request)
		}
		if isSnapshot && len(enabled) > 0 && !enabled[0]() {
			return next(request)
		}
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "configuration", Message: err.Error()}}
		}
		raw, revision, err := store.Module("wintray")
		// Unmigrated profiles retain their recovery route until the old transaction is resolved.
		if err != nil {
			if request.Method == "settings.save" || isSnapshot {
				return next(request)
			}
			return fail(err)
		}
		if request.Method == "settings.save" {
			var args struct {
				Revision string          `json:"revision"`
				Settings json.RawMessage `json:"settings"`
				History  json.RawMessage `json:"history,omitempty"`
			}
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			expected, parseErr := strconv.ParseUint(args.Revision, 10, 64)
			if parseErr != nil || expected != revision {
				return fail(configuration.ErrConflict)
			}
			if _, err = preferences.Decode(args.Settings); err != nil {
				return fail(err)
			}
			if len(args.History) > 0 && string(args.History) != "null" {
				if directory == "" {
					return fail(errors.New("运行历史目录不可用"))
				}
				if err = preferences.MergeHistory(filepath.Join(directory, "modules", "wintray"), args.History); err != nil {
					return fail(err)
				}
			}
			raw, revision, err = store.SaveModule("wintray", expected, args.Settings)
			if err != nil {
				return fail(err)
			}
		}
		value, err := preferences.Decode(raw)
		if err != nil {
			return fail(err)
		}
		if isSnapshot {
			if runtime := get(); runtime != nil {
				response, called := runtime.HandleRunning(request)
				if called {
					if response.Error != nil {
						return response
					}
					var fields map[string]json.RawMessage
					if err = json.Unmarshal(response.Result, &fields); err != nil {
						return fail(err)
					}
					fields["settings"] = raw
					fields["revision"], err = json.Marshal(strconv.FormatUint(revision, 10))
					if err != nil {
						return fail(err)
					}
					response.Result, err = json.Marshal(fields)
					if err != nil {
						return fail(err)
					}
					return response
				}
				if state := runtime.Status(); state.State == "faulted" {
					return fail(errors.New(state.Detail))
				}
			}
		}
		snapshot := trayConfigurationSnapshot{Revision: strconv.FormatUint(revision, 10), Settings: value, ConfigurationManaged: true, InitializationState: "idle"}
		var result any = snapshot
		if request.Method == "settings.save" {
			result = map[string]any{"state": "completed", "result": snapshot}
		}
		data, err := json.Marshal(result)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: data}
	}
}

func readWinTrayProjection(path string) (preferences.Settings, error) {
	file, err := os.Open(path)
	if err != nil {
		return preferences.Settings{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil {
		return preferences.Settings{}, err
	}
	if len(data) > 2<<20 {
		return preferences.Settings{}, errors.New("程序配置过大")
	}
	var projection struct {
		Revision uint64          `json:"revision"`
		Settings json.RawMessage `json:"settings"`
	}
	if err = configjson.DecodeObject(data, &projection); err != nil {
		return preferences.Settings{}, err
	}
	return preferences.Decode(projection.Settings)
}

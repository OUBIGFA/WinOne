//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/settings"
	"winone/modules/always-on-top/engine"
)

func aotConfiguration(directory string, worker func() *modules.Lazy) configuration.ModuleAdapter {
	moduleDirectory := filepath.Join(directory, "modules", "always-on-top")
	return configuration.ModuleAdapter{
		ID: "always-on-top",
		Load: func() (json.RawMessage, error) {
			value := engine.Defaults()
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
			value.Enabled = true
			if err = value.Validate(); err != nil {
				return nil, err
			}
			return json.Marshal(value)
		},
		Validate: func(raw json.RawMessage) error {
			var value engine.Settings
			if err := configjson.DecodeObject(raw, &value); err != nil {
				return err
			}
			if value.Revision != 0 || !value.Enabled {
				return errors.New("置顶开关和修订号须由 Host 管理")
			}
			return value.Validate()
		},
		Apply: func(raw json.RawMessage, revision uint64) error {
			var value engine.Settings
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}
			value.Revision = revision
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

// Preference operations bypass activation only; feature actions remain gated.
func aotConfigurationHandler(store *configuration.HostStore, next func(host.Request) host.Response) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		if request.Method != "settings.save" && request.Method != "settings.get" {
			return next(request)
		}
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "configuration", Message: err.Error()}}
		}
		raw, revision, err := store.Module("always-on-top")
		if err != nil {
			return fail(err)
		}
		if request.Method == "settings.save" {
			var value engine.Settings
			if err = configjson.DecodeObject(request.Params, &value); err != nil {
				return fail(err)
			}
			var legacy struct {
				Enabled *bool `json:"enabled"`
			}
			if err = json.Unmarshal(request.Params, &legacy); err != nil {
				return fail(err)
			}
			if legacy.Enabled != nil && !*legacy.Enabled {
				return fail(errors.New("请通过模块管理关闭窗口置顶"))
			}
			expected := value.Revision
			value.Revision = 0
			value.Enabled = true
			raw, err = json.Marshal(value)
			if err != nil {
				return fail(err)
			}
			raw, revision, err = store.SaveModule("always-on-top", expected, raw)
			if err != nil {
				return fail(err)
			}
		}
		var value engine.Settings
		if err = json.Unmarshal(raw, &value); err != nil {
			return fail(err)
		}
		value.Revision = revision
		data, err := json.Marshal(value)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: data}
	}
}

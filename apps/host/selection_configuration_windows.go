//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/selection"
	"winone/internal/settings"
)

func selectionConfiguration(directory string, runtime func() *selectionRuntime) configuration.ModuleAdapter {
	return configuration.ModuleAdapter{
		ID:   "selection",
		Load: func() (json.RawMessage, error) { return json.Marshal(selection.Defaults()) },
		Validate: func(raw json.RawMessage) error {
			var value selection.Preferences
			if err := configjson.DecodeObject(raw, &value); err != nil {
				return err
			}
			if value.Revision != 0 {
				return errors.New("划词设置修订号须由 Host 管理")
			}
			return value.Validate()
		},
		Apply: func(raw json.RawMessage, revision uint64) error {
			var value selection.Preferences
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}
			value.Revision = revision
			value.Actions = value.EffectiveActions()
			value.ToolbarVersion = 1
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err = settings.WriteAtomic(filepath.Join(directory, "modules", "selection", "managed-settings.json"), data); err != nil {
				return err
			}
			if runtime != nil {
				if current := runtime(); current != nil {
					current.configure(value)
				}
			}
			return nil
		},
	}
}
func selectionConfigurationHandler(store *configuration.HostStore, next func(host.Request) host.Response) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		if request.Method != "settings.get" && request.Method != "settings.save" {
			return next(request)
		}
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "configuration", Message: err.Error()}}
		}
		raw, revision, err := store.Module("selection")
		if err != nil {
			return fail(err)
		}
		if request.Method == "settings.save" {
			var value selection.Preferences
			if err = configjson.DecodeObject(request.Params, &value); err != nil {
				return fail(err)
			}
			expected := value.Revision
			value.Revision = 0
			raw, err = json.Marshal(value)
			if err != nil {
				return fail(err)
			}
			raw, revision, err = store.SaveModule("selection", expected, raw)
			if err != nil {
				return fail(err)
			}
		}
		var value selection.Preferences
		if err = json.Unmarshal(raw, &value); err != nil {
			return fail(err)
		}
		value.Revision = revision
		value.Actions = value.EffectiveActions()
		value.ToolbarVersion = 1
		data, err := json.Marshal(value)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: data}
	}
}

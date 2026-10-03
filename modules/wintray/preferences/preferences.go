// Package preferences exposes the data contract without importing the native UI.
package preferences

import (
	"encoding/json"
	"winone/internal/configjson"
	"winone/modules/wintray/internal/config"
)

type Settings = config.Settings

func Defaults() Settings {
	value := config.DefaultSettings()
	value.RunAtLogon = false
	value.ExitAfterManagedAppsCompleted = false
	return value
}
func Decode(raw json.RawMessage) (Settings, error) {
	var value Settings
	if err := configjson.DecodeObject(raw, &value); err != nil {
		return value, err
	}
	return value, config.Validate(value)
}
func DecodeLegacy(raw json.RawMessage) (Settings, error) {
	value, err := config.DecodeImport(raw)
	if err != nil {
		return value, err
	}
	return value, config.Validate(value)
}

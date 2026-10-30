package profiles

import (
	"encoding/json"
	"errors"
	"winone/internal/timedpower"
)

// Only a reusable template crosses devices; armed tasks and their deadlines
// are never part of Preferences and therefore cannot be imported or resumed.
func exportTimedPower(raw json.RawMessage) (ExportedSettings, error) {
	var p timedpower.Preferences
	if len(raw) > MaxEntryBytes {
		return ExportedSettings{}, errors.New("模块配置过大")
	}
	if err := strictJSON(raw, &p); err != nil {
		return ExportedSettings{}, err
	}
	if err := p.Validate(); err != nil {
		return ExportedSettings{}, err
	}
	p.Revision = 0
	encoded, err := json.Marshal(p)
	return ExportedSettings{Module: ModuleSettings{SchemaVersion: 1, Settings: encoded}, Paths: map[string]PathReference{}, OmittedArguments: []string{}}, err
}
func importTimedPower(source map[string]json.RawMessage) (map[string]any, error) {
	raw, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	var p timedpower.Preferences
	if err = strictJSON(raw, &p); err != nil {
		return nil, err
	}
	if p.Revision != 0 {
		return nil, errors.New("定时关机模板不能带运行修订号")
	}
	if err = p.Validate(); err != nil {
		return nil, err
	}
	var result map[string]any
	err = json.Unmarshal(raw, &result)
	return result, err
}

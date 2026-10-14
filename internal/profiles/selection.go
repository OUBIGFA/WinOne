package profiles

import (
	"encoding/json"
	"errors"
	"winone/internal/resultwindow"
	"winone/internal/selection"
)

// Only executable names and user preferences migrate. Selection contents,
// window identities and all AI credentials remain outside profile archives.
func selectionFields(source map[string]json.RawMessage, strict bool) (map[string]any, error) {
	var values map[string]any
	var err error
	if strict {
		nested := []string{"excludedApps"}
		if _, exists := source["actions"]; exists {
			nested = append(nested, "actions")
		}
		values, err = exactFields(source, "automatic:b keyboard:b", nested...)
	} else {
		values, err = projectFields(source, "automatic:b keyboard:b")
	}
	if err != nil {
		return nil, err
	}
	var apps []string
	if err = json.Unmarshal(source["excludedApps"], &apps); err != nil || apps == nil {
		return nil, errors.New("划词排除应用列表无效")
	}
	var actions []resultwindow.SelectionAction
	if raw, exists := source["actions"]; exists {
		if err = json.Unmarshal(raw, &actions); err != nil || actions == nil {
			return nil, errors.New("工具栏功能列表无效")
		}
		values["actions"] = actions
	}
	if err = (selection.Preferences{SchemaVersion: 1, ExcludedApps: apps, Actions: actions}).Validate(); err != nil {
		return nil, err
	}
	values["excludedApps"] = apps
	return values, nil
}

package profiles

import (
	"encoding/json"
	"errors"
	"winone/internal/resultwindow"
)

func quickAssistantFields(source map[string]json.RawMessage, strict bool) (map[string]any, error) {
	fields := ""
	if _, ok := source["learningVersion"]; ok {
		fields = "learningVersion:n"
	}
	values, err := projectFields(source, fields)
	if err != nil {
		return nil, err
	}
	if fields != "" {
		if err = numberRange(values, "learningVersion", 0, 1); err != nil {
			return nil, err
		}
	}
	if strict {
		if _, err := exactFields(source, fields, "actions"); err != nil {
			return nil, err
		}
	}
	var actions []resultwindow.SelectionAction
	if err := json.Unmarshal(source["actions"], &actions); err != nil || actions == nil {
		return nil, errors.New("快捷助手功能列表无效")
	}
	if err := resultwindow.ValidateActions(actions); err != nil {
		return nil, err
	}
	values["actions"] = actions
	return values, nil
}

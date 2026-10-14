package profiles

import (
	"encoding/json"
	"errors"
	"winone/internal/resultwindow"
)

func quickAssistantFields(source map[string]json.RawMessage, strict bool) (map[string]any, error) {
	if strict {
		if _, err := exactFields(source, "", "actions"); err != nil {
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
	return map[string]any{"actions": actions}, nil
}

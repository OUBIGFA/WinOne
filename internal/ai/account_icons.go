package ai

import (
	"encoding/json"
	"errors"
)

// WithAccountIcons decorates public catalog/list metadata while retaining all
// account fields. Credential discovery and sign-in still belong to magpie.
func WithAccountIcons(operation, defaultAgent string, output json.RawMessage) (json.RawMessage, error) {
	field, key := "agents", "id"
	if operation == "accounts.list" {
		field, key = "accounts", "agent"
	} else if operation != "accounts.catalog" {
		return output, nil
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(output, &body) != nil {
		return nil, errors.New("账号列表响应无效")
	}
	raw, ok := body[field]
	if !ok {
		return nil, errors.New("账号列表字段缺失")
	}
	var entries []map[string]json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return nil, errors.New("账号列表内容无效")
	}
	for _, entry := range entries {
		agent := defaultAgent
		if value, ok := entry[key]; ok {
			if json.Unmarshal(value, &agent) != nil {
				return nil, errors.New("账号类型无效")
			}
		}
		if entry == nil {
			return nil, errors.New("账号项目无效")
		}
		entry["icon"], _ = json.Marshal(AccountIcon(agent))
	}
	body[field], _ = json.Marshal(entries)
	return json.Marshal(body)
}

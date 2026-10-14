//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type ModelOption struct {
	Metadata ModelMetadata `json:"metadata,omitempty"`
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Icon     string        `json:"icon,omitempty"`
	Free     bool          `json:"free,omitempty"`
}

func (t Translator) Models(ctx context.Context, p Provider, key string) ([]ModelOption, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	input := map[string]any{"operation": "models.fetch", "protocol": p.Protocol, "baseUrl": p.BaseURL, "key": key}
	input["headers"] = p.Headers
	if p.Preset != "" {
		input["preset"] = p.Preset
		input["presetRegion"] = p.PresetRegion
	}
	if p.AuthMode == "account" {
		input["agent"] = p.Agent
		input["agentBackend"] = p.AgentBackend
	}
	output, err := t.ForProvider(p.ID).invoke(ctx, input)
	var result struct {
		Models            []ModelOption `json:"models"`
		Error             string        `json:"error"`
		Status            int           `json:"status"`
		RetryAfterSeconds int           `json:"retryAfterSeconds"`
	}
	decodeErr := json.Unmarshal(output, &result)
	if decodeErr == nil && result.Error != "" && result.Status >= 400 && result.Status <= 599 && result.RetryAfterSeconds >= 0 && result.RetryAfterSeconds <= 86400 {
		return nil, &AttemptFailure{Status: result.Status, RetryAfter: time.Duration(result.RetryAfterSeconds) * time.Second}
	}
	if err != nil || decodeErr != nil || result.Error != "" {
		return nil, errors.New("无法获取模型列表，可以手动添加模型")
	}
	if len(result.Models) > 2048 {
		return nil, errors.New("供应商模型列表过大")
	}
	for i := range result.Models {
		result.Models[i].Icon = (Provider{ModelNames: map[string]string{result.Models[i].ID: result.Models[i].Name}}).ModelIcon(result.Models[i].ID)
	}
	return result.Models, nil
}

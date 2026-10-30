//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type UsageQuery struct {
	Models    []string `json:"models,omitempty"`
	Zone      string   `json:"tz,omitempty"`
	CallerKey string   `json:"callerKey,omitempty"`
	Period    string   `json:"period"`
	Provider  string   `json:"provider"`
	Source    string   `json:"source"`
	Failed    bool     `json:"failed"`
	Offset    int      `json:"offset"`
	Day       string   `json:"day"`
	Model     string   `json:"model"`
	Account   string   `json:"account"`
	Purpose   string   `json:"purpose"`
	Query     string   `json:"query"`
	Export    bool     `json:"export,omitempty"`
}

// Usage reads only the shared installation profile's Magpie ledger. Provider
// filters are metadata, never paths supplied by the settings window.
func (t Translator) Usage(ctx context.Context, query UsageQuery) (json.RawMessage, error) {
	if _, _, err := UsagePeriodRange(query.Period, time.Now()); err != nil {
		return nil, err
	}
	if query.Offset < 0 || query.Offset > 1000000 || len(query.Provider) > 256 || (query.Source != "" && query.Source != "winone" && query.Source != "gateway") {
		return nil, errors.New("用量筛选条件无效")
	}
	if len(query.Zone) > 128 || strings.ContainsRune(query.Zone, '\x00') || len(query.Model) > 512 || len(query.Account) > 256 || len(query.Purpose) > 128 || len(query.CallerKey) > 128 || len(query.Query) > 512 || strings.ContainsRune(query.Query, '\x00') {
		return nil, errors.New("用量筛选条件无效")
	}
	if query.Day != "" {
		if _, err := time.Parse("2006-01-02", query.Day); err != nil {
			return nil, errors.New("用量日期无效")
		}
	}
	for _, model := range query.Models {
		if len(model) > 512 || strings.ContainsRune(model, '\x00') {
			return nil, errors.New("用量模型标识无效")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	maximum := int64(1 << 20)
	if query.Export {
		maximum = 65 << 20
	}
	raw, err := t.invokeBounded(ctx, map[string]any{"operation": "usage.read", "usage": query}, maximum)
	if err != nil {
		return nil, errors.New("用量统计读取失败，请刷新重试")
	}
	var result struct {
		Rows json.RawMessage `json:"rows"`
		Sum  json.RawMessage `json:"sum"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Rows) == 0 || len(result.Sum) == 0 {
		return nil, errors.New("用量统计响应无效，请修复 AI 组件")
	}
	return raw, nil
}

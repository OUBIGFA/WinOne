//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// The Host scopes this translator to the existing provider's private home.
// Migration responses carry account outcomes, never the imported credentials.
func (t Translator) MoveAntigravityAccounts(ctx context.Context, p Provider) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	raw, err := t.invoke(ctx, map[string]any{"operation": "accounts.move-agy", "agent": "antigravity", "agentBackend": t.AccountBackend, "providerPreferences": p.ProviderPreferences, "models": p.Models, "headers": p.Headers})
	var state struct {
		Results json.RawMessage `json:"results"`
		Error   string          `json:"error"`
	}
	if json.Unmarshal(raw, &state) != nil {
		return nil, errors.New("账号迁移服务返回无效结果")
	}
	if strings.Contains(state.Error, "install the official client first") {
		return nil, errors.New("请先安装官方 Antigravity CLI（agy），然后重试迁移")
	}
	if err != nil || state.Error != "" || len(state.Results) == 0 {
		return nil, errors.New("账号迁移失败，请检查登录状态或客户端安装后重试")
	}
	return raw, nil
}

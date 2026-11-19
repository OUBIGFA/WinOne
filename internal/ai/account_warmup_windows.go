//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// WarmAccounts uses the selected saved provider's private account home. It
// creates no timer and leaves provider preferences in the existing save flow.
func (t Translator) WarmAccounts(ctx context.Context, p Provider, user string, all bool) (json.RawMessage, error) {
	if p.AuthMode != "account" || p.Agent != "codex" && p.Agent != "claude" || !all && (user == "" || len(user) > 320) {
		return nil, errors.New("请选择支持预热的已保存账号")
	}
	operation := "accounts.warm-now"
	if all {
		operation = "accounts.warm-all"
	}
	// WarmAll queues the original two-minute account requests in batches.
	// Its caller owns cancellation; only a single warm-up gets a total cap.
	if !all {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
	}
	raw, err := t.invoke(ctx, map[string]any{"operation": operation, "agent": p.Agent, "agentBackend": p.AgentBackend,
		"user": user, "providerPreferences": p.ProviderPreferences, "models": p.Models, "headers": p.Headers})
	var reply struct {
		Results json.RawMessage `json:"results"`
		Error   string          `json:"error"`
	}
	if err != nil || json.Unmarshal(raw, &reply) != nil || reply.Error != "" || len(reply.Results) == 0 {
		return nil, errors.New("账号预热未能完成，请检查登录、网络或官方客户端")
	}
	return raw, nil
}

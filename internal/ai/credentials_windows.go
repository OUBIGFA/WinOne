//go:build windows

package ai

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// AttemptFailure carries only bounded protocol metadata, never vendor bodies.
type AttemptFailure struct {
	Limited    bool // Magpie's quota, credit or rate classification; no response body.
	Status     int
	RetryAfter time.Duration
}

func (e *AttemptFailure) Error() string {
	return fmt.Sprintf("AI 服务请求失败（HTTP %d）", e.Status)
}

type credential struct {
	id, sealed string
	options    KeyOptions
}

// KeyPlan is an immutable request snapshot. It cannot be serialized as secrets.
type KeyPlan struct {
	provider string
	keys     []credential
	// A pinned disabled slot must remain visible to upstream's identity rules,
	// so it blocks that caller instead of silently selecting another key.
	pinnedDisabled []credential
	configured     bool
}

func (s *Store) keyPlanLocked(p Provider) KeyPlan {
	plan := KeyPlan{provider: p.ID}
	for _, slot := range p.Keys {
		sealed := s.sealedCredentialLocked(p.ID, slot.ID)
		if sealed == "" {
			continue
		}
		plan.configured = true
		if slot.Enabled {
			plan.keys = append(plan.keys, credential{id: slot.ID, sealed: sealed, options: slot.KeyOptions})
		} else {
			for _, pinned := range p.AgentAccounts {
				if pinned == slot.ID {
					plan.pinnedDisabled = append(plan.pinnedDisabled, credential{id: slot.ID, sealed: sealed, options: slot.KeyOptions})
					break
				}
			}
		}
	}
	return plan
}
func (plan KeyPlan) firstKey() (string, error) {
	if len(plan.keys) > 0 {
		return unsealKey(plan.keys[0].sealed)
	}
	if plan.configured {
		return "", errors.New("供应商没有启用的密钥")
	}
	return "", nil
}

// WithKeys opens one immutable credential snapshot. Magpie receives the whole
// enabled set and owns ordering, failures, cooldowns and per-key limits.
// The callback remains the private transport boundary; it is never retried here.
func (s *Store) WithKeys(ctx context.Context, plan KeyPlan, run func(context.Context, string) (bool, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan.configured && len(plan.keys) == 0 {
		return errors.New("供应商没有启用的密钥")
	}
	var keys []RuntimeKey
	for _, credential := range plan.keys {
		secret, err := unsealKey(credential.sealed)
		if err != nil {
			return err
		}
		keys = append(keys, RuntimeKey{ID: credential.id, Key: secret, KeyOptions: credential.options})
	}
	for _, credential := range plan.pinnedDisabled {
		secret, err := unsealKey(credential.sealed)
		if err != nil {
			return err
		}
		keys = append(keys, RuntimeKey{ID: credential.id, Key: secret, Off: true, KeyOptions: credential.options})
	}
	key := ""
	if len(keys) > 0 {
		key = keys[0].Key
		ctx = contextWithRuntimeKey(ctx, keys[0])
		ctx = context.WithValue(ctx, credentialSetContextKey{}, keys)
	}
	_, err := run(ctx, key)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

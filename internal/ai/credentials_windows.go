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
	Status     int
	RetryAfter time.Duration
}

func (e *AttemptFailure) Error() string {
	return fmt.Sprintf("AI 服务请求失败（HTTP %d）", e.Status)
}
func (e *AttemptFailure) retryable() bool {
	return e.Status == 401 || e.Status == 403 || e.Status == 408 || e.Status == 429 || e.Status >= 500 && e.Status <= 599
}

type credential struct{ id, sealed string }

// KeyPlan is an immutable request snapshot. It cannot be serialized as secrets.
type KeyPlan struct {
	provider   string
	keys       []credential
	configured bool
	policy     KeyPolicy
}
type keyRest struct {
	sealed string
	until  time.Time
}
type keyRound struct {
	next string
	rest map[string]keyRest
}

func (s *Store) keyPlanLocked(p Provider) KeyPlan {
	plan := KeyPlan{provider: p.ID, policy: keyPolicy(p)}
	for _, slot := range p.Keys {
		sealed := s.sealedCredentialLocked(p.ID, slot.ID)
		if sealed == "" {
			continue
		}
		plan.configured = true
		if slot.Enabled {
			plan.keys = append(plan.keys, credential{id: slot.ID, sealed: sealed})
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
func (s *Store) keyNow() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}
func restingKey(round *keyRound, key credential, now time.Time) bool {
	rest, ok := round.rest[key.id]
	return ok && rest.sealed == key.sealed && now.Before(rest.until)
}

func (s *Store) orderedKeys(plan KeyPlan) ([]credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(plan.keys) == 0 {
		return nil, errors.New("供应商没有启用的密钥")
	}
	if s.keyRounds == nil {
		s.keyRounds = map[string]*keyRound{}
	}
	round := s.keyRounds[plan.provider]
	if round == nil {
		round = &keyRound{rest: map[string]keyRest{}}
		s.keyRounds[plan.provider] = round
	}
	start := 0
	if plan.policy.Strategy == "round-robin" {
		for index, key := range plan.keys {
			if key.id == round.next {
				start = index
				break
			}
		}
	}
	first := -1
	now := s.keyNow()
	for step := range len(plan.keys) {
		index := (start + step) % len(plan.keys)
		if !restingKey(round, plan.keys[index], now) {
			first = index
			break
		}
	}
	if first < 0 {
		return nil, errors.New("所有密钥正在冷却，请稍后重试或检查供应商设置")
	}
	if plan.policy.Strategy == "round-robin" {
		round.next = plan.keys[(first+1)%len(plan.keys)].id
	}
	ordered := make([]credential, 0, len(plan.keys))
	for step := range len(plan.keys) {
		key := plan.keys[(first+step)%len(plan.keys)]
		if !restingKey(round, key, now) {
			ordered = append(ordered, key)
		}
	}
	return ordered, nil
}

// WithKeys never holds the configuration mutex while invoking a provider.
// The callback reports any emitted content (including reasoning), which closes
// the failover boundary even if the provider subsequently returns an error.
func (s *Store) WithKeys(ctx context.Context, plan KeyPlan, run func(context.Context, string) (bool, error)) error {
	// The caller owns protocol-specific budgets (notably long image draws).
	// Supply the traditional ceiling only when it has not bounded the request.
	if _, bounded := ctx.Deadline(); !bounded {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !plan.configured {
		_, err := run(ctx, "")
		return err
	}
	// Rotation options are exposed for multiple keys. With only one enabled
	// credential, preserve the existing immediate manual retry behavior.
	if len(plan.keys) == 1 {
		secret, err := unsealKey(plan.keys[0].sealed)
		if err != nil {
			return err
		}
		_, err = run(ctx, secret)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	ordered, err := s.orderedKeys(plan)
	if err != nil {
		return err
	}
	limit := plan.policy.MaxAttempts
	if !plan.policy.Failover {
		limit = 1
	}
	attempts := 0
	for _, key := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		round := s.keyRounds[plan.provider]
		cooling := round != nil && restingKey(round, key, s.keyNow())
		s.mu.Unlock()
		if cooling {
			continue
		}
		secret, decodeErr := unsealKey(key.sealed)
		if decodeErr != nil {
			return decodeErr
		}
		if attempts > 0 && plan.policy.Strategy == "round-robin" {
			s.mu.Lock()
			// A fallback consumes a turn too. Do not rewind a cursor that a
			// concurrent request has already advanced past this credential.
			if round := s.keyRounds[plan.provider]; round != nil && round.next == key.id {
				for index, candidate := range plan.keys {
					if candidate.id == key.id {
						round.next = plan.keys[(index+1)%len(plan.keys)].id
						break
					}
				}
			}
			s.mu.Unlock()
		}
		attempts++
		partial, failure := run(ctx, secret)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if failure == nil {
			return nil
		}
		err = failure
		var rejected *AttemptFailure
		if !errors.As(failure, &rejected) || !rejected.retryable() {
			return failure
		}
		duration := max(time.Duration(plan.policy.CooldownSeconds)*time.Second, min(rejected.RetryAfter, 24*time.Hour))
		s.mu.Lock()
		// An in-flight request can finish after a key was edited or deleted.
		// Its outcome is relevant only while this exact ciphertext still exists.
		if round := s.keyRounds[plan.provider]; round != nil && s.sealedCredentialLocked(plan.provider, key.id) == key.sealed {
			round.rest[key.id] = keyRest{sealed: key.sealed, until: s.keyNow().Add(duration)}
		}
		s.mu.Unlock()
		if partial || attempts >= limit {
			return failure
		}
	}
	if err != nil {
		return err
	}
	return errors.New("所有密钥正在冷却，请稍后重试或检查供应商设置")
}

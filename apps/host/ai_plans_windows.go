//go:build windows

package main

import (
	"context"
	"errors"
	"time"
	"winone/internal/configjson"
	"winone/internal/host"
)

func (s *aiService) handlePlans(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai_plan", Message: err.Error()}}
	}
	var params struct {
		Provider string `json:"provider"`
	}
	if err := configjson.DecodeObject(r.Params, &params); err != nil {
		return fail(err)
	}
	runtime, err := s.magpieTranslator()
	if err != nil {
		return fail(err)
	}
	p, _, err := s.store.ProviderKeysForModels(params.Provider)
	if err != nil {
		return fail(err)
	}
	if p.AuthMode != "api-key" {
		return fail(errors.New("请先保存套餐供应商"))
	}
	ctx, cancel := context.WithTimeout(s.ctx, 25*time.Second)
	defer cancel()
	gate := s.accountGateFor(p.ID)
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return fail(errors.New("套餐登录正在处理，请稍后重试"))
	}
	out, err := runtime.PlanSession(ctx, r.Method, p)
	if err != nil {
		return fail(err)
	}
	return host.Response{ID: r.ID, Result: out}
}

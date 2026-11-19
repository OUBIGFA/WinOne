//go:build windows

package main

import (
	"winone/internal/configjson"
	"winone/internal/host"
)

func (s *aiService) handleProxyProbe(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai_proxy", Message: err.Error()}}
	}
	var input struct {
		Proxy string `json:"proxy"`
	}
	if err := configjson.DecodeObject(r.Params, &input); err != nil {
		return fail(err)
	}
	runtime, err := s.magpieTranslator()
	if err != nil {
		return fail(err)
	}
	raw, err := runtime.ProbeProxy(s.ctx, input.Proxy)
	if err != nil {
		return fail(err)
	}
	return host.Response{ID: r.ID, Result: raw}
}

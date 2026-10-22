//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/host"
)

// Privacy owns a separate instance of the existing private AI process/HTTP
// transport. Saving providers may restart routing, but must not interrupt a
// model download or allocate a model for every short-lived account worker.
func (s *aiService) startPrivacy() (*ai.RoutingProcess, error) {
	s.privacy.mu.Lock()
	defer s.privacy.mu.Unlock()
	if s.privacy.process != nil {
		if s.privacy.process.Alive() {
			return s.privacy.process, nil
		}
		s.privacy.cancel()
		s.privacy.process.Close()
		s.privacy.process = nil
	}
	ctx, cancel := context.WithCancel(s.ctx)
	process, err := s.translator.StartPrivacy(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	s.privacy.process, s.privacy.cancel = process, cancel
	return process, nil
}

// Checking an absent/off preference does not launch a process or download.
func (s *aiService) privacyEndpoint(ctx context.Context) (string, string, error) {
	on, err := s.translator.PrivacyEnabled()
	if err != nil || !on {
		return "", "", err
	}
	process, err := s.startPrivacy()
	if err != nil {
		return "", "", err
	}
	return process.URL + "/api/privacy-model/find", process.Key, nil
}

func (s *aiService) releasePrivacyViewer(viewer string) {
	s.privacy.mu.Lock()
	defer s.privacy.mu.Unlock()
	delete(s.privacy.viewers, viewer)
	if len(s.privacy.viewers) != 0 || s.privacy.process == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	state, err := s.privacy.process.PrivacyModel(ctx, "")
	cancel()
	// A failed status read must not silently disable requested privacy. The
	// owned job still terminates on Host exit or can be replaced after a crash.
	if err != nil || state.On || state.Downloading {
		return
	}
	s.privacy.cancel()
	s.privacy.process.Close()
	s.privacy.process = nil
}

func (s *aiService) handlePrivacyModel(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai_privacy", Message: err.Error()}}
	}
	var input struct {
		Viewer string `json:"viewer"`
		Action string `json:"action,omitempty"`
	}
	if err := configjson.DecodeObject(r.Params, &input); err != nil {
		return fail(err)
	}
	if len(input.Viewer) < 1 || len(input.Viewer) > 80 || strings.Trim(input.Viewer, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-") != "" {
		return fail(errors.New("隐私模型页面标识无效"))
	}
	switch r.Method {
	case "magpie.privacy.close":
		s.releasePrivacyViewer(input.Viewer)
		return host.Response{ID: r.ID, Result: json.RawMessage("{}")}
	case "magpie.privacy.open", "magpie.privacy.state", "magpie.privacy.action":
	default:
		return fail(errors.New("不支持的隐私模型操作"))
	}
	if r.Method != "magpie.privacy.action" && input.Action != "" {
		return fail(errors.New("状态读取不能修改隐私模型"))
	}
	if r.Method == "magpie.privacy.action" && input.Action != "on" && input.Action != "off" && input.Action != "cancel" && input.Action != "remove" {
		return fail(errors.New("隐私模型操作无效"))
	}
	s.privacy.mu.Lock()
	if s.privacy.viewers == nil {
		s.privacy.viewers = map[string]bool{}
	}
	s.privacy.viewers[input.Viewer] = true
	s.privacy.mu.Unlock()
	process, err := s.startPrivacy()
	if err != nil {
		s.releasePrivacyViewer(input.Viewer)
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	state, err := process.PrivacyModel(ctx, input.Action)
	if err != nil {
		return fail(err)
	}
	raw, _ := json.Marshal(state)
	return host.Response{ID: r.ID, Result: raw}
}

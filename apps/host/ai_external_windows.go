//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/ai"
	"winone/internal/host"
)

// External client files are visible only to a separately owned worker opened
// by the native page's explicit Read button. No ordinary AI call starts it.
func (s *aiService) handleExternal(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "external", Message: err.Error()}}
	}
	var in struct {
		Viewer string `json:"viewer"`
		On     bool   `json:"on"`
	}
	if json.Unmarshal(r.Params, &in) != nil || len(in.Viewer) < 1 || len(in.Viewer) > 80 || strings.Trim(in.Viewer, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-") != "" {
		return fail(errors.New("外部客户端页面标识无效"))
	}
	s.external.mu.Lock()
	defer s.external.mu.Unlock()
	switch r.Method {
	case "external.open":
		if s.external.process != nil && !s.external.process.Alive() {
			s.external.cancel()
			s.external.process.Close()
			s.external.process = nil
		}
		if s.external.process == nil {
			ctx, cancel := context.WithCancel(s.ctx)
			translator := s.translator
			translator.RootDataDirectory = s.translator.DataDirectory
			translator.DataDirectory = filepath.Join(s.translator.DataDirectory, "external-clients")
			process, err := translator.StartExternal(ctx, s.externalGatewayEndpoint())
			if err != nil {
				cancel()
				return fail(err)
			}
			s.external.process, s.external.cancel = process, cancel
		}
		if s.external.viewers == nil {
			s.external.viewers = map[string]bool{}
		}
		s.external.viewers[in.Viewer] = true
		raw, _ := json.Marshal(map[string]string{"url": s.external.process.PageURL()})
		return host.Response{ID: r.ID, Result: raw}
	case "external.close":
		delete(s.external.viewers, in.Viewer)
		if len(s.external.viewers) == 0 && s.external.process != nil {
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			state, err := s.external.process.ExternalState(ctx)
			cancel()
			if err != nil {
				return fail(err)
			}
			if state.Enabled || state.Exporting {
				return host.Response{ID: r.ID, Result: json.RawMessage(`{}`)}
			}
			s.external.cancel()
			s.external.process.Close()
			s.external.process = nil
		}
		return host.Response{ID: r.ID, Result: json.RawMessage(`{}`)}
	case "external.export":
		if s.external.process == nil || !s.external.viewers[in.Viewer] {
			return fail(errors.New("请先读取客户端资料"))
		}
		// Stopping an exporter must still work if unrelated saved preferences
		// are damaged. Only an explicit start needs collector credentials.
		var prefs *ai.MagpieSettings
		if in.On {
			store, err := s.magpieStore()
			if err != nil {
				return fail(err)
			}
			prefs, err = store.MagpieRuntime()
			if err != nil {
				return fail(err)
			}
		}
		ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
		defer cancel()
		if err := s.external.process.ExternalExport(ctx, prefs, in.On); err != nil {
			return fail(err)
		}
		return host.Response{ID: r.ID, Result: json.RawMessage(`{}`)}
	default:
		return fail(errors.New("不支持的外部客户端操作"))
	}
}

// Restore only a previously enabled local MCP endpoint. StartExternal registers
// lazy handlers and never warms client discovery or session exports.
func (s *aiService) restoreExternal() {
	translator := s.translator
	translator.RootDataDirectory = s.translator.DataDirectory
	translator.DataDirectory = filepath.Join(s.translator.DataDirectory, "external-clients")
	enabled, err := translator.ExternalEnabled()
	if err != nil {
		log.Printf("external clients: %v", err)
		return
	}
	if !enabled {
		return
	}
	s.external.mu.Lock()
	defer s.external.mu.Unlock()
	if s.external.process != nil {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	process, err := translator.StartExternal(ctx, s.externalGatewayEndpoint())
	if err != nil {
		cancel()
		log.Printf("external clients: %v", err)
		return
	}
	s.external.process, s.external.cancel = process, cancel
}

func (s *aiService) externalGatewayEndpoint() ai.ExternalGatewayEndpoint {
	if s.gateway == nil {
		return ai.ExternalGatewayEndpoint{}
	}
	state := s.gateway.Status()
	if !state.Enabled || !state.Running {
		return ai.ExternalGatewayEndpoint{}
	}
	key, err := s.gateway.Key()
	if err != nil {
		log.Printf("external clients gateway: %v", err)
		return ai.ExternalGatewayEndpoint{}
	}
	return ai.ExternalGatewayEndpoint{URL: strings.TrimSuffix(state.BaseURL, "/v1"), Key: key}
}

func (s *aiService) syncExternalGateway() {
	s.external.mu.Lock()
	defer s.external.mu.Unlock()
	if s.external.process == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	if err := s.external.process.ExternalGateway(ctx, s.externalGatewayEndpoint()); err != nil {
		log.Printf("external clients gateway sync: %v", err)
	}
}

func (s *aiService) closeExternal() {
	s.external.mu.Lock()
	defer s.external.mu.Unlock()
	if s.external.process != nil {
		s.external.cancel()
		s.external.process.Close()
		s.external.process = nil
	}
	clear(s.external.viewers)
}

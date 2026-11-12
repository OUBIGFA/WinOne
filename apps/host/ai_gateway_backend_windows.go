//go:build windows

package main

import (
	"context"
	"sync"
	"time"
	"winone/internal/ai"
	"winone/internal/processidentity"
)

// The local gateway owns its protocol state and administration. It has no
// routing-group, plugin-page or external-client service lifecycle.
type gatewayBackend struct {
	mu             sync.Mutex
	process        *ai.GatewayProcess
	cancel         context.CancelFunc
	pendingCatalog map[string]ai.Provider
	viewers        map[string]*gatewayViewer
}

type gatewayViewer struct {
	cancel context.CancelFunc
}

func (s *aiService) startGatewayBackend() (*ai.GatewayProcess, error) {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	return s.startGatewayBackendLocked()
}

func (s *aiService) openGatewayBackend(viewer string, ownerID uint32) (*ai.GatewayProcess, error) {
	owner, err := processidentity.Open(ownerID, s.desktopExecutable)
	if err != nil {
		return nil, err
	}
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	process, err := s.startGatewayBackendLocked()
	if err != nil {
		owner.Close()
		return nil, err
	}
	if s.backend.viewers == nil {
		s.backend.viewers = map[string]*gatewayViewer{}
	}
	if prior := s.backend.viewers[viewer]; prior != nil {
		prior.cancel()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	lease := &gatewayViewer{cancel: cancel}
	s.backend.viewers[viewer] = lease
	// Closing the native window cancels its outstanding pipe requests. The
	// pinned process therefore also owns release, even if its close RPC is lost.
	go func() {
		defer owner.Close()
		_ = owner.Wait(ctx)
		if ctx.Err() != nil {
			return
		}
		s.backend.mu.Lock()
		defer s.backend.mu.Unlock()
		if s.backend.viewers[viewer] == lease {
			s.releaseGatewayBackendLocked(viewer)
		}
	}()
	return process, nil
}

func (s *aiService) startGatewayBackendLocked() (*ai.GatewayProcess, error) {
	if s.backend.process != nil {
		if s.backend.process.Alive() {
			ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
			defer cancel()
			for id, provider := range s.backend.pendingCatalog {
				if err := s.backend.process.SyncCatalog(ctx, provider); err != nil {
					return nil, err
				}
				delete(s.backend.pendingCatalog, id)
			}
			return s.backend.process, nil
		}
		s.backend.cancel()
		s.backend.process.Close()
		s.backend.process = nil
	}
	store, err := s.magpieStore()
	if err != nil {
		return nil, err
	}
	prefs, err := store.MagpieRuntime()
	if err != nil {
		return nil, err
	}
	providers, err := store.MagpieProviders()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(s.ctx)
	process, err := s.translator.StartGateway(ctx, prefs, providers)
	if err != nil {
		cancel()
		return nil, err
	}
	s.backend.process, s.backend.cancel = process, cancel
	s.backend.pendingCatalog = nil
	return process, nil
}

func (s *aiService) closeGatewayBackend() {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	s.closeGatewayBackendLocked()
}

// Each window owns a unique lease. A late close from an old page must not
// stop a newly opened page or a saved public gateway.
func (s *aiService) releaseGatewayBackend(viewer string) {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	s.releaseGatewayBackendLocked(viewer)
}

func (s *aiService) releaseGatewayBackendLocked(viewer string) {
	if lease := s.backend.viewers[viewer]; lease != nil {
		lease.cancel()
	}
	delete(s.backend.viewers, viewer)
	if len(s.backend.viewers) == 0 && !s.gateway.Status().Running {
		s.closeGatewayBackendLocked()
	}
}

func (s *aiService) closeGatewayBackendLocked() {
	if s.backend.process != nil {
		s.backend.cancel()
		s.backend.process.Close()
		s.backend.process = nil
	}
}

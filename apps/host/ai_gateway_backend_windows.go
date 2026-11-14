//go:build windows

package main

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sys/windows"
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
	resultOwners   map[uint32]*gatewayViewer
}

// Result windows lease the existing AI runtime for their process lifetime.
// This preserves upstream policy state without a second Host scheduler or an
// idle background engine after the last result and gateway owner has closed.
func (s *aiService) resultGatewayBackend(ownerID uint32) (*ai.GatewayProcess, error) {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	if ownerID == 0 || s.backend.resultOwners[ownerID] != nil {
		return s.startGatewayBackendLocked()
	}
	// Pin the result owner before starting its worker. A window that has
	// already exited must never create a runtime with no remaining owner.
	owner, err := windows.OpenProcess(windows.SYNCHRONIZE, false, ownerID)
	if err != nil {
		return nil, err
	}
	process, err := s.startGatewayBackendLocked()
	if err != nil {
		windows.CloseHandle(owner)
		return nil, err
	}
	ctx, cancel := context.WithCancel(s.ctx)
	lease := &gatewayViewer{cancel: cancel}
	if s.backend.resultOwners == nil {
		s.backend.resultOwners = map[uint32]*gatewayViewer{}
	}
	s.backend.resultOwners[ownerID] = lease
	go func() {
		defer windows.CloseHandle(owner)
		defer cancel()
		for ctx.Err() == nil {
			status, err := windows.WaitForSingleObject(owner, 200)
			if err != nil || status == windows.WAIT_OBJECT_0 {
				break
			}
		}
		s.backend.mu.Lock()
		defer s.backend.mu.Unlock()
		if s.backend.resultOwners[ownerID] == lease {
			delete(s.backend.resultOwners, ownerID)
		}
		if len(s.backend.resultOwners) == 0 && len(s.backend.viewers) == 0 && (s.gateway == nil || !s.gateway.Status().Running) {
			s.closeGatewayBackendLocked()
		}
	}()
	return process, nil
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

// New requests read the newly saved configuration. Existing result and public
// gateway streams keep their old upstream snapshot until their response closes.
func (s *aiService) retireGatewayBackend() {
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	if process := s.backend.process; process != nil {
		process.Retire(s.backend.cancel)
		s.backend.process, s.backend.cancel = nil, nil
	}
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
	if len(s.backend.viewers) == 0 && len(s.backend.resultOwners) == 0 && (s.gateway == nil || !s.gateway.Status().Running) {
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

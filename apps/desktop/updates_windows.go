//go:build windows

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"sync"
	"winone/internal/updates"
)

// Set only by the build; portable user preferences cannot redirect update checks.
var updateRepository string
var updateVersion string

//go:embed frontend/package.json
var updatePackageInfo []byte

type UpdateBuildInfo struct {
	Version    string `json:"version"`
	Configured bool   `json:"configured"`
}
type UpdateReply struct {
	updates.Result
	ErrorCode string `json:"errorCode,omitempty"`
}
type updateSession struct {
	mu     sync.Mutex
	id     string
	cancel context.CancelFunc
	url    string
}

func (a *App) UpdateInfo() UpdateBuildInfo {
	version := updateVersion
	if version == "" {
		var info struct {
			Version string `json:"version"`
		}
		_ = json.Unmarshal(updatePackageInfo, &info)
		version = info.Version
	}
	return UpdateBuildInfo{Version: version, Configured: updateRepository != ""}
}
func (s *updateSession) check(ctx context.Context, id string, run func(context.Context) (updates.Result, error)) UpdateReply {
	if id == "" || len(id) > 128 {
		return UpdateReply{Result: updates.Result{State: "failed"}, ErrorCode: "invalid_request"}
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return UpdateReply{Result: updates.Result{State: "failed"}, ErrorCode: "busy"}
	}
	ctx, cancel := context.WithCancel(ctx)
	s.id = id
	s.cancel = cancel
	s.url = ""
	s.mu.Unlock()
	defer cancel()
	result, err := run(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Cancellation wins even when a response arrives at the same time.
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	reply := UpdateReply{Result: result}
	if err != nil {
		reply.Result = updates.Result{State: "failed"}
		reply.ErrorCode = "unavailable"
		var failure *updates.CheckError
		if errors.As(err, &failure) {
			reply.ErrorCode = failure.Code
		}
		if errors.Is(err, context.Canceled) {
			reply.ErrorCode = "cancelled"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			reply.ErrorCode = "timeout"
		}
	}
	if err == nil && result.State == "available" {
		s.url = result.ReleaseURL
	}
	s.id = ""
	s.cancel = nil
	return reply
}
func (s *updateSession) cancelRequest(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil && s.id == id {
		s.cancel()
	}
}
func (s *updateSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}
func (s *updateSession) releaseURL() string { s.mu.Lock(); defer s.mu.Unlock(); return s.url }

func (a *App) CheckForUpdates(id string) UpdateReply {
	ctx := a.windowContext()
	if ctx == nil {
		return UpdateReply{Result: updates.Result{State: "failed"}, ErrorCode: "unavailable"}
	}
	checker := updates.Checker{Repository: updateRepository, CurrentVersion: a.UpdateInfo().Version}
	return a.updateSession.check(ctx, id, checker.Check)
}
func (a *App) CancelUpdateCheck(id string) { a.updateSession.cancelRequest(id) }

// No URL is accepted from the WebView. Only the last successful check can open it.
func (a *App) OpenUpdateRelease() error {
	address := a.updateSession.releaseURL()
	ctx := a.windowContext()
	if address == "" || ctx == nil {
		return errors.New("update release unavailable")
	}
	runtime.BrowserOpenURL(ctx, address)
	return nil
}

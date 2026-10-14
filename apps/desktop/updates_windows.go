//go:build windows

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
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
type UpdateReply = updates.Reply
type updateSession struct{ updates.Session }

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
	return s.Check(ctx, id, run)
}
func (s *updateSession) cancelRequest(id string) { s.Cancel(id) }
func (s *updateSession) close()                  { s.Close() }
func (s *updateSession) releaseURL() string      { return s.ReleaseURL() }

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

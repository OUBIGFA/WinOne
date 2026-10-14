//go:build windows

package distribution

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"time"
	"winone/internal/maintenance"
	"winone/internal/modules"
	"winone/internal/transport"
)

// PrepareNativeUpgrade is the production preparation callback. The coordinator
// already holds the installation/profile lease; this function never takes a
// second lease or shuts down user processes. WinTray starts recovery-only and
// deactivates its resources without removing the product's login task.
func PrepareNativeUpgrade(installation, profile string) error {
	if err := maintenance.RequireStoppedAt(installation); err != nil {
		return err
	}
	pipe, err := transport.PipeName()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	worker := modules.NewLazy(ctx, "wintray", "WinTray", filepath.Join(installation, "WinOne.WinTray.exe"), pipe+"-upgrade-"+rand.Text(), filepath.Join(profile, "modules", "wintray"))
	if err = modules.ReleaseWinTray(ctx, worker, false); err != nil {
		return err
	}
	return maintenance.RequireStoppedAt(installation)
}

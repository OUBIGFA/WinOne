//go:build windows

package startup

import (
	"errors"
	"path/filepath"
	"strings"
	"winone/modules/wintray/internal/config"
)

// ReleaseAll is the module-disable transaction. Try using the existing token
// first; if protected tasks remain, one helper restores the entire remainder.
// Cancellation is final for this request, never one prompt per program.
func (t *AppTasks) ReleaseAll() error {
	err := t.Sync(config.Settings{}, false)
	if err == nil || !errors.Is(err, errElevationNeeded) || t.isElevated() || t.elevateSelf == nil {
		return err
	}
	if err = t.elevateSelf(AppTaskHelperRelease, filepath.Dir(t.statePath)); err != nil {
		return err
	}
	// Re-read the ledger and inventory after the helper. A successful process
	// exit alone does not establish that module-owned resources were restored.
	return t.Sync(config.Settings{}, false)
}

func ReleaseAppTasksHeadless(directory string) error {
	expected, err := config.AppDirWithError()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(directory) || !strings.EqualFold(filepath.Clean(directory), filepath.Clean(expected)) {
		return errors.New("startup restoration requires the current user's module directory")
	}
	if err := config.SetWorkerDataDirectory(directory); err != nil {
		return err
	}
	tasks, err := NewAppTasks()
	if err != nil {
		return err
	}
	return tasks.Sync(config.Settings{}, false)
}

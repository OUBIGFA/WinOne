//go:build windows

package worker

import (
	"errors"
	"testing"

	"winone/modules/wintray/internal/config"
	"winone/modules/wintray/internal/traybox"
)

// A taskbar that is not ready yet must not fail the reconciliation. The
// collector retries until Explorer creates it.
func TestCollectTrayIconsRetriesUntilTheTaskbarExists(t *testing.T) {
	attempts := 0
	apply := func([]string) error {
		attempts++
		if attempts < 3 {
			return traybox.ErrTaskbarUnavailable
		}
		return nil
	}
	wait := func() error { return nil }
	err := collectTrayIconsWith(apply, wait, config.Settings{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

// Any other failure is reported at once; retrying cannot fix it.
func TestCollectTrayIconsReportsOtherFailures(t *testing.T) {
	attempts := 0
	apply := func([]string) error {
		attempts++
		return errors.New("collection refused")
	}
	wait := func() error { return nil }
	err := collectTrayIconsWith(apply, wait, config.Settings{}, nil)
	if err == nil || attempts != 1 {
		t.Fatalf("err = %v, attempts = %d, want one attempt", err, attempts)
	}
}

// A stopped worker must not keep retrying the collector.
func TestCollectTrayIconsStopsWithTheWorker(t *testing.T) {
	attempts := 0
	apply := func([]string) error {
		attempts++
		return traybox.ErrTaskbarUnavailable
	}
	wait := func() error { return errors.New("module stopped") }
	err := collectTrayIconsWith(apply, wait, config.Settings{}, nil)
	if err == nil || attempts != 1 {
		t.Fatalf("err = %v, attempts = %d, want one attempt", err, attempts)
	}
}

//go:build windows

package orchestrator

import (
	"context"
	"testing"
	"time"
	"winone/modules/wintray/internal/config"
)

func TestCustomScheduleRunsWithoutOrdinaryStartup(t *testing.T) {
	entry := queueTestEntry(t, "custom schedule")
	entry.RunOnStartup = false
	entry.Schedule = config.Schedule{Enabled: true, StartDelayMinutes: 1}
	old := logonTimeLookup
	defer func() { logonTimeLookup = old }()
	settings := config.DefaultSettings()
	settings.ManagedApps = []config.ManagedAppEntry{entry}
	// Before the sign-in delay expires, cancellation must leave no process.
	logonTimeLookup = func() (time.Time, error) { return time.Now(), nil }
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	results := queueTestService().StartManagedApps(ctx, settings, nil)
	cancel()
	if len(results) != 1 || results[0].Code != ResultCancelled || len(startupTestPIDs(entry.ExePath)) != 0 {
		t.Fatalf("custom schedule did not wait for its delay: %+v", results)
	}
	// Starting WinOne late uses the same sign-in clock, not another full delay.
	logonTimeLookup = func() (time.Time, error) { return time.Now().Add(-2 * time.Minute), nil }
	ctx, cancel = context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	results = queueTestService().StartManagedApps(ctx, settings, nil)
	if len(results) != 1 || results[0].Code != ResultStartedHidden {
		t.Fatalf("due custom schedule did not launch: %+v", results)
	}
	queueProcessStart(t, entry)
}

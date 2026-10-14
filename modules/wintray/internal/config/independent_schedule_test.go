package config

import (
	"path/filepath"
	"testing"
	"time"
)

// A custom schedule is its own automatic-start choice, including its quota.
func TestCustomScheduleWithoutOrdinaryStartup(t *testing.T) {
	entry := ManagedAppEntry{ID: "scheduled", Name: "Scheduled app", ExePath: `C:\Apps\Scheduled.exe`,
		Schedule: Schedule{Enabled: true, StartDelayMinutes: 7, AutoExitMinutes: 9, FrequencyEnabled: true, FrequencyDays: 2, FrequencyRuns: 1}}
	if !ShouldLaunchViaWinTray(entry) {
		t.Error("custom schedule was excluded from automatic startup")
	}
	settings := DefaultSettings()
	settings.ManagedApps = []ManagedAppEntry{entry}
	apps := LogonTaskApps(settings)
	if len(apps) != 1 || apps[0].DelaySeconds != 420 {
		t.Errorf("scheduled quota task missing or wrong delay: %+v", apps)
	}
	history := NewStartupHistory(filepath.Join(t.TempDir(), "history.json"))
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	for i, want := range []bool{true, false} {
		if allowed, err := history.Launch(entry, now, func() error { return nil }); err != nil || allowed != want {
			t.Errorf("launch %d allowed=%t err=%v, want %t", i, allowed, err, want)
		}
	}
	entry.Schedule.Enabled = false
	if ShouldLaunchViaWinTray(entry) || StartupFrequencyEnabled(entry) {
		t.Error("both automatic-start choices are off but the program is still scheduled")
	}
}

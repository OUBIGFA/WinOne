package worker

import (
	"testing"
	"winone/modules/wintray/internal/config"
)

func TestStartupEffectsIgnoreUnrelatedEditsAndKeepEveryTaskInput(t *testing.T) {
	base := config.DefaultSettings()
	base.RunAtLogon = true
	base.ManagedApps = []config.ManagedAppEntry{{ID: "one", Name: "One", ExePath: `C:\test.exe`, RunOnStartup: true, LaunchViaLogonTask: true, Schedule: config.Schedule{Enabled: true, FrequencyEnabled: true, FrequencyDays: 1, FrequencyRuns: 1}}}
	for name, edit := range map[string]func(*config.Settings){
		"language": func(v *config.Settings) { v.Language = "en" },
		"interval": func(v *config.Settings) { v.StartupIntervalSeconds++ },
		"window":   func(v *config.Settings) { v.ManagedApps[0].TrayBehavior.CloseDelaySeconds++ },
		"tray":     func(v *config.Settings) { v.ManagedApps[0].CollectTrayIcon = true },
		"autoexit": func(v *config.Settings) { v.ManagedApps[0].Schedule.AutoExitMinutes++ },
		"qq":       func(v *config.Settings) { v.ManagedApps[0].QQWaitForLogin = true },
	} {
		t.Run(name, func(t *testing.T) {
			next := clone(base)
			edit(&next)
			if !sameStartupEffects(base, next) {
				t.Fatal("unrelated edit repeated task synchronization")
			}
		})
	}
	for name, edit := range map[string]func(*config.Settings){
		"logon":     func(v *config.Settings) { v.RunAtLogon = false },
		"remove":    func(v *config.Settings) { v.ManagedApps = nil },
		"path":      func(v *config.Settings) { v.ManagedApps[0].ExePath = `C:\other.exe` },
		"arguments": func(v *config.Settings) { v.ManagedApps[0].Args = "--new" },
		"directory": func(v *config.Settings) { v.ManagedApps[0].WorkingDirectory = `D:\Data` },
		"id":        func(v *config.Settings) { v.ManagedApps[0].ID = "two" },
		"task":      func(v *config.Settings) { v.ManagedApps[0].LaunchViaLogonTask = false },
		"hidden":    func(v *config.Settings) { v.ManagedApps[0].LaunchHiddenInBackground = true },
		"delay":     func(v *config.Settings) { v.ManagedApps[0].Schedule.StartDelayMinutes++ },
		"frequency": func(v *config.Settings) { v.ManagedApps[0].Schedule.FrequencyRuns++ },
	} {
		t.Run(name, func(t *testing.T) {
			next := clone(base)
			edit(&next)
			if sameStartupEffects(base, next) {
				t.Fatal("task edit was ignored")
			}
		})
	}
}

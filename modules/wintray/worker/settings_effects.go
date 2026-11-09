package worker

import (
	"reflect"
	"winone/modules/wintray/internal/config"
)

// Keep only fields consumed by startup.AppTasks. Names are included because
// they form task descriptions; hidden launch matters to frequency wrappers.
func sameStartupEffects(before, next config.Settings) bool {
	projection := func(value config.Settings) config.Settings {
		result := config.Settings{RunAtLogon: value.RunAtLogon}
		if !value.RunAtLogon {
			return result
		}
		for _, entry := range value.ManagedApps {
			if !config.ShouldLaunchViaWinTray(entry) {
				continue
			}
			entry.CollectTrayIcon = false
			entry.WeChatAutoLogin = false
			entry.TrayBehavior = config.TrayBehavior{}
			entry.Schedule.AutoExitMinutes = 0
			result.ManagedApps = append(result.ManagedApps, entry)
		}
		return result
	}
	return reflect.DeepEqual(projection(before), projection(next))
}

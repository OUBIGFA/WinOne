//go:build windows

package main

import (
	"os"
	"winone/internal/adminstartup"
	"winone/internal/host"
	"winone/internal/settings"
)

type administratorAuthorizationTask interface {
	Configured() (bool, error)
	Register() error
	Remove() error
}

func (a *App) SetAdministratorPreference(enabled bool) (settings.Settings, error) {
	a.authorizationMu.Lock()
	defer a.authorizationMu.Unlock()
	var snapshot host.Snapshot
	if err := a.request("snapshot", nil, &snapshot); err != nil {
		return settings.Settings{}, err
	}
	if err := a.configureAdministratorAuthorization(enabled); err != nil {
		return snapshot.Settings, err
	}
	// UAC can remain open while other settings change. Read the latest revision
	// after consent rather than overwriting unrelated preferences from before it.
	if err := a.request("snapshot", nil, &snapshot); err != nil {
		return settings.Settings{}, err
	}
	if snapshot.Settings.Startup == nil {
		snapshot.Settings.Startup = &settings.StartupPreferences{StartMinimized: true}
	}
	snapshot.Settings.Startup.PreferAdministrator = enabled
	return a.SaveSettings(snapshot.Settings)
}

// The consent helper accepts no path, script or forwarded application flags.
// Another administrator's credentials cannot grant the originating user's task.
func administratorAuthorizationRequest(args []string) (bool, string, bool, error) {
	return adminstartup.AuthorizationRequest(args)
}
func applyAdministratorAuthorization(enable, elevated bool, expectedSID, currentSID string, task administratorAuthorizationTask) error {
	return adminstartup.ApplyAuthorization(enable, elevated, expectedSID, currentSID, task)
}
func runAdministratorAuthorizationHelper(args []string, executable string) (bool, error) {
	return adminstartup.RunAuthorizationHelper(args, executable)
}

// Authorizing a launcher never stops the current Host or changes user settings.
// The caller must separately complete restart handoff and persist its preference.
func configureAdministratorAuthorization(enable, elevated bool, exe, sid string, task administratorAuthorizationTask, launch func(string, []string) (uint32, func() error, error)) error {
	return adminstartup.ConfigureAuthorization(enable, elevated, exe, sid, task, launch)
}

func (a *App) ConfigureAdministratorAuthorization(enabled bool) error {
	a.authorizationMu.Lock()
	defer a.authorizationMu.Unlock()
	return a.configureAdministratorAuthorization(enabled)
}
func (a *App) configureAdministratorAuthorization(enabled bool) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return adminstartup.ConfigureCurrent(enabled, a.pipe, a.dataDirectory, executable)
}

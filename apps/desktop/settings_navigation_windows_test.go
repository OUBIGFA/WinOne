//go:build windows

package main

import "testing"

func TestSettingsNavigationRetainsStartupRequest(t *testing.T) {
	a := &App{}
	if a.SettingsNavigation() != 0 {
		t.Fatal("unexpected navigation")
	}
	a.requestSettingsNavigation()
	if a.SettingsNavigation() != 1 || a.SettingsNavigation() != 1 {
		t.Fatal("request must survive reads before React is ready")
	}
	a.requestSettingsNavigation()
	if a.SettingsNavigation() != 2 {
		t.Fatal("a later click must create a new request")
	}
}

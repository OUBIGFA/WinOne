//go:build windows

package main

import (
	"errors"
	"reflect"
	"testing"
)

type authorizationFixture struct {
	ready              bool
	registers, removes int
}

func (f *authorizationFixture) Configured() (bool, error) { return f.ready, nil }
func (f *authorizationFixture) Register() error           { f.registers++; f.ready = true; return nil }
func (f *authorizationFixture) Remove() error             { f.removes++; f.ready = false; return nil }

func TestAdministratorHelperAcceptsOnlyFixedOperationAndOneSID(t *testing.T) {
	const sid = "S-1-5-21-1"
	for _, args := range [][]string{{"--authorize-administrator"}, {"--authorize-administrator", sid, "--data-dir", "other"}, {"--revoke-administrator", "--other"}} {
		_, _, handled, err := administratorAuthorizationRequest(args)
		if !handled || err == nil {
			t.Fatal(args, handled, err)
		}
	}
	enable, user, handled, err := administratorAuthorizationRequest([]string{"--authorize-administrator", sid})
	if err != nil || !handled || !enable || user != sid {
		t.Fatal(enable, user, handled, err)
	}
	_, _, handled, err = administratorAuthorizationRequest([]string{"--open-settings"})
	if handled || err != nil {
		t.Fatal("ordinary app startup intercepted")
	}
}
func TestAdministratorHelperRejectsUnelevatedAndDifferentUsers(t *testing.T) {
	task := &authorizationFixture{}
	if err := applyAdministratorAuthorization(true, false, "same", "same", task); err == nil {
		t.Fatal("unelevated authorization accepted")
	}
	if err := applyAdministratorAuthorization(true, true, "original", "different", task); err == nil {
		t.Fatal("different administrator account accepted")
	}
	if task.registers != 0 || task.removes != 0 {
		t.Fatal(task)
	}
	if err := applyAdministratorAuthorization(true, true, "same", "same", task); err != nil {
		t.Fatal(err)
	}
	if task.registers != 1 {
		t.Fatal(task)
	}
}
func TestAdministratorConsentCancellationLeavesAuthorizationUnchanged(t *testing.T) {
	task := &authorizationFixture{}
	cancelled := errors.New("cancelled")
	err := configureAdministratorAuthorization(true, false, `C:\WinOne\WinOne.exe`, "SID", task, func(string, []string) (uint32, func() error, error) { return 0, nil, cancelled })
	if !errors.Is(err, cancelled) || task.ready || task.registers != 0 {
		t.Fatal(err, task)
	}
}

func TestAdministratorAuthorizationIsUnavailableToIsolatedTestProfiles(t *testing.T) {
	app := &App{dataDirectory: t.TempDir()}
	if err := app.ConfigureAdministratorAuthorization(true); err == nil {
		t.Fatal("test profile reached system authorization")
	}
}
func TestAdministratorAuthorizationWaitsAndVerifiesTaskBeforeSuccess(t *testing.T) {
	task := &authorizationFixture{}
	waited := false
	err := configureAdministratorAuthorization(true, false, `C:\WinOne\WinOne.exe`, "SID", task, func(exe string, args []string) (uint32, func() error, error) {
		if exe != `C:\WinOne\WinOne.exe` || !reflect.DeepEqual(args, []string{"--authorize-administrator", "SID"}) {
			t.Fatal(exe, args)
		}
		return 123, func() error { waited = true; task.ready = true; return nil }, nil
	})
	if err != nil || !waited {
		t.Fatal(err, waited)
	}
	err = configureAdministratorAuthorization(true, false, "unused", "SID", task, func(string, []string) (uint32, func() error, error) {
		t.Fatal("repeated authorization requested UAC")
		return 0, nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	task.ready = false
	err = configureAdministratorAuthorization(true, false, "unused", "SID", task, func(string, []string) (uint32, func() error, error) { return 123, func() error { return nil }, nil })
	if err == nil {
		t.Fatal("helper exit was mistaken for registered authorization")
	}
}

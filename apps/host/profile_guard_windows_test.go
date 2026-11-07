//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
	"winone/internal/distribution"
)

func TestTemporaryCopyCannotRunAgainstDefaultProfile(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	profile := filepath.Join(local, "WinOne")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	previous := captureProfileRegistration
	t.Cleanup(func() { captureProfileRegistration = previous })
	calls := 0
	captureProfileRegistration = func(directory, key string) (distribution.RegistrationSnapshot, error) {
		calls++
		if directory != filepath.Dir(executable) || key != `Software\Microsoft\Windows\CurrentVersion\Uninstall\WinOne` {
			t.Fatal("guard checked a different installation identity")
		}
		return distribution.RegistrationSnapshot{}, errors.New("registration belongs to another installation")
	}
	// A registered different installation owns the default profile. Absence
	// is intentionally portable use, and must not depend on this test machine.
	if err := guardDefaultProfile(profile, false); err == nil {
		t.Fatal("temporary copy opened the default profile without --data-dir")
	}
	if err := guardDefaultProfile(profile, true); err != nil {
		t.Fatal("explicit --data-dir must keep working")
	}
	if err := guardDefaultProfile(t.TempDir(), false); err != nil {
		t.Fatal("isolated profile must not be blocked")
	}
	if calls != 1 {
		t.Fatal("explicit or isolated profile read the installation registry")
	}
	for _, exists := range []bool{false, true} {
		captureProfileRegistration = func(string, string) (distribution.RegistrationSnapshot, error) {
			return distribution.RegistrationSnapshot{Exists: exists}, nil
		}
		if err := guardDefaultProfile(profile, false); err != nil {
			t.Fatalf("portable or correctly registered launch rejected: %v", err)
		}
	}
}

func TestProfileGuardWithIsolatedRegistration(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("explicit isolated registry check")
	}
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	profile := filepath.Join(local, "WinOne")
	keyPath := fmt.Sprintf(`Software\WinOneInstallerTests\profile-guard-%d`, time.Now().UnixNano())
	previous := captureProfileRegistration
	t.Cleanup(func() { captureProfileRegistration = previous })
	captureProfileRegistration = func(directory, _ string) (distribution.RegistrationSnapshot, error) {
		return distribution.CaptureRegistration(directory, keyPath)
	}
	if err := guardDefaultProfile(profile, false); err != nil {
		t.Fatalf("portable use with no registration failed: %v", err)
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		key.Close()
		if err := registry.DeleteKey(registry.CURRENT_USER, keyPath); err != nil {
			t.Error(err)
		}
	})
	if err := key.SetStringValue("InstallLocation", local); err != nil {
		t.Fatal(err)
	}
	if err := guardDefaultProfile(profile, false); err == nil {
		t.Fatal("unrelated copy reached the default profile")
	}
	if err := guardDefaultProfile(profile, true); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := key.SetStringValue("InstallLocation", filepath.Dir(executable)); err != nil {
		t.Fatal(err)
	}
	if err := guardDefaultProfile(profile, false); err != nil {
		t.Fatalf("registered owner rejected: %v", err)
	}
}

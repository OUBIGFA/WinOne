//go:build windows

package logintask

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdministratorOtherInstallationIsDistinctFromTampering(t *testing.T) {
	sid := "S-1-5-21-1-2-3-1001"
	raw, _ := taskXML(sid, `C:\trial\WinOne.exe`, administratorLaunchArguments, true, true)
	if err := validateAdministratorDefinition([]byte(raw), sid, `C:\installed\WinOne.exe`); !errors.Is(err, ErrAdministratorOtherInstallation) {
		t.Fatalf("valid task from another installation should be retained: %v", err)
	}
	for _, changed := range []string{
		strings.ReplaceAll(raw, "HighestAvailable", "LeastPrivilege"),
		strings.ReplaceAll(raw, "WinOne.exe", "other.exe"),
		strings.ReplaceAll(raw, "--authorized-start", "--other"),
	} {
		if err := validateAdministratorDefinition([]byte(changed), sid, `C:\installed\WinOne.exe`); err == nil || errors.Is(err, ErrAdministratorOtherInstallation) {
			t.Fatalf("tampered task treated as another valid installation: %v", err)
		}
	}
}

func TestAdministratorDefinitionAcceptsSameInstallationThroughDirectoryLink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	link := filepath.Join(root, "alias")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "WinOne.exe"), []byte("program"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("directory links unavailable: %v", err)
	}
	sid := "S-1-5-21-1-2-3-1001"
	raw, _ := taskXML(sid, filepath.Join(link, "WinOne.exe"), administratorLaunchArguments, true, true)
	if err := validateAdministratorDefinition([]byte(raw), sid, filepath.Join(real, "WinOne.exe")); err != nil {
		t.Fatal(err)
	}
}

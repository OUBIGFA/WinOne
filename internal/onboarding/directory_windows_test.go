//go:build windows

package onboarding

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func directoryACL(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}

func TestPrepareDirectoryGrantsOnlyNewOnboardingDirectoryAndPreservesMarker(t *testing.T) {
	root := t.TempDir()
	before := directoryACL(t, root)
	directory := filepath.Join(root, "WinOne", "onboarding")
	if err := PrepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	acl := directoryACL(t, directory)
	if !strings.Contains(acl, "(A;OICI;0x1301bf;;;BU)") {
		t.Fatalf("ordinary users cannot inherit Modify: %s", acl)
	}
	if directoryACL(t, root) != before {
		t.Fatal("ancestor permissions changed")
	}
	marker := filepath.Join(directory, "result-send-hint-v1.once")
	if err := os.WriteFile(marker, []byte("claimed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "claimed" || directoryACL(t, directory) != acl {
		t.Fatal("repeat setup reset the marker or permissions", err)
	}
}

func TestPrepareDirectoryDoesNotExpandAnExistingDirectoryACL(t *testing.T) {
	directory := t.TempDir()
	before := directoryACL(t, directory)
	if err := PrepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	if directoryACL(t, directory) != before {
		t.Fatal("existing directory permissions changed")
	}
}

func TestDirectoryLockRejectsReplacementAndReparsePoints(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "WinOne", "onboarding")
	if err := PrepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	release, err := LockDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Dir(directory), filepath.Join(root, "moved")); err == nil {
		release()
		t.Fatal("a locked parent was replaced")
	}
	release()
	if err := os.Rename(filepath.Dir(directory), filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "target")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "alias")
	command := exec.Command("powershell.exe", "-NoProfile", "-Command", `New-Item -ItemType Junction -Path $env:WINONE_ONBOARDING_TEST_LINK -Target $env:WINONE_ONBOARDING_TEST_TARGET -ErrorAction Stop | Out-Null`)
	command.Env = append(os.Environ(), "WINONE_ONBOARDING_TEST_LINK="+link, "WINONE_ONBOARDING_TEST_TARGET="+outside)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("junction fixture: %v %s", err, output)
	}
	before := directoryACL(t, outside)
	if err := PrepareDirectory(filepath.Join(link, "onboarding")); err == nil {
		t.Fatal("initializer followed a junction")
	}
	if _, err := os.Stat(filepath.Join(outside, "onboarding")); !os.IsNotExist(err) {
		t.Fatal("initializer wrote through junction", err)
	}
	if directoryACL(t, outside) != before {
		t.Fatal("initializer changed target permissions")
	}
}

//go:build windows

package distribution

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInstalledVerificationRejectsJunctionParent(t *testing.T) {
	dir, archive := installedFixture(t)
	outside := filepath.Join(t.TempDir(), "capture")
	if err := os.Rename(filepath.Join(dir, "capture"), outside); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", "New-Item -ItemType Junction -Path $env:WINONE_TEST_JUNCTION -Target $env:WINONE_TEST_OUTSIDE -ErrorAction Stop | Out-Null")
	cmd.Env = append(os.Environ(), "WINONE_TEST_JUNCTION="+filepath.Join(dir, "capture"), "WINONE_TEST_OUTSIDE="+outside)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("junction: %v %s", err, output)
	}
	if _, err := VerifyInstalled(dir, archive); err == nil {
		t.Fatal("junction-backed installation accepted")
	}
	if data, err := os.ReadFile(filepath.Join(outside, "coreclr.dll")); err != nil || string(data) != "capture/coreclr.dll" {
		t.Fatal("outside file changed", err)
	}
}

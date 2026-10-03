//go:build windows

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"winone/internal/maintenance"
)

func TestNSISPendingMarkerBlocksRuntimeUntilCleared(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("set WINONE_NSIS_EXE")
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	installation, profile, work := t.TempDir(), t.TempDir(), t.TempDir()
	record := filepath.Join(installation, ".winone-recovery.ini")
	if err := os.WriteFile(record, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"Mark", "Clear"} {
		script := fmt.Sprintf(`Unicode true
Name "Pending marker test"
OutFile "%s"
RequestExecutionLevel user
SilentInstall silent
InstallDir "%s"
!include "%s"
!insertmacro WinOnePendingFunctions ""
Section
Call WinOne%sPending
IfErrors failed
SetErrorLevel 0
Quit
failed:
SetErrorLevel 1
Quit
SectionEnd
`, filepath.Join(work, operation+".exe"), installation, filepath.Join(root, "build", "windows", "pending.nsh"), operation)
		path := filepath.Join(work, operation+".nsi")
		if err = os.WriteFile(path, []byte(script), 0600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput(); err != nil {
			t.Fatalf("compile: %v %s", err, output)
		}
		if err = exec.Command(filepath.Join(work, operation+".exe")).Run(); err != nil {
			t.Fatal(err)
		}
		lease, err := maintenance.AcquireRuntimeForInstallation(profile, installation)
		if operation == "Mark" {
			if err == nil {
				lease.Close()
				t.Fatal("NSIS marker did not block runtime")
			}
			repair, err := maintenance.AcquireMaintenanceForInstallation(profile, installation)
			if err != nil {
				t.Fatal("repair blocked", err)
			}
			repair.Close()
		} else {
			if _, err := os.Stat(record); !os.IsNotExist(err) {
				t.Fatal("recovery record remained after completion", err)
			}
			if err != nil {
				t.Fatal("cleared marker still blocks runtime", err)
			}
			lease.Close()
		}
	}
}

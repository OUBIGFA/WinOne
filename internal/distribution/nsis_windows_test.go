//go:build windows

package distribution

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNSISFilesStopOnGuardOrDeleteFailure(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("set WINONE_NSIS_EXE")
	}
	for _, scenario := range []string{"guard", "locked", "write_locked", "late_locked", "late_readonly", "parent_junction", "write_parent_junction"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			payload, target := filepath.Join(dir, "payload"), filepath.Join(dir, "target")
			os.MkdirAll(payload, 0700)
			os.MkdirAll(target, 0700)
			for _, name := range []string{"WinOne.exe", "manifest.json"} {
				if err := os.WriteFile(filepath.Join(payload, name), []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, name), []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			manifest := newManifest("0.1.0-alpha.1")
			manifest.Files = []File{{Path: "WinOne.exe"}}
			if scenario == "parent_junction" || scenario == "write_parent_junction" {
				outside := filepath.Join(dir, "outside")
				if err := os.Mkdir(outside, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(outside, "notice.txt"), []byte("outside"), 0600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", "New-Item -ItemType Junction -Path $env:WINONE_TEST_JUNCTION -Target $env:WINONE_TEST_OUTSIDE -ErrorAction Stop | Out-Null")
				cmd.Env = append(os.Environ(), "WINONE_TEST_JUNCTION="+filepath.Join(target, "licenses"), "WINONE_TEST_OUTSIDE="+outside)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("junction fixture: %v %s", err, output)
				}
				manifest.Files = append(manifest.Files, File{Path: "licenses/notice.txt"})
				if err := os.MkdirAll(filepath.Join(payload, "licenses"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(payload, "licenses", "notice.txt"), []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if data, err := os.ReadFile(filepath.Join(outside, "notice.txt")); err != nil || string(data) != "outside" {
						t.Error("outside file changed", err)
					}
				})
			}
			macros, err := NSISFileMacros(manifest)
			if err != nil {
				t.Fatal(err)
			}
			guard, operation := "SetErrors", "WinOneInstallFiles"
			if scenario == "parent_junction" {
				guard, operation = "ClearErrors", "WinOneRemoveFiles"
			}
			if scenario == "write_parent_junction" {
				guard, operation = "ClearErrors", "WinOneInstallFiles"
			}
			if scenario == "late_readonly" {
				guard, operation = "ClearErrors", "WinOneRemoveFiles"
				path, _ := windows.UTF16PtrFromString(filepath.Join(target, "manifest.json"))
				if err := windows.SetFileAttributes(path, windows.FILE_ATTRIBUTE_READONLY); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := windows.SetFileAttributes(path, windows.FILE_ATTRIBUTE_NORMAL); err != nil {
						t.Error(err)
					}
				})
			}
			if scenario == "locked" || scenario == "write_locked" || scenario == "late_locked" {
				guard, operation = "ClearErrors", "WinOneRemoveFiles"
				if scenario == "write_locked" {
					operation = "WinOneInstallFiles"
				}
				lockedName := "WinOne.exe"
				if scenario == "late_locked" {
					lockedName = "manifest.json"
				}
				path, _ := windows.UTF16PtrFromString(filepath.Join(target, lockedName))
				handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer windows.CloseHandle(handle)
			}
			script := fmt.Sprintf(`Unicode true
Name "WinOne operation test"
OutFile "%s"
RequestExecutionLevel user
SilentInstall silent
InstallDir "%s"
!define WINONE_PAYLOAD_DIR "%s"
%s
Function Guard
  %s
FunctionEnd
Section
  !insertmacro %s Guard failed
  SetErrorLevel 0
  Quit
failed:
  SetErrorLevel 7
  Quit
SectionEnd
`, filepath.Join(dir, "check.exe"), target, payload, macros, guard, operation)
			path := filepath.Join(dir, "check.nsi")
			if err = os.WriteFile(path, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput(); err != nil {
				t.Fatalf("compile: %v %s", err, output)
			}
			err = exec.Command(filepath.Join(dir, "check.exe")).Run()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 {
				t.Fatalf("operation did not stop: %v", err)
			}
			for _, name := range []string{"WinOne.exe", "manifest.json"} {
				if data, err := os.ReadFile(filepath.Join(target, name)); err != nil || string(data) != "original" {
					t.Fatal("modified after failure", name, err)
				}
			}
		})
	}
}

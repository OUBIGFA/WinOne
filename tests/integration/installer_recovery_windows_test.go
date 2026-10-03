//go:build windows

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNSISRecoveryRecordIdentity(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("set WINONE_NSIS_EXE")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"valid", "Product", "Schema", "Version", "PackageHash", "Directory", "Operation", "Complete", "missing-marker"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".winone-pending"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			change := ""
			if field == "missing-marker" {
				change = `Delete "$INSTDIR\.winone-pending"`
			} else if field != "valid" {
				change = fmt.Sprintf(`WriteINIStr "$INSTDIR\.winone-recovery.ini" "Recovery" "%s" "mismatch"`, field)
			}
			script := fmt.Sprintf(`Unicode true
Name "Recovery record test"
OutFile "%s"
RequestExecutionLevel user
SilentInstall silent
InstallDir "%s"
!define WINONE_VERSION "0.1.0-alpha.1"
!define WINONE_MANIFEST_SHA256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
!include "%s"
!insertmacro WinOneRecoveryFunctions ""
Section
StrCpy $WinOneRecoveryOperation "install"
Call WinOneWriteRecoveryRecord
IfErrors writefailed
%s
Call WinOneCheckRecoveryRecord
IfErrors rejected
SetErrorLevel 0
Quit
rejected:
SetErrorLevel 7
Quit
writefailed:
FileOpen $0 "$INSTDIR\error.txt" w
FileWrite $0 $WinOneRecoveryError
FileClose $0
SetErrorLevel 9
Quit
SectionEnd
`, filepath.Join(dir, "check.exe"), dir, filepath.Join(root, "build", "windows", "recovery.nsh"), change)
			path := filepath.Join(dir, "check.nsi")
			if err := os.WriteFile(path, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput(); err != nil {
				t.Fatalf("compile: %v %s", err, output)
			}
			err := exec.Command(filepath.Join(dir, "check.exe")).Run()
			if data, readErr := os.ReadFile(filepath.Join(dir, "error.txt")); readErr == nil {
				t.Log(string(data))
			}
			if field == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 {
				t.Fatalf("mismatched record accepted or write failed: %v", err)
			}
		})
	}
}

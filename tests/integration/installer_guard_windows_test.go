//go:build windows

package integration

import (
	"context"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Compile and execute NSIS against a test-owned maintenance process. No package
// files, registry values, shortcuts or real user configuration are modified.
func TestNSISGuardProtocol(t *testing.T) {
	compiler, host := os.Getenv("WINONE_NSIS_EXE"), os.Getenv("WINONE_HOST_EXE")
	if compiler == "" || host == "" {
		t.Skip("set WINONE_NSIS_EXE and WINONE_HOST_EXE")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return strings.ReplaceAll(s, "$", "$$") }
	for _, scenario := range []string{"ready", "failed", "lost", "released", "closed-owner-alive"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			helper := filepath.Join(dir, "WinOne.Maintenance.exe")
			data, err := os.ReadFile(host)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(helper, data, 0700); err != nil {
				t.Fatal(err)
			}
			command := fmt.Sprintf(`"%s" --guard-installation "%s" --data-dir "%s" --pipe "\\.\pipe\WinOne-nsis-%d"`, quote(helper), quote(filepath.Dir(host)), quote(filepath.Join(dir, "profile")), time.Now().UnixNano())
			if scenario == "failed" {
				command += " --unknown-option"
			}
			after := `IfErrors failed
  Call WinOneCheckGuard
  IfErrors failed
  StrCpy $8 "ready"
  Goto write
failed:
  StrCpy $8 $WinOneGuardError`
			if scenario == "lost" {
				after = `IfErrors failed
  ; Fault injection targets only the process just created by this fixture.
  System::Call 'kernel32::TerminateProcess(p $WinOneGuardHandle, i 9)'
  System::Call 'kernel32::WaitForSingleObject(p $WinOneGuardHandle, i 5000)'
  Call WinOneCheckGuard
  IfErrors failed
  StrCpy $8 "unexpected_alive"
  Goto write
failed:
  StrCpy $8 $WinOneGuardError`
			}
			if scenario == "released" {
				after = `IfErrors failed
  Call WinOneReleaseGuard
  IfErrors failed
  StrCpy $8 "released"
  Goto write
failed:
  StrCpy $8 "release_failed"`
			}
			if scenario == "closed-owner-alive" {
				after = `IfErrors failed
  ; A wizard's GUI can close while a foreign DLL delays final process exit.
  ; Closing our guard must therefore release it while this owner is alive.
  Call WinOneCloseGuardHandle
  System::Call 'kernel32::OpenProcess(i 0x100000, i 0, i $WinOneGuardPID) p.r6'
  StrCpy $8 "closed"
  ${If} $6 != 0
    System::Call 'kernel32::WaitForSingleObject(p r6, i 1000) i.r7'
    System::Call 'kernel32::CloseHandle(p r6)'
    ${If} $7 != 0
      StrCpy $8 "guard_retained_live_owner"
    ${EndIf}
  ${EndIf}
  Goto write
failed:
  StrCpy $8 $WinOneGuardError`
			}
			script := fmt.Sprintf(`Unicode true
Name "WinOne guard test"
OutFile "%s"
RequestExecutionLevel user
SilentInstall silent
!include "%s"
!insertmacro WinOneGuardFunctions ""
Section
  StrCpy $WinOneGuardCommand '%s'
  Call WinOneStartGuard
  %s
write:
  FileOpen $0 "%s" w
  FileWrite $0 "$8|$WinOneGuardPID"
  FileClose $0
  Call WinOneCloseGuardHandle
SectionEnd
`, filepath.Join(dir, "check.exe"), filepath.Join(root, "build", "windows", "maintenance.nsh"), command, after, quote(filepath.Join(dir, "result.txt")))
			path := filepath.Join(dir, "check.nsi")
			if err = os.WriteFile(path, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput(); err != nil {
				t.Fatalf("compile: %v %s", err, output)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
			defer cancel()
			if output, err := exec.CommandContext(ctx, filepath.Join(dir, "check.exe")).CombinedOutput(); err != nil {
				t.Fatalf("run: %v %s", err, output)
			}
			result, err := os.ReadFile(filepath.Join(dir, "result.txt"))
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.SplitN(string(result), "|", 2)
			if len(parts) != 2 {
				t.Fatal("missing guard identity")
			}
			pid, err := strconv.ParseUint(parts[1], 10, 32)
			if err != nil {
				t.Fatal(err)
			}
			if pid != 0 {
				handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
				if err == nil {
					state, waitErr := windows.WaitForSingleObject(handle, 5000)
					windows.CloseHandle(handle)
					if waitErr != nil || state != windows.WAIT_OBJECT_0 {
						t.Fatal("fixture guard still running", state, waitErr)
					}
				} else if err != windows.ERROR_INVALID_PARAMETER {
					t.Fatal(err)
				}
			}
			want := map[string]string{"ready": "ready", "failed": "guard_not_ready", "lost": "guard_exited", "released": "released", "closed-owner-alive": "closed"}[scenario]
			if parts[0] != want {
				t.Fatalf("got %q want %q", result, want)
			}
		})
	}
}

//go:build windows

package integration

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// Click only buttons in a dialog owned by this test's installer process.
// This exercises the actual native prompt, not a replacement confirmation API.
func clickRepairChoice(pid uint32, button uintptr) bool {
	user32 := windows.NewLazySystemDLL("user32.dll")
	found := false
	callback := windows.NewCallback(func(hwnd, unused uintptr) uintptr {
		var owner uint32
		user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			return 1
		}
		control, _, _ := user32.NewProc("GetDlgItem").Call(hwnd, button)
		if control == 0 {
			return 1
		}
		user32.NewProc("PostMessageW").Call(control, 0xf5, 0, 0)
		found = true
		return 0
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	return found
}

func TestNSISRepairPromptConsent(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("set WINONE_NSIS_EXE for owned native dialog tests")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name            string
		button          uintptr
		silent, invalid bool
		code            int
	}{
		{"accept", 6, false, false, 0}, {"decline", 7, false, false, 1}, {"accept_zh", 6, false, false, 0}, {"decline_zh", 7, false, false, 1}, {"silent", 0, true, false, 8}, {"mismatch", 0, false, true, 8},
		{"uninstall_accept", 6, false, false, 0}, {"uninstall_decline", 7, false, false, 1}, {"uninstall_accept_zh", 6, false, false, 0}, {"uninstall_decline_zh", 7, false, false, 1}, {"uninstall_silent", 0, true, false, 8}, {"uninstall_mismatch", 0, false, true, 8},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dir := t.TempDir()
			operation, selected, other := "install", "WinOneRepairMode", "WinOneUninstallRecovery"
			if strings.HasPrefix(scenario.name, "uninstall_") {
				operation, selected, other = "uninstall", "WinOneUninstallRecovery", "WinOneRepairMode"
			}
			validation := fmt.Sprintf("${If} $WinOneRecoveryOperation == \"%s\"\nClearErrors\n${Else}\nSetErrors\n${EndIf}", operation)
			if scenario.invalid {
				validation = "SetErrors"
			}
			language, id, question := "English", 1033, "The previous installation did not finish. Repair it now? Your settings will be kept."
			if strings.HasSuffix(scenario.name, "_zh") {
				language, id, question = "SimpChinese", 2052, "上次安装未完成。现在修复吗？你的设置会保留。"
			}
			uninstallQuestion := "The previous uninstall did not finish. Continue uninstalling? Your settings will be kept."
			if id == 2052 {
				uninstallQuestion = "上次卸载未完成。现在继续卸载吗？你的设置会保留。"
			}
			script := fmt.Sprintf(`Unicode true
Name "WinOne repair UI test"
OutFile "%s"
RequestExecutionLevel user
!include LogicLib.nsh
LoadLanguageFile "${NSISDIR}\Contrib\Language files\%s.nlf"
Var WinOneRepairMode
Var WinOneRecoveryOperation
Var WinOneUninstallRecovery
LangString RepairQuestion %d "%s"
LangString UninstallQuestion %d "%s"
Function WinOneCheckRecoveryRecord
%s
FunctionEnd
!include "%s"
Section
StrCpy $WinOneRepairMode 0
StrCpy $WinOneUninstallRecovery 0
Call WinOneOfferRecovery
${If} $%s == 1
${AndIf} $%s == 0
SetErrorLevel 0
${Else}
SetErrorLevel 8
${EndIf}
Quit
SectionEnd
`, filepath.Join(dir, "check.exe"), language, id, question, id, uninstallQuestion, validation, filepath.Join(root, "build", "windows", "repair-ui.nsh"), selected, other)
			path := filepath.Join(dir, "check.nsi")
			if err := os.WriteFile(path, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput(); err != nil {
				t.Fatalf("compile: %v %s", err, out)
			}
			args := []string{}
			if scenario.silent {
				args = append(args, "/S")
			}
			cmd := exec.Command(filepath.Join(dir, "check.exe"), args...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			clicked := false
			timer := time.NewTimer(10 * time.Second)
			defer timer.Stop()
			tick := time.NewTicker(25 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case err := <-done:
					code := 0
					if err != nil {
						if exit, ok := err.(*exec.ExitError); ok {
							code = exit.ExitCode()
						} else {
							t.Fatal(err)
						}
					}
					if code != scenario.code || (scenario.button != 0 && !clicked) {
						t.Fatalf("code=%d clicked=%v", code, clicked)
					}
					return
				case <-tick.C:
					if scenario.button != 0 && !clicked {
						clicked = clickRepairChoice(uint32(cmd.Process.Pid), scenario.button)
					}
				case <-timer.C:
					cmd.Process.Kill()
					<-done
					t.Fatal("owned repair prompt did not finish")
				}
			}
		})
	}
}

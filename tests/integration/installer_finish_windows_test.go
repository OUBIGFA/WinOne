//go:build windows

package integration

import (
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var finishProbeMu sync.Mutex
var finishProbe struct {
	pid              uint32
	label            string
	dialog, checkbox uintptr
}
var finishChildCallback = windows.NewCallback(func(window, unused uintptr) uintptr {
	var text [256]uint16
	overlapUser32.NewProc("GetWindowTextW").Call(window, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)))
	if windows.UTF16ToString(text[:]) == finishProbe.label {
		finishProbe.checkbox = window
		return 0
	}
	return 1
})
var finishWindowCallback = windows.NewCallback(func(window, unused uintptr) uintptr {
	var pid uint32
	overlapUser32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
	if pid != finishProbe.pid {
		return 1
	}
	overlapUser32.NewProc("EnumChildWindows").Call(window, finishChildCallback, 0)
	if finishProbe.checkbox != 0 {
		finishProbe.dialog = window
		return 0
	}
	return 1
})

func finishControls(pid uint32, label string) (uintptr, uintptr) {
	finishProbeMu.Lock()
	defer finishProbeMu.Unlock()
	finishProbe.pid, finishProbe.label, finishProbe.dialog, finishProbe.checkbox = pid, label, 0, 0
	overlapUser32.NewProc("EnumWindows").Call(finishWindowCallback, 0)
	return finishProbe.dialog, finishProbe.checkbox
}

func TestNSISFinishPageLaunchChoice(t *testing.T) {
	compiler, host, frontend := os.Getenv("WINONE_NSIS_EXE"), os.Getenv("WINONE_HOST_EXE"), os.Getenv("WINONE_FINISH_FIXTURE_EXE")
	if compiler == "" || host == "" || frontend == "" {
		t.Skip("explicit owned NSIS finish-page fixtures required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name, language, label                       string
		checked, silent, uninstall, failed, updated bool
	}{
		{"checked-zh", "SimpChinese", "立即启动 WinOne", true, false, false, false, false},
		{"unchecked-zh", "SimpChinese", "立即启动 WinOne", false, false, false, false, false},
		{"checked-en", "English", "Launch WinOne now", true, false, false, false, false},
		{"unchecked-en", "English", "Launch WinOne now", false, false, false, false, false},
		{"silent", "English", "Launch WinOne now", false, true, false, false, false},
		{"uninstall", "English", "Launch WinOne now", false, false, true, false, false},
		{"failed", "English", "Launch WinOne now", false, true, false, true, false},
		{"updated-checked", "SimpChinese", "立即启动 WinOne", true, false, false, false, true},
		{"updated-unchecked", "SimpChinese", "立即启动 WinOne", false, false, false, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dir := t.TempDir()
			installation := filepath.Join(dir, "installed")
			profile := filepath.Join(dir, "profile")
			marker := filepath.Join(dir, "launched.json")
			if err := os.Mkdir(installation, 0700); err != nil {
				t.Fatal(err)
			}
			for source, target := range map[string]string{host: filepath.Join(dir, "WinOne.Maintenance.exe"), frontend: filepath.Join(installation, "WinOne.exe")} {
				data, err := os.ReadFile(source)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(target, data, 0700); err != nil {
					t.Fatal(err)
				}
			}
			quote := func(s string) string { return strings.ReplaceAll(s, "$", "$$") }
			command := fmt.Sprintf(`"%s" --guard-installation "%s" --data-dir "%s" --pipe "\\.\pipe\WinOne-finish-%d"`, quote(filepath.Join(dir, "WinOne.Maintenance.exe")), quote(installation), quote(profile), time.Now().UnixNano())
			languageID := 1033
			if scenario.language == "SimpChinese" {
				languageID = 2052
			}
			recovery, fail := 0, ""
			if scenario.uninstall {
				recovery = 1
			}
			if scenario.failed {
				fail = "SetErrorLevel 7\nAbort"
			}
			if scenario.updated {
				// The real synchronous updater has exited before Finish is shown.
				fail = "Call WinOneReleaseGuard\nIfErrors failed\nStrCpy $WinOneUpdateMode 1"
			}
			script := fmt.Sprintf(`Unicode true
Name "WinOne finish page fixture"
OutFile "%s"
RequestExecutionLevel user
SetFont "Microsoft YaHei UI" 9
InstallDir "%s"
!include MUI2.nsh
!include "%s"
Var WinOneUninstallRecovery
Var WinOneUpdateMode
!define MUI_FINISHPAGE_RUN "$INSTDIR\WinOne.exe"
!define MUI_FINISHPAGE_RUN_TEXT "%s"
!define MUI_FINISHPAGE_RUN_FUNCTION WinOneFinishRun
!define MUI_PAGE_CUSTOMFUNCTION_SHOW WinOneFinishShow
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_LANGUAGE "%s"
LangString LaunchFailed %d "Launch failed"
!insertmacro WinOneGuardFunctions ""
!include "%s"
Function .onInit
  StrCpy $WinOneUninstallRecovery %d
  StrCpy $WinOneGuardCommand '%s'
  Call WinOneStartGuard
  IfErrors failed
  %s
  Return
failed:
  SetErrorLevel 8
  Abort
FunctionEnd
Function .onGUIEnd
  Call WinOneCloseGuardHandle
FunctionEnd
Section
SectionEnd
`, quote(filepath.Join(dir, "finish.exe")), quote(installation), quote(filepath.Join(root, "build/windows/maintenance.nsh")), scenario.label, scenario.language, languageID, quote(filepath.Join(root, "build/windows/finish.nsh")), recovery, command, fail)
			path := filepath.Join(dir, "finish.nsi")
			if err = os.WriteFile(path, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput(); err != nil {
				t.Fatalf("finish page compile: %v %s", err, output)
			}
			cmd := exec.Command(filepath.Join(dir, "finish.exe"))
			if scenario.silent {
				cmd.Args = append(cmd.Args, "/S")
			}
			cmd.Env = append(os.Environ(), "WINONE_FINISH_MARKER="+marker, "WINONE_FINISH_PROFILE="+profile)
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			defer func() { _ = cmd.Process.Kill() }()
			if !scenario.silent {
				var dialog, checkbox uintptr
				waitOverlap(t, "owned Finish page checkbox", func() bool {
					dialog, checkbox = finishControls(uint32(cmd.Process.Pid), scenario.label)
					return checkbox != 0
				})
				checked, _, _ := overlapUser32.NewProc("SendMessageW").Call(checkbox, 0xf0, 0, 0)
				visible, _, _ := overlapUser32.NewProc("IsWindowVisible").Call(checkbox)
				if scenario.uninstall {
					if checked != 0 || visible != 0 {
						t.Fatal("uninstall recovery offered launch")
					}
				} else {
					if checked != 1 || visible == 0 {
						t.Fatal("launch checkbox missing or not checked by default")
					}
					if !scenario.checked {
						overlapUser32.NewProc("SendMessageW").Call(checkbox, 0xf5, 0, 0)
					}
				}
				if shot, output := os.Getenv("WINONE_FINISH_SHOT_EXE"), os.Getenv("WINONE_FINISH_IMAGE_DIR"); shot != "" && output != "" {
					if err = os.MkdirAll(output, 0700); err != nil {
						t.Fatal(err)
					}
					capture := exec.Command(shot, strconv.Itoa(cmd.Process.Pid), filepath.Join(output, scenario.name+".png"))
					capture.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
					if data, err := capture.CombinedOutput(); err != nil {
						t.Fatalf("Finish page screenshot: %v %s", err, data)
					}
				}
				button, _, _ := overlapUser32.NewProc("GetDlgItem").Call(dialog, 1)
				if button == 0 {
					t.Fatal("Finish action missing")
				}
				overlapUser32.NewProc("PostMessageW").Call(button, 0xf5, 0, 0)
			}
			select {
			case err = <-done:
				if (err != nil) != scenario.failed {
					t.Fatal("finish outcome", err)
				}
			case <-time.After(12 * time.Second):
				t.Fatal("Finish did not close installer")
			}
			if scenario.checked {
				waitOverlap(t, "launched fixture", func() bool { _, err := os.Stat(marker); return err == nil })
				var result struct {
					RuntimeAdmitted bool   `json:"runtimeAdmitted"`
					Error           string `json:"error"`
				}
				data, _ := os.ReadFile(marker)
				if err = json.Unmarshal(data, &result); err != nil || !result.RuntimeAdmitted {
					t.Fatal("launched before maintenance ended", result, err)
				}
			} else {
				if _, err = os.Stat(marker); !os.IsNotExist(err) {
					t.Fatal("WinOne launched without a checked Finish choice", err)
				}
			}
		})
	}
}

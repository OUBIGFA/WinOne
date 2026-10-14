//go:build windows

package distribution

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNSISUpdateProgressUsesActivityAndMarksFailure(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("explicit isolated NSIS progress test")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, "WinOne-progress-fixture.exe")
	source := filepath.Join(directory, "progress.nsi")
	script := fmt.Sprintf(`Unicode true
!include LogicLib.nsh
Name "WinOne progress fixture"
XPStyle on
OutFile "%s"
RequestExecutionLevel user
SilentInstall silent
Var mui.InstFilesPage.ProgressBar
!include "%s"
Section
  System::Call 'comctl32::InitCommonControls()'
  System::Call 'user32::CreateWindowExW(i 0, w "msctls_progress32", p 0, i 0, i 0, i 0, i 100, i 20, p 0, p 0, p 0, p 0) p.r0'
  StrCpy $mui.InstFilesPage.ProgressBar $0
  StrCpy $1 11
  IntCmp $0 0 failed
  Call WinOneBeginUpdateProgress
  StrCpy $1 12
  System::Call 'user32::GetWindowLongW(p $mui.InstFilesPage.ProgressBar, i -16) i.r0'
  IntOp $0 $0 & 8
  IntCmp $0 8 +2
  Goto failed
  ; An old extraction position must not be displayed as update completion.
  Call WinOneFailUpdateProgress
  StrCpy $1 13
  System::Call 'user32::GetWindowLongW(p $mui.InstFilesPage.ProgressBar, i -16) i.r0'
  IntOp $0 $0 & 8
  IntCmp $0 0 +2
  Goto failed
  SendMessage $mui.InstFilesPage.ProgressBar 0x411 0 0 $0
  StrCpy $1 14
  IntCmp $0 2 +2
  Goto failed
  SendMessage $mui.InstFilesPage.ProgressBar 0x408 0 0 $0
  StrCpy $1 15
  IntCmp $0 0 +2
  Goto failed
  System::Call 'user32::DestroyWindow(p $mui.InstFilesPage.ProgressBar)'
  SetErrorLevel 0
  Quit
failed:
  SetErrorLevel $1
  Quit
SectionEnd
`, executable, filepath.Join(root, "build", "windows", "progress.nsh"))
	if err := os.WriteFile(source, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", source).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v %s", err, output)
	}
	if output, err := exec.Command(executable).CombinedOutput(); err != nil {
		t.Fatalf("native progress: %v %s", err, output)
	}
}

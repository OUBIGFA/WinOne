//go:build windows

package integration

import (
	"context"
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
)

func TestInstallerFirstPageFollowsWindowsLanguageWithoutSelector(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("explicit owned NSIS language fixture required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	report := filepath.Join(dir, "language.txt")
	exe := filepath.Join(dir, "language-probe.exe")
	license := filepath.Join(dir, "license.txt")
	if err = os.WriteFile(license, []byte("fixture license"), 0600); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`Unicode true
Name "WinOne language fixture"
OutFile "%s"
RequestExecutionLevel user
!include MUI2.nsh
!define MUI_PAGE_CUSTOMFUNCTION_SHOW FirstPage
!insertmacro MUI_PAGE_LICENSE "%s"
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"
!include "%s"
!insertmacro WinOneLanguageFunctions ""
Function .onInit
 Call WinOneInitializeLanguage
FunctionEnd
Function FirstPage
 FileOpen $0 "%s" w
 FileWriteUTF16LE /BOM $0 "$LANGUAGE"
 FileClose $0
FunctionEnd
Section
SectionEnd
`, exe, license, filepath.Join(root, "build/windows/language.nsh"), report)
	path := filepath.Join(dir, "language.nsi")
	if err = os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v %s", err, output)
	}
	command := exec.Command(exe)
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	// Observe the first page, rather than waiting for an installer to exit from
	// a page-show callback. No install action is executed by this fixture.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(report); err == nil && info.Size() >= 10 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	raw, err := os.ReadFile(report)
	if err != nil {
		t.Fatal("first page did not appear", err)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	text := strings.TrimPrefix(string(utf16.Decode(units)), "\ufeff")
	language, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	want := "1033"
	if language&0x3ff == 4 {
		want = "2052"
	}
	if text != want {
		t.Fatalf("first page language %q, want %s", text, want)
	}
}

func TestSetupAndUninstallShareSupportedLanguageMapping(t *testing.T) {
	compiler := os.Getenv("WINONE_NSIS_EXE")
	if compiler == "" {
		t.Skip("explicit NSIS fixture required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "language-map.exe")
	uninstaller := filepath.Join(dir, "remove-fixture.exe")
	report := filepath.Join(dir, "install.txt")
	unreport := filepath.Join(dir, "uninstall.txt")
	calls := func(prefix string) string {
		var out strings.Builder
		for _, language := range []int{2052, 4100, 1028, 3076, 1033, 2057, 1031, 0} {
			fmt.Fprintf(&out, "Push %d\nCall %sWinOneApplyUILanguage\nFileWrite $1 \"$LANGUAGE;\"\n", language, prefix)
		}
		return out.String()
	}
	script := fmt.Sprintf(`Unicode true
Name "WinOne language mapping fixture"
OutFile "%s"
RequestExecutionLevel user
!include MUI2.nsh
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"
!include "%s"
!insertmacro WinOneLanguageFunctions ""
!insertmacro WinOneLanguageFunctions "un."
Function .onInit
 Call WinOneInitializeLanguage
 FileOpen $1 "%s" w
 FileWrite $1 "$LANGUAGE|"
 %s
 FileClose $1
FunctionEnd
Function un.onInit
 Call un.WinOneInitializeLanguage
 FileOpen $1 "%s" w
 FileWrite $1 "$LANGUAGE|"
 %s
 FileClose $1
 SetErrorLevel 0
 Quit
FunctionEnd
Section
 WriteUninstaller "%s"
SectionEnd
Section "Uninstall"
SectionEnd
`, exe, filepath.Join(root, "build/windows/language.nsh"), report, calls(""), unreport, calls("un."), uninstaller)
	path := filepath.Join(dir, "map.nsi")
	if err = os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(compiler, "/V2", "/INPUTCHARSET", "UTF8", path).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v %s", err, output)
	}
	for _, program := range []string{exe, uninstaller} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		command := exec.CommandContext(ctx, program, "/S")
		if program == uninstaller {
			// Run the owned uninstaller in place, retaining its real process;
			// the usual self-copy bootstrap exits before its child completes.
			command.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(program) + " /S _?=" + dir, HideWindow: true}
		}
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("language init: %v %s", err, output)
		}
	}
	language, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	initial := "1033"
	if language&0x3ff == 4 {
		initial = "2052"
	}
	want := initial + "|2052;2052;2052;2052;1033;1033;1033;1033;"
	for _, path := range []string{report, unreport} {
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != want {
			t.Fatalf("language result: %q, want %q (%v)", raw, want, err)
		}
	}
}

// Open only the first page of an explicitly supplied final installer. No Next
// or Install action is sent; the owned process is closed after observing it.
func TestPackagedInstallerFirstPageLanguage(t *testing.T) {
	exe := os.Getenv("WINONE_INSTALLER_LANGUAGE_EXE")
	if exe == "" {
		t.Skip("explicit installer first-page check required")
	}
	language, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	label := "I &Agree"
	if language&0x3ff == 4 {
		label = "我接受(&I)"
	}
	command := exec.Command(exe)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	waitOverlap(t, "system-language first page", func() bool { _, button := finishControls(uint32(command.Process.Pid), label); return button != 0 })
}

//go:build windows

package ai

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestAgentEnvironmentIsPrivateAndDoesNotChangeParent(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "parent-test-key")
	original := os.Getenv("USERPROFILE")
	root := t.TempDir()
	env := map[string]string{}
	for _, entry := range isolatedEnvironment(root) {
		key, value, _ := strings.Cut(entry, "=")
		env[strings.ToUpper(key)] = value
	}
	if env["OPENAI_API_KEY"] != "" || env["USERPROFILE"] != filepath.Join(root, "home") || env["APPDATA"] != filepath.Join(root, "home/AppData/Roaming") {
		t.Fatal("child inherited another client's account environment")
	}
	if os.Getenv("USERPROFILE") != original || os.Getenv("OPENAI_API_KEY") != "parent-test-key" {
		t.Fatal("parent environment was changed")
	}
}

func TestAIConsoleFlagsPreserveGUIStartupAndProcessOptions(t *testing.T) {
	command := exec.Command("unused-fixture.exe")
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	hideConsole(command)
	if command.SysProcAttr.HideWindow || command.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 || command.SysProcAttr.CreationFlags&windows.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatalf("AI worker must hide only its console, without SW_HIDE or dropping existing flags: %+v", command.SysProcAttr)
	}
}

// Explicit system probe: creates only a short-lived WinOne-owned test window.
// SW_SHOWDEFAULT follows the startup contract used by native GUI children.
func TestAIWorkerAllowsDedicatedLoginWindow(t *testing.T) {
	if os.Getenv("WINONE_AI_WINDOW_TEST") != "1" {
		t.Skip("set WINONE_AI_WINDOW_TEST=1 for the isolated window check")
	}
	command := exec.Command(os.Args[0], "-test.run=^TestAIWindowStartupProbeProcess$")
	command.Env = append(os.Environ(), "WINONE_AI_WINDOW_PROBE=1")
	hideConsole(command)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("window probe failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "visible=true") {
		t.Fatalf("dedicated GUI window was hidden: %s", output)
	}
}

func TestAIWindowStartupProbeProcess(t *testing.T) {
	if os.Getenv("WINONE_AI_WINDOW_PROBE") != "1" {
		t.Skip("subprocess-only fixture")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32 := windows.NewLazySystemDLL("user32.dll")
	class := windows.StringToUTF16Ptr("STATIC")
	title := windows.StringToUTF16Ptr("WinOne isolated login visibility check")
	window, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), 0x00cf0000, 80, 80, 360, 160, 0, 0, 0, 0)
	if window == 0 {
		t.Fatal(err)
	}
	defer user32.NewProc("DestroyWindow").Call(window)
	user32.NewProc("ShowWindow").Call(window, 10) // SW_SHOWDEFAULT, not a forced override of Host intent.
	visible, _, _ := user32.NewProc("IsWindowVisible").Call(window)
	if visible != 0 {
		os.Stdout.WriteString("visible=true\n")
	} else {
		os.Stdout.WriteString("visible=false\n")
	}
}

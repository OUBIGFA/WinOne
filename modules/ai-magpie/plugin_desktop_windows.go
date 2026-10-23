//go:build windows

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"syscall"
	"unsafe"
)

// Login uses the interactive desktop. Request workers and their descendants
// start on a private desktop, so even a new browser window cannot flash or
// steal focus. Headed Chromium retains its normal rendering and sandbox.
// This bootstrap inherits Host's limited token, pipes and owned Job.
func init() {
	if os.Getenv("WINONE_AI_BACKGROUND_DESKTOP") != "1" {
		return
	}
	code, err := runOnPluginDesktop()
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": "无法启动后台插件浏览器，请重试"})
		fmt.Fprintln(os.Stderr, "plugin desktop:", err)
		os.Exit(1)
	}
	os.Exit(int(code))
}

func runOnPluginDesktop() (uint32, error) {
	// Host writes only after assigning this process to its kill-on-close Job.
	// Wait for that admission before spawning anything; job assignment is not
	// retroactive for an already-running child.
	var first [1]byte
	if _, err := io.ReadFull(os.Stdin, first[:]); err != nil {
		return 0, err
	}
	childInput, forward, err := os.Pipe()
	if err != nil {
		return 0, err
	}
	defer childInput.Close()
	defer forward.Close()
	nameBytes := make([]byte, 16)
	if _, err := rand.Read(nameBytes); err != nil {
		return 0, err
	}
	name := "WinOneAI_" + hex.EncodeToString(nameBytes)
	user32 := windows.NewLazySystemDLL("user32.dll")
	desktopName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	// Preserve the interactive desktop's AppContainer grants and integrity
	// label. A default process DACL alone makes Chromium's network service
	// crash when its restricted token opens the new desktop.
	current, _, err := user32.NewProc("GetThreadDesktop").Call(uintptr(windows.GetCurrentThreadId()))
	if current == 0 {
		return 0, err
	}
	security, err := windows.GetSecurityInfo(windows.Handle(current), windows.SE_WINDOW_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.LABEL_SECURITY_INFORMATION)
	if err != nil {
		return 0, err
	}
	attributes := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: security}
	desktop, _, err := user32.NewProc("CreateDesktopW").Call(uintptr(unsafe.Pointer(desktopName)), 0, 0, 0, 0x01ff, uintptr(unsafe.Pointer(&attributes)))
	if desktop == 0 {
		return 0, err
	}
	defer user32.NewProc("CloseDesktop").Call(desktop)
	// The child flag prevents recursion. A desktop stays alive while its
	// browsers use it, even after this short-lived bootstrap closes its handle.
	if err := os.Setenv("WINONE_AI_BACKGROUND_DESKTOP", name); err != nil {
		return 0, err
	}
	executable, err := os.Executable()
	if err != nil {
		return 0, err
	}
	app, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return 0, err
	}
	command, err := windows.UTF16PtrFromString(syscall.EscapeArg(executable))
	if err != nil {
		return 0, err
	}
	handles := []windows.Handle{windows.Handle(childInput.Fd()), windows.Handle(os.Stdout.Fd()), windows.Handle(os.Stderr.Fd())}
	for _, h := range handles {
		if err := windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			return 0, err
		}
	}
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})), Desktop: desktopName, Flags: windows.STARTF_USESTDHANDLES, StdInput: handles[0], StdOutput: handles[1], StdErr: handles[2]}
	var child windows.ProcessInformation
	if err := windows.CreateProcess(app, command, nil, nil, true, windows.CREATE_NO_WINDOW, nil, nil, &startup, &child); err != nil {
		return 0, err
	}
	windows.CloseHandle(child.Thread)
	defer windows.CloseHandle(child.Process)
	childInput.Close()
	go func() {
		defer forward.Close()
		// The child may exit without consuming more input; its exit status is
		// authoritative. The original first byte must reach its JSON decoder.
		_, _ = io.Copy(forward, io.MultiReader(bytes.NewReader(first[:]), os.Stdin))
	}()
	if _, err := windows.WaitForSingleObject(child.Process, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(child.Process, &code); err != nil {
		return 0, err
	}
	return code, nil
}

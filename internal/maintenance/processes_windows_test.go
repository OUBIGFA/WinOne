//go:build windows

package maintenance

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestProductProcessScopeHelper(t *testing.T) {
	if os.Getenv("WINONE_PROCESS_SCOPE_HELPER") != "1" {
		return
	}
	fmt.Println("ready")
	var one [1]byte
	os.Stdin.Read(one[:])
}

func TestMaintenanceProcessCheckDistinguishesInstallations(t *testing.T) {
	for _, name := range []string{"WinOne.Host.exe", "WinOne.WinTray.Recovery.exe", "WinOne.AI.exe", "WinOne.Capture.Regions.exe"} {
		t.Run(name, func(t *testing.T) { checkMaintenanceProcessScope(t, name, true) })
	}
}

func TestIndependentWinTrayIsNotAWinOneMaintenanceProcess(t *testing.T) {
	for _, name := range []string{"WinTray.exe", "WINTRAY.EXE"} {
		if IsProductProcessName(name) {
			t.Fatalf("independent product %s entered WinOne maintenance scope", name)
		}
	}
	if !IsProductProcessName("WinOne.WinTray.exe") {
		t.Fatal("embedded program manager escaped WinOne maintenance")
	}
	checkMaintenanceProcessScope(t, "WinTray.exe", false)
}

func TestMaintenanceProcessLocationIgnoresAnExitedSnapshotEntry(t *testing.T) {
	child := exec.Command(os.Args[0], "-test.run=^TestProductProcessScopeHelper$")
	child.Env = append(os.Environ(), "WINONE_PROCESS_SCOPE_HELPER=1")
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		input.Close()
		if child.ProcessState == nil {
			child.Wait()
		}
	})
	if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatal(line, err)
	}
	// Keep the exited process object (and PID) alive exactly as an outstanding
	// Toolhelp entry can outlive the process that was observed in its snapshot.
	pin, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(child.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { windows.CloseHandle(pin) })
	for _, handle := range []windows.Handle{pin, 0} {
		if err := processErrorUnlessExited(handle, windows.ERROR_ACCESS_DENIED); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			t.Fatal("live or unverified process access failure was suppressed", handle, err)
		}
	}
	input.Close()
	if err = child.Wait(); err != nil {
		t.Fatal(err)
	}
	if state, err := windows.WaitForSingleObject(pin, 0); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("fixture has not exited", state, err)
	}
	if inside, err := processInInstallation(uint32(child.Process.Pid), t.TempDir()); err != nil || inside {
		t.Fatal("an exited snapshot entry blocked maintenance", inside, err)
	}
}

func checkMaintenanceProcessScope(t *testing.T, name string, product bool) {
	directory := t.TempDir()
	path := filepath.Join(directory, name)
	data, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(path, "-test.run=^TestProductProcessScopeHelper$")
	child.Env = append(os.Environ(), "WINONE_PROCESS_SCOPE_HELPER=1")
	input, _ := child.StdinPipe()
	output, _ := child.StdoutPipe()
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { input.Close(); child.Wait() }()
	if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatal(line, err)
	}
	if err = RequireStoppedAt(t.TempDir()); err != nil {
		t.Fatal("another installation blocked maintenance", err)
	}
	err = RequireStoppedAt(directory)
	if !product {
		if err != nil {
			t.Fatal("independent WinTray blocked WinOne maintenance", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), strconv.Itoa(child.Process.Pid)) {
		t.Fatal("the actual installation's process was not reported", err)
	}
}

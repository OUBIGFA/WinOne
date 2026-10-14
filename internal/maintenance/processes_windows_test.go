//go:build windows

package maintenance

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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

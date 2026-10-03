//go:build windows

package maintenance

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestLockProcessHelper(t *testing.T) {
	dir := os.Getenv("WINONE_LOCK_TEST_DIRECTORY")
	if dir == "" {
		return
	}
	lease, err := AcquireRuntime(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	fmt.Println("locked")
	var one [1]byte
	_, _ = os.Stdin.Read(one[:])
}

func TestSharedRuntimeAndExclusiveMaintenanceAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockProcessHelper$")
	cmd.Env = append(os.Environ(), "WINONE_LOCK_TEST_DIRECTORY="+dir)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("helper not ready: %q %v", line, err)
	}
	shared, err := AcquireRuntime(dir)
	if err != nil {
		t.Fatal("second runtime rejected", err)
	}
	if lock, err := AcquireMaintenance(dir); err == nil {
		lock.Close()
		t.Fatal("maintenance overlapped runtime")
	}
	shared.Close()
	input.Close()
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	exclusive, err := AcquireMaintenance(dir)
	if err != nil {
		t.Fatal("process exit retained lock", err)
	}
	defer exclusive.Close()
	if lock, err := AcquireRuntime(dir); err == nil {
		lock.Close()
		t.Fatal("runtime entered maintenance")
	}
	if lock, err := AcquireMaintenance(dir); err == nil {
		lock.Close()
		t.Fatal("two maintenance owners")
	}
	exclusive.Close()
	if err = exclusive.Close(); err != nil {
		t.Fatal("close not idempotent", err)
	}
	shared, err = AcquireRuntime(dir)
	if err != nil {
		t.Fatal("runtime could not restart", err)
	}
	shared.Close()
}

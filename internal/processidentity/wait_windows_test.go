//go:build windows

package processidentity

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWaitProcessHelper(t *testing.T) {
	if os.Getenv("WINONE_PROCESS_WAIT_HELPER") != "1" {
		return
	}
	var input [1]byte
	if _, err := os.Stdin.Read(input[:]); err != nil {
		os.Exit(8)
	}
	os.Exit(7)
}

func TestWaitObservesActualExitWithoutTerminatingOnCancellation(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestWaitProcessHelper$")
	cmd.Env = append(os.Environ(), "WINONE_PROCESS_WAIT_HELPER=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { input.Close(); cmd.Process.Kill(); cmd.Wait() }()
	process, err := Open(uint32(cmd.Process.Pid), exe)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = process.Wait(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if exited, err := process.Exited(); err != nil || exited {
		t.Fatal("cancellation ended the observed process", exited, err)
	}
	if _, err = input.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = process.Wait(ctx); err == nil || !strings.Contains(err.Error(), "7") {
		t.Fatal("actual exit code was lost", err)
	}
}

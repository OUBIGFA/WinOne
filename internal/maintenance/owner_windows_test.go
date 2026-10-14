//go:build windows

package maintenance

import (
	"bufio"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestInstallerOwnerHelper(t *testing.T) {
	if os.Getenv("WINONE_OWNER_TEST") != "1" {
		return
	}
	fmt.Println("ready")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}

func TestInstallerGuardSignalsOnlyAfterPreparationAndWaitsForOwner(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestInstallerOwnerHelper$")
	cmd.Env = append(os.Environ(), "WINONE_OWNER_TEST=1")
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { in.Close(); cmd.Process.Kill(); cmd.Wait() }()
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatal(line, err)
	}
	name := fmt.Sprintf("Local\\WinOne-maintenance-test-%d", time.Now().UnixNano())
	key, _ := windows.UTF16PtrFromString(name)
	event, err := windows.CreateEvent(nil, 1, 0, key)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(event)
	failure := errors.New("restore failed")
	if err := HoldUntilOwnerExit(uint32(cmd.Process.Pid), name, func() error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if state, _ := windows.WaitForSingleObject(event, 0); state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("failed preparation signalled ready")
	}
	done := make(chan error, 1)
	go func() { done <- HoldUntilOwnerExit(uint32(cmd.Process.Pid), name, func() error { return nil }) }()
	if state, err := windows.WaitForSingleObject(event, 5000); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("not ready", state, err)
	}
	select {
	case err := <-done:
		t.Fatal("guard exited before owner", err)
	default:
	}
	in.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("guard retained dead owner")
	}
}

func TestInstallerGuardReleasesBeforeFinishPageLaunch(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestInstallerOwnerHelper$")
	cmd.Env = append(os.Environ(), "WINONE_OWNER_TEST=1")
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { in.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatal(line, err)
	}
	name := fmt.Sprintf("Local\\WinOne-finish-test-%d", time.Now().UnixNano())
	key, _ := windows.UTF16PtrFromString(name)
	ready, err := windows.CreateEvent(nil, 1, 0, key)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(ready)
	releaseName := name + "-release"
	key, _ = windows.UTF16PtrFromString(releaseName)
	release, err := windows.CreateEvent(nil, 1, 0, key)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(release)
	owner, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(owner)
	done := make(chan error, 1)
	go func() {
		done <- HoldUntilOwnerExit(uint32(cmd.Process.Pid), name, func() error { return nil }, releaseName)
	}()
	if state, err := windows.WaitForSingleObject(ready, 5000); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("guard not ready", state, err)
	}
	select {
	case err := <-done:
		t.Fatal("guard released before finish", err)
	default:
	}
	if err := windows.SetEvent(release); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("finish page still holds the launch-blocking maintenance lease")
	}
	if state, err := windows.WaitForSingleObject(owner, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("guard required the installer to exit", state, err)
	}
}

//go:build windows

package processidentity

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestIdentityChild(t *testing.T) {
	if os.Getenv("WINONE_IDENTITY_CHILD") != "1" {
		t.Skip("subprocess fixture")
	}
	fmt.Println("ready")
	var command string
	_, _ = fmt.Fscanln(os.Stdin, &command)
}
func TestIdentityHandleObservesExitWithoutPIDReuse(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestIdentityChild$")
	cmd.Env = append(os.Environ(), "WINONE_IDENTITY_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		input.Close()
		if !waited {
			_ = cmd.Wait()
		}
	}()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatal(line, err)
	}
	process, err := Open(uint32(cmd.Process.Pid), executable)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	if _, err = fmt.Fprintln(input, "exit"); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	waited = true
	if err != nil {
		t.Fatal(err)
	}
	if exited, err := process.Exited(); err != nil || !exited {
		t.Fatal(exited, err)
	}
	if reopened, err := Open(uint32(cmd.Process.Pid), executable); !errors.Is(err, ErrExited) {
		if reopened != nil {
			reopened.Close()
		}
		t.Fatal("exited process treated as ready", err)
	}
}

//go:build windows

package processidentity

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenVerifiesPathAndActualToken(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process, err := Open(uint32(os.Getpid()), executable)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	if process.Elevated != windows.GetCurrentProcessToken().IsElevated() {
		t.Fatal("wrong token level")
	}
	if exited, err := process.Exited(); err != nil || exited {
		t.Fatal(exited, err)
	}
	if _, err = Open(uint32(os.Getpid()), filepath.Join(t.TempDir(), "other.exe")); err == nil {
		t.Fatal("wrong image accepted")
	}
	if _, err = Open(0, executable); err == nil {
		t.Fatal("empty PID accepted")
	}
}

//go:build windows

package processidentity

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

// Task Scheduler accepts an installation reached through a junction (its
// validation resolves links). Process verification must use the same identity.
func TestOpenAcceptsJunctionToSameInstallation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "installation-alias")
	command := exec.Command("cmd.exe")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW, CmdLine: "cmd.exe /d /c mklink /J \"" + alias + "\" \"" + filepath.Dir(executable) + "\""}
	output, createErr := command.CombinedOutput()
	if createErr != nil {
		t.Fatalf("create isolated junction: %v %s", createErr, output)
	}
	process, err := Open(uint32(os.Getpid()), filepath.Join(alias, filepath.Base(executable)))
	if err != nil {
		resolved, resolveErr := filepath.EvalSymlinks(filepath.Join(alias, filepath.Base(executable)))
		t.Fatalf("same installation through a junction: %v (actual=%q resolved=%q error=%v)", err, executable, resolved, resolveErr)
	}
	process.Close()
}

func TestOpenAcceptsExtendedLengthSpellingOfSameExecutable(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process, err := Open(uint32(os.Getpid()), `\\?\`+executable)
	if err != nil {
		t.Fatalf("same executable with extended path prefix: %v", err)
	}
	process.Close()
}

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
	if _, err = Open(uint32(os.Getpid()), filepath.Join(t.TempDir(), "other.exe")); !errors.Is(err, ErrDifferentInstallation) {
		t.Fatal("different installation must be distinguishable from inaccessible identity", err)
	}
	if _, err = Open(0, executable); err == nil {
		t.Fatal("empty PID accepted")
	}
}

func TestOpenRelatedVerifiesExecutableNameAndCurrentUser(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process, err := OpenRelated(uint32(os.Getpid()), filepath.Base(executable))
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	if process.Elevated != windows.GetCurrentProcessToken().IsElevated() {
		t.Fatal("wrong token")
	}
	for _, name := range []string{"another-product.exe", "", executable, "../" + filepath.Base(executable)} {
		if p, err := OpenRelated(uint32(os.Getpid()), name); err == nil {
			p.Close()
			t.Fatalf("unrelated executable accepted: %q", name)
		}
	}
}

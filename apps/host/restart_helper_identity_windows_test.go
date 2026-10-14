//go:build windows

package main

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"winone/internal/processidentity"
)

func TestAdministratorHelperIdentity(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if mode := os.Getenv("WINONE_HELPER_IDENTITY_TEST"); mode != "" {
		process, err := openAdministratorHelper(uint32(os.Getpid()), filepath.Join(filepath.Dir(executable), "WinOne.exe"))
		if mode == "denied" {
			if !errors.Is(err, processidentity.ErrDifferentInstallation) {
				t.Fatal("unrelated binary accepted", err)
			}
			return
		}
		if err != nil {
			t.Fatal("sibling helper rejected", err)
		}
		defer process.Close()
		if process.Elevated != windows.GetCurrentProcessToken().IsElevated() {
			t.Fatal("helper token identity changed")
		}
		if other, err := openAdministratorHelper(uint32(os.Getpid()), filepath.Join(t.TempDir(), "WinOne.exe")); err == nil {
			other.Close()
			t.Fatal("other installation accepted")
		}
		return
	}
	for _, test := range []struct{ name, mode string }{{"WinOne.Host.exe", "allowed"}, {"WinOne.exe", "allowed"}, {"Unrelated.exe", "denied"}} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.name)
			source, err := os.Open(executable)
			if err != nil {
				t.Fatal(err)
			}
			target, err := os.Create(path)
			if err != nil {
				source.Close()
				t.Fatal(err)
			}
			_, copyErr := io.Copy(target, source)
			closeErr := target.Close()
			source.Close()
			if copyErr != nil || closeErr != nil {
				t.Fatal(copyErr, closeErr)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, path, "-test.run=^TestAdministratorHelperIdentity$")
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
			command.Env = append(os.Environ(), "WINONE_HELPER_IDENTITY_TEST="+test.mode)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
		})
	}
}

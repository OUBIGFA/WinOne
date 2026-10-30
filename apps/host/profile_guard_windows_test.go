//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTemporaryCopyCannotRunAgainstDefaultProfile(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	profile := filepath.Join(local, "WinOne")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The test binary's directory cannot own a WinOne installer registration,
	// so the guard must refuse the default profile without an explicit data-dir.
	if err := guardDefaultProfile(profile, false); err == nil {
		t.Fatal("temporary copy opened the default profile without --data-dir")
	}
	explicit := errors.Is(guardDefaultProfile(profile, true), nil)
	if !explicit {
		t.Fatal("explicit --data-dir must keep working")
	}
	isolated := errors.Is(guardDefaultProfile(t.TempDir(), false), nil)
	if !isolated {
		t.Fatal("isolated profile must not be blocked")
	}
	if err := os.Chdir(filepath.Dir(previous)); err != nil {
		t.Fatal(err)
	}
}

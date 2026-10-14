//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProgramMetadataCacheSurvivesReopenAndInvalidatesChangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.exe")
	os.WriteFile(path, []byte("original"), 0600)
	calls := 0
	read := func(string) (ProgramMetadata, error) {
		calls++
		return ProgramMetadata{Name: "Example", Icon: []byte{1, 2, 3}}, nil
	}
	cache := filepath.Join(dir, "cache")
	for range 2 {
		value, err := cachedProgramInfo(cache, path, read)
		if err != nil || len(value.Icon) != 3 {
			t.Fatal(value, err)
		}
	}
	if calls != 1 {
		t.Fatal("Repeated extraction", calls)
	}
	os.WriteFile(path, []byte("replacement file"), 0600)
	if _, err := cachedProgramInfo(cache, path, read); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("Changed file reused stale metadata")
	}
}

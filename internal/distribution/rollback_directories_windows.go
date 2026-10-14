//go:build windows

package distribution

import (
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Drop only empty package directories that the previous version did not own.
// This is repeatable after interruption; user files are never recursively
// removed, and a new file arriving during cleanup keeps its directory alive.
func removeRolledBackDirectories(root *os.Root, plan UpgradePlan) error {
	old, added := map[string]bool{}, map[string]string{}
	parents := func(name string, visit func(string)) {
		for directory := path.Dir(name); directory != "."; directory = path.Dir(directory) {
			visit(directory)
		}
	}
	for _, file := range plan.Files {
		if file.PreviousSHA256 != "" {
			name := file.PreviousPath
			if name == "" {
				name = file.Path
			}
			parents(name, func(dir string) { old[strings.ToLower(dir)] = true })
		}
	}
	for _, file := range plan.Files {
		if file.NextSHA256 != "" {
			parents(file.Path, func(dir string) {
				if !old[strings.ToLower(dir)] {
					added[strings.ToLower(dir)] = dir
				}
			})
		}
	}
	directories := make([]string, 0, len(added))
	for _, dir := range added {
		directories = append(directories, dir)
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		missing := false
		parts := strings.Split(directory, "/")
		for i := range parts {
			info, err := root.Lstat(filepath.Join(parts[:i+1]...))
			if errors.Is(err, os.ErrNotExist) {
				missing = true
				break
			}
			if err != nil {
				return err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("rollback directory is not a real directory")
			}
		}
		if missing {
			continue
		}
		dir, err := root.Open(filepath.FromSlash(directory))
		if err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(1)
		closeErr := dir.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if len(entries) != 0 {
			continue
		}
		if err = root.Remove(filepath.FromSlash(directory)); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, windows.ERROR_DIR_NOT_EMPTY) {
			return err
		}
	}
	return nil
}

//go:build windows

package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/recycle"
)

type legacyLog struct{ path, source string }

func legacyLogs(root *os.Root) ([]legacyLog, error) {
	var logs []legacyLog
	for _, id := range []string{"capture", "always-on-top", "wintray", "selection"} {
		logs = append(logs, legacyLog{filepath.Join("modules", id, "worker.log"), id})
	}
	for _, name := range []string{"wintray.log", "wintray.log.old"} {
		logs = append(logs, legacyLog{filepath.Join("modules", "wintray", name), "wintray"})
	}
	native := filepath.Join("modules", "always-on-top", "native")
	dir, err := root.Open(native)
	if errors.Is(err, os.ErrNotExist) {
		return logs, nil
	}
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		known := name == "native.log"
		if strings.HasPrefix(name, "native_") && strings.HasSuffix(name, ".log") {
			_, err := time.Parse("2006-01-02", strings.TrimSuffix(strings.TrimPrefix(name, "native_"), ".log"))
			known = err == nil
		}
		if known && !entry.IsDir() {
			logs = append(logs, legacyLog{filepath.Join(native, name), "always-on-top"})
		}
	}
	return logs, nil
}

// ConsolidateLegacy keeps useful tails of known diagnostics in the one product
// log, then recycles their original files. Runtime databases are never globbed.
func ConsolidateLegacy(ctx context.Context, directory string) error {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	directory = absolute
	return consolidateLegacy(directory, func(path string, check func() error) error {
		_, err := recycle.CheckedFile(ctx, path, check)
		return err
	})
}

func consolidateLegacy(directory string, retire func(string, func() error) error) error {
	root, err := os.OpenRoot(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	logs, err := legacyLogs(root)
	if err != nil {
		return err
	}
	var failures []error
	for _, entry := range logs {
		info, err := root.Lstat(entry.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			failures = append(failures, fmt.Errorf("legacy log is not a regular file: %s", entry.path))
			continue
		}
		file, err := root.Open(entry.path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		// Match the existing viewer's 64 KiB tail bound; historical diagnostic
		// output must not displace the entire current product log during upgrade.
		data, readErr := io.ReadAll(io.NewSectionReader(file, max(int64(0), info.Size()-(64<<10)), 64<<10))
		closeErr := file.Close()
		err = errors.Join(readErr, closeErr)
		if err == nil && len(data) > 0 {
			writer := &Writer{directory: directory, source: entry.source}
			_, err = writer.Write(data)
			err = errors.Join(err, writer.Close())
		}
		if err == nil {
			check := func() error {
				current, err := root.Lstat(entry.path)
				if err != nil {
					return err
				}
				if !os.SameFile(info, current) || current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) {
					return errors.New("legacy log changed during migration")
				}
				return nil
			}
			err = retire(filepath.Join(directory, entry.path), check)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", entry.path, err))
		}
	}
	return errors.Join(failures...)
}

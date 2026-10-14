// Package diagnostics reads and prunes only known product logs without starting modules.
package diagnostics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type ModuleLogEntry struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Truncated bool   `json:"truncated"`
	Text      string `json:"text"`
}
type ModuleLogs struct {
	FolderAvailable bool             `json:"folderAvailable"`
	Entries         []ModuleLogEntry `json:"entries"`
}
type moduleLogFile struct{ name, file string }

// Only known log files are exposed; this API never accepts a filesystem path.
func moduleLogFiles(id string) ([]moduleLogFile, error) {
	switch id {
	case "host", "wintray", "capture", "always-on-top", "selection":
		return []moduleLogFile{{"process", "host.log"}}, nil
	default:
		return nil, errors.New("unknown diagnostics module")
	}
}

func moduleLogDirectory(id string) string {
	// Legacy module IDs are compatibility aliases for the unified product log.
	return "logs"
}

// Read reads existing data without contacting or starting a worker.
// Root prevents module/log symlinks from escaping the product data directory.
func Read(dataDirectory, id string) (ModuleLogs, error) {
	files, err := moduleLogFiles(id)
	if err != nil {
		return ModuleLogs{}, err
	}
	result := ModuleLogs{Entries: make([]ModuleLogEntry, len(files))}
	for i, file := range files {
		result.Entries[i].Name = file.name
	}
	if dataDirectory == "" {
		return result, errors.New("diagnostics directory unavailable")
	}
	root, err := os.OpenRoot(dataDirectory)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer root.Close()
	folder, err := root.OpenRoot(moduleLogDirectory(id))
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer folder.Close()
	result.FolderAvailable = true
	for i, spec := range files {
		entry, err := readModuleLog(folder, spec)
		if err != nil {
			return result, err
		}
		result.Entries[i] = entry
	}
	return result, nil
}

func readModuleLog(folder *os.Root, spec moduleLogFile) (ModuleLogEntry, error) {
	entry := ModuleLogEntry{Name: spec.name}
	file, err := folder.Open(spec.file)
	if os.IsNotExist(err) {
		return entry, nil
	}
	if err != nil {
		return entry, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return entry, err
	}
	if !info.Mode().IsRegular() {
		return entry, errors.New("log is not a regular file")
	}
	const limit = 64 << 10
	offset := max(int64(0), info.Size()-limit)
	data, err := io.ReadAll(io.NewSectionReader(file, offset, limit))
	if err != nil {
		return entry, err
	}
	entry.Available = true
	entry.Truncated = offset > 0
	if entry.Truncated {
		for len(data) > 0 && !utf8.RuneStart(data[0]) {
			data = data[1:]
		}
	}
	entry.Text = string(bytes.ToValidUTF8(data, []byte("?")))
	return entry, nil
}

// Folder validates an existing product directory for an explicit UI open action.
// The caller may open this directory, but cannot choose arbitrary filesystem paths.
func Folder(dataDirectory, id string) (string, error) {
	if _, err := moduleLogFiles(id); err != nil {
		return "", err
	}
	if dataDirectory == "" {
		return "", errors.New("diagnostics directory unavailable")
	}
	root, err := os.OpenRoot(dataDirectory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	folder, err := root.OpenRoot(moduleLogDirectory(id))
	if err != nil {
		return "", err
	}
	if err := folder.Close(); err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(dataDirectory, moduleLogDirectory(id)))
}

// retentionWindow is how long product logs are kept (two days). The Host
// enforces it once per startup; size caps inside the writers bound growth
// between restarts, so entries can survive slightly past the window only
// while the Host keeps running without a restart.
const retentionWindow = 48 * time.Hour

// Prune drops product log content older than the retention window. It runs at
// Host startup before any worker is started, so log writers are idle; a worker
// orphaned by a crashed Host could still append concurrently and at worst lose
// one line. Failures are joined instead of aborting the sweep: stale
// diagnostics are a startup warning, never a reason to block the product.
func Prune(dataDirectory string, now time.Time) error {
	cutoff := now.Add(-retentionWindow)
	if dataDirectory == "" {
		return nil
	}
	root, err := os.OpenRoot(dataDirectory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	var errs []error
	if err := pruneTimestampedLog(root, filepath.Join("logs", "host.log"), cutoff, hostEventTime); err != nil {
		errs = append(errs, fmt.Errorf("logs/host.log: %w", err))
	}
	errs = append(errs, pruneModuleLogs(root, cutoff)...)
	return errors.Join(errs...)
}

// hostEventTime reports the time of one host.log event line.
func hostEventTime(line []byte) (time.Time, bool) {
	var event struct {
		Time time.Time `json:"time"`
	}
	if json.Unmarshal(line, &event) != nil || event.Time.IsZero() {
		return time.Time{}, false
	}
	return event.Time, true
}

// wintrayEntryTime reports the timestamp that starts a wintray.log entry.
// A message may span further lines without a timestamp; those belong to the
// preceding entry and share its keep/drop verdict.
func wintrayEntryTime(line []byte) (time.Time, bool) {
	sep := bytes.IndexByte(line, ' ')
	if sep <= 0 {
		return time.Time{}, false
	}
	stamp, err := time.Parse(time.RFC3339, string(line[:sep]))
	if err != nil {
		return time.Time{}, false
	}
	return stamp, true
}

// pruneTimestampedLog rewrites a line-oriented log keeping only entries from
// cutoff onward. Lines that do not start a parseable entry inherit the verdict
// of the preceding entry, so multi-line messages are never split. Lines before
// the first parseable entry are dropped with the expired prefix they belong to.
func pruneTimestampedLog(root *os.Root, name string, cutoff time.Time, entryTime func([]byte) (time.Time, bool)) error {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		// Non-regular files are a Read-time error; retention never deletes them.
		return nil
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return err
	}
	kept := make([]byte, 0, len(data))
	keep := false
	for offset := 0; offset < len(data); {
		var line []byte
		terminated := false
		next := len(data)
		if end := bytes.IndexByte(data[offset:], '\n'); end >= 0 {
			line, next, terminated = data[offset:offset+end], offset+end+1, true
		} else {
			line = data[offset:]
		}
		if stamp, ok := entryTime(line); ok {
			keep = !stamp.Before(cutoff)
		}
		if keep {
			kept = append(kept, line...)
			if terminated {
				kept = append(kept, '\n')
			}
		}
		offset = next
	}
	if len(kept) == len(data) {
		// Nothing expired; skip the rewrite so a busy log is not touched.
		return nil
	}
	// Replace through a sibling temp file: a crash mid-prune must never
	// truncate the log, and readers never observe a partial rewrite.
	temp := name + ".prune"
	if err := root.Remove(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := root.WriteFile(temp, kept, info.Mode().Perm()); err != nil {
		_ = root.Remove(temp)
		return err
	}
	if err := root.Rename(temp, name); err != nil {
		_ = root.Remove(temp)
		return err
	}
	return nil
}

// pruneModuleLogs applies retention inside every module data directory:
// timestamped activity logs are trimmed by entry age, rotated (.old) logs and
// per-day native engine logs are deleted once stale, and worker output is
// dropped as a whole when the file was not written within the window.
func pruneModuleLogs(root *os.Root, cutoff time.Time) []error {
	info, err := root.Lstat("modules")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return []error{err}
	}
	if !info.IsDir() {
		return nil // not a module layout; not retention's business
	}
	dir, err := root.Open("modules")
	if err != nil {
		return []error{err}
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return []error{err}
	}
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		module, err := root.OpenRoot(filepath.Join("modules", entry.Name()))
		if err != nil {
			// A module directory disappearing mid-sweep is not an error.
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("modules/%s: %w", entry.Name(), err))
			continue
		}
		errs = append(errs, pruneModuleDirectory(module, cutoff, entry.Name())...)
		module.Close()
	}
	return errs
}

func pruneModuleDirectory(module *os.Root, cutoff time.Time, id string) []error {
	dir, err := module.Open(".")
	if err != nil {
		return []error{fmt.Errorf("modules/%s: %w", id, err)}
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return []error{fmt.Errorf("modules/%s: %w", id, err)}
	}
	var errs []error
	for _, entry := range entries {
		name := entry.Name()
		var err error
		switch {
		case name == "wintray.log":
			// The only timestamped module activity log today.
			err = pruneTimestampedLog(module, name, cutoff, wintrayEntryTime)
		case name == "worker.log":
			err = truncateIfStale(module, name, cutoff)
		case name == "wintray.log.old":
			// Only the known rotated activity log belongs to retention. A
			// module can also keep configuration backups ending in .old.
			err = removeIfStale(module, name, cutoff)
		case entry.IsDir() && name == "native":
			errs = append(errs, pruneNativeLogs(module, cutoff, id)...)
			continue
		default:
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("modules/%s/%s: %w", id, name, err))
		}
	}
	return errs
}

// pruneNativeLogs deletes per-day engine logs under modules/<id>/native.
// Those files are written by prebuilt native DLLs with dated names, so
// whole-file age (mtime) is the only reliable signal available to the Host.
func pruneNativeLogs(module *os.Root, cutoff time.Time, id string) []error {
	dir, err := module.Open("native")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []error{fmt.Errorf("modules/%s/native: %w", id, err)}
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return []error{fmt.Errorf("modules/%s/native: %w", id, err)}
	}
	var errs []error
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		if err := removeIfStale(module, filepath.Join("native", entry.Name()), cutoff); err != nil {
			errs = append(errs, fmt.Errorf("modules/%s/native/%s: %w", id, entry.Name(), err))
		}
	}
	return errs
}

// removeIfStale deletes a file whose last write predates cutoff. Non-regular
// files are left untouched: retention never deletes links or odd objects.
func removeIfStale(root *os.Root, name string, cutoff time.Time) error {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
		return nil
	}
	return root.Remove(name)
}

// truncateIfStale empties a file whose last write predates cutoff. Worker
// output is raw stdout/stderr without timestamps, so retention is
// all-or-nothing per file; recent output stays completely intact.
func truncateIfStale(root *os.Root, name string, cutoff time.Time) error {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
		return nil
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	return file.Close()
}

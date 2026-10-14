//go:build windows

package modulepackage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/maintenance"
	"winone/internal/settings"
)

type DependencyLocation struct {
	Dependency
	Directory string          `json:"directory"`
	Exports   []RuntimeExport `json:"exports,omitempty"`
}
type DependencyContext struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Root          Dependency           `json:"root"`
	LockSHA256    string               `json:"lockSha256"`
	HostPipe      string               `json:"hostPipe"`
	Packages      []DependencyLocation `json:"packages"`
}

// DependencyLease owns file/ancestor handles for one worker generation. The
// persistent declarations decide installation references; these handles decide
// whether already-resolved code is still in use after a crash or cancellation.
type DependencyLease struct {
	source      *DependencySource
	view        DependencyContext
	files       []*runtimeFiles
	contextPath string
	closed      bool
}

func (s *DependencySource) Hold(ctx context.Context, root *VerifiedManifest) (result *DependencyLease, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	admission, err := maintenance.AcquireRuntime(s.modules.directory)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, admission.Close())
		if err != nil && result != nil {
			err = errors.Join(err, result.Close())
		}
	}()
	lock, err := s.Resolve(ctx, root)
	if err != nil {
		return nil, err
	}
	digest, err := lock.Digest()
	if err != nil {
		return nil, err
	}
	result = &DependencyLease{source: s, view: DependencyContext{SchemaVersion: 1, Root: lock.Root, LockSHA256: digest, Packages: []DependencyLocation{}}}
	for _, reference := range lock.Packages {
		stage, loadErr := s.stage(ctx, reference)
		if loadErr != nil {
			return result, errors.Join(loadErr, result.Close())
		}
		files, holdErr := holdRuntimeFiles(ctx, stage.Directory(), stage.Manifest())
		if files != nil {
			result.files = append(result.files, files)
		}
		if holdErr != nil {
			return result, errors.Join(holdErr, result.Close())
		}
		// Final validation happens with both names and bytes held against change.
		checked, checkErr := s.stage(ctx, reference)
		if checkErr != nil {
			return result, errors.Join(checkErr, result.Close())
		}
		if checked.Directory() != stage.Directory() {
			return result, errors.Join(errors.New("dependency directory changed while acquiring files"), result.Close())
		}
		location := DependencyLocation{Dependency: reference, Directory: stage.Directory()}
		if runtime := checked.Manifest().Metadata().Runtime; runtime != nil {
			for _, export := range runtime.Exports {
				export.Path = filepath.Join(stage.Directory(), filepath.FromSlash(export.Path))
				location.Exports = append(location.Exports, export)
			}
		}
		result.view.Packages = append(result.view.Packages, location)
	}
	return result, nil
}

// WriteContext uses one Host-owned per-module file instead of a potentially
// unbounded process environment variable. The context remains immutable until
// this worker generation closes; no global PATH or system runtime is changed.
func (l *DependencyLease) WriteContext(directory, hostPipe string) (string, error) {
	if l.closed || l.contextPath != "" || !filepath.IsAbs(directory) || !strings.HasPrefix(hostPipe, `\\.\pipe\WinOne-`) || strings.ContainsRune(hostPipe, '\x00') || len(hostPipe) > 256 {
		return "", errors.New("dependency context destination or lifetime is invalid")
	}
	// The complete package store is immutable code space, including this root
	// module and versions that are not part of the current dependency closure.
	if relative, err := filepath.Rel(l.source.modules.directory, directory); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("dependency context must remain outside the package store")
	}
	l.view.HostPipe = hostPipe
	raw, err := json.Marshal(l.view)
	if err != nil {
		return "", err
	}
	if len(raw) > 1<<20 {
		return "", errors.New("dependency context exceeds size limit")
	}
	// Pin the nearest existing ancestor before creating directories, so even
	// a missing per-worker directory cannot lead through a junction elsewhere.
	ancestor := filepath.Clean(directory)
	for {
		_, statErr := os.Lstat(ancestor)
		if statErr == nil {
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) || filepath.Dir(ancestor) == ancestor {
			return "", statErr
		}
		ancestor = filepath.Dir(ancestor)
	}
	parents, err := holdRuntimeParents(ancestor)
	if parents != nil {
		l.files = append(l.files, parents)
	}
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	if ancestor != filepath.Clean(directory) {
		parents, err = holdRuntimeParents(directory)
		if parents != nil {
			l.files = append(l.files, parents)
		}
		if err != nil {
			return "", err
		}
	}
	path := filepath.Join(directory, "dependencies.json")
	if err = settings.WriteAtomic(path, raw); err != nil {
		return "", err
	}
	handle, err := holdRuntimePath(path, false)
	if handle != 0 {
		l.files = append(l.files, &runtimeFiles{handles: []windows.Handle{handle}})
	}
	if err != nil {
		return "", err
	}
	checked, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(checked, raw) {
		return "", errors.New("dependency context changed before its file lease was acquired")
	}
	l.contextPath = path
	return path, nil
}

// Recheck is cheap while file leases are held: inspect only live bindings and
// authenticated metadata, including current publisher trust, not all payload.
func (l *DependencyLease) Recheck(ctx context.Context) error {
	if l.closed {
		return errors.New("dependency lease is closed")
	}
	for _, location := range l.view.Packages {
		if err := ctx.Err(); err != nil {
			return err
		}
		var record RegisteredPackage
		var directory string
		if location.Kind == "runtime" {
			snapshot, err := l.source.runtimes.read(ctx)
			if err != nil {
				return err
			}
			record = snapshot.Records[location.ManifestSHA256]
			directory = l.source.runtimes.directory
		} else {
			current, err := l.source.modules.runtimeRecord(ctx, location.ID, nil)
			if err != nil {
				return err
			}
			record = current
			directory = l.source.modules.directory
		}
		if record.ModuleID != location.ID || record.Version != location.Version || record.ManifestSHA256 != location.ManifestSHA256 || filepath.Join(directory, record.StageID) != location.Directory {
			return errors.New("active dependency binding changed")
		}
		namespace, _, _ := strings.Cut(location.ID, ".")
		publisher, err := l.source.trust(namespace)
		if err != nil {
			return err
		}
		if _, err = inspectStoredReferenceManifest(ctx, directory, record, publisher); err != nil {
			return err
		}
	}
	return nil
}

func (l *DependencyLease) Close() error {
	if l == nil || l.closed {
		return nil
	}
	var failures []error
	for index := len(l.files) - 1; index >= 0; index-- {
		if err := l.files[index].Close(); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	l.files = nil
	l.closed = true
	return nil
}

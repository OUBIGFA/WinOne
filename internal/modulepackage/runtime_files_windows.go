//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"sort"
	"strings"
)

// runtimeFiles pins signed payload/control files against writes and deletion.
// Directory handles deny rename along the complete path. This is file identity
// protection, not a security sandbox for arbitrary code in a trusted package.
type runtimeFiles struct{ handles []windows.Handle }

func holdRuntimeFiles(ctx context.Context, directory string, manifest *VerifiedManifest) (*runtimeFiles, error) {
	if !filepath.IsAbs(directory) || manifest == nil {
		return nil, errors.New("runtime file identity is incomplete")
	}
	lease := &runtimeFiles{}
	fail := func(err error) (*runtimeFiles, error) {
		if closeErr := lease.Close(); closeErr != nil {
			return lease, errors.Join(err, closeErr)
		}
		return nil, err
	}
	directories := map[string]string{}
	addParents := func(path string) {
		for {
			parent := filepath.Dir(path)
			if parent == path {
				break
			}
			directories[strings.ToLower(path)] = path
			path = parent
		}
	}
	addParents(filepath.Clean(directory))
	names := []string{"module.json", "module.json.minisig", StageReceiptName}
	for _, file := range manifest.Files() {
		names = append(names, filepath.FromSlash(file.Path))
		addParents(filepath.Dir(filepath.Join(directory, filepath.FromSlash(file.Path))))
	}
	paths := make([]string, 0, len(directories))
	for _, path := range directories {
		paths = append(paths, path)
	}
	// Parents must be held before children so an already checked path cannot
	// be redirected while later files are opened through their absolute names.
	sort.Slice(paths, func(i, j int) bool {
		if len(paths[i]) == len(paths[j]) {
			return paths[i] < paths[j]
		}
		return len(paths[i]) < len(paths[j])
	})
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		handle, err := holdRuntimePath(path, true)
		if handle != 0 {
			lease.handles = append(lease.handles, handle)
		}
		if err != nil {
			return fail(err)
		}
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		handle, err := holdRuntimePath(filepath.Join(directory, name), false)
		if handle != 0 {
			lease.handles = append(lease.handles, handle)
		}
		if err != nil {
			return fail(err)
		}
	}
	return lease, nil
}

func holdRuntimePath(path string, directory bool) (windows.Handle, error) {
	return holdRuntimePathAccess(path, directory, 0)
}

func holdRuntimePathAccess(path string, directory bool, extraAccess uint32) (windows.Handle, error) {
	name := filepath.Clean(path)
	// Raw Win32 calls do not get Go's automatic extended-length conversion.
	if !strings.HasPrefix(name, `\\?\`) {
		if strings.HasPrefix(name, `\\`) {
			name = `\\?\UNC\` + name[2:]
		} else {
			name = `\\?\` + name
		}
	}
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	access, share, flags := uint32(windows.GENERIC_READ), uint32(windows.FILE_SHARE_READ), uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		access = windows.FILE_READ_ATTRIBUTES
		share |= windows.FILE_SHARE_WRITE
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	access |= extraAccess
	handle, err := windows.CreateFile(wide, access, share, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return 0, fmt.Errorf("cannot protect module path %s: %w", path, err)
	}
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &info); err == nil {
		if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
			err = errors.New("module execution path is linked or has the wrong type")
		}
	}
	if err != nil {
		if closeErr := windows.CloseHandle(handle); closeErr != nil {
			return handle, errors.Join(err, closeErr)
		}
		return 0, err
	}
	return handle, nil
}

// Keep any failed close for a later recovery attempt instead of losing the
// handle and reporting a package safe to remove while it is still protected.
func (r *runtimeFiles) Close() error {
	if r == nil {
		return nil
	}
	var result error
	for i := len(r.handles) - 1; i >= 0; i-- {
		if r.handles[i] == 0 {
			continue
		}
		if err := windows.CloseHandle(r.handles[i]); err != nil {
			result = errors.Join(result, err)
		} else {
			r.handles[i] = 0
		}
	}
	return result
}

//go:build windows

package modulepackage

import (
	"context"
	"encoding/binary"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

func verifyReclaimDirectory(ctx context.Context, directory string, record RegisteredPackage, publisher Publisher) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("reclaim directory is linked or not a directory")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	manifest, err := verifyStageContents(ctx, root, record.StageID, StageOptions{Publisher: publisher, ExpectedManifestSHA256: record.ManifestSHA256}, false)
	if err != nil {
		return err
	}
	metadata := manifest.Metadata()
	if metadata.PackageID() != record.ModuleID || metadata.Version != record.Version {
		return errors.New("reclaim directory does not match the recorded package")
	}
	return nil
}

// Pin ancestors before operations that use an absolute path more than once.
// Context publication and reclamation share the same no-junction boundary.
func holdRuntimeParents(directory string) (*runtimeFiles, error) {
	var paths []string
	for current := directory; filepath.Dir(current) != current; current = filepath.Dir(current) {
		paths = append(paths, current)
	}
	lease := &runtimeFiles{}
	for i := len(paths) - 1; i >= 0; i-- {
		handle, err := holdRuntimePath(paths[i], true)
		if handle != 0 {
			lease.handles = append(lease.handles, handle)
		}
		if err != nil {
			return lease, errors.Join(err, lease.Close())
		}
	}
	return lease, nil
}

// Rename the verified directory through its open handle. Runtime directory
// leases deny DELETE access, so a live worker blocks this move. The destination
// is a unique quarantine name and must never replace an existing object.
func moveReclaimDirectory(source, target string, check func() error) (err error) {
	// os.Root intentionally denies directory deletion while it is open. Verify
	// under a read guard, then compare the directory's file identity after
	// acquiring DELETE access; a path swap during that handoff must fail.
	read, err := holdRuntimePath(source, true)
	if err != nil {
		return err
	}
	var expected windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(read, &expected)
	if err == nil {
		err = check()
	}
	err = errors.Join(err, windows.CloseHandle(read))
	if err != nil {
		return err
	}
	handle, err := holdRuntimePathAccess(source, true, windows.DELETE)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(handle)) }()
	var actual windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &actual); err != nil {
		return err
	}
	if actual.VolumeSerialNumber != expected.VolumeSerialNumber || actual.FileIndexHigh != expected.FileIndexHigh || actual.FileIndexLow != expected.FileIndexLow {
		return errors.New("reclaim directory identity changed")
	}
	const fileAddSubdirectory = 0x0004 // FILE_ADD_SUBDIRECTORY (directory-specific access)
	parent, err := holdRuntimePathAccess(filepath.Dir(target), true, fileAddSubdirectory)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(parent)) }()
	name, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	var layout struct {
		Replace uint32
		Root    windows.Handle
		Length  uint32
		Name    [1]uint16
	}
	offset := int(unsafe.Offsetof(layout.Name))
	buffer := make([]byte, offset+len(name)*2)
	header := (*struct {
		Replace uint32
		Root    windows.Handle
		Length  uint32
	})(unsafe.Pointer(&buffer[0]))
	// Kernel32's FileRenameInfo path uses a fully qualified target. Keep the
	// parent handle as a rename guard, not as an NT relative-name root.
	header.Root = 0
	header.Length = uint32((len(name) - 1) * 2)
	for i, value := range name {
		binary.LittleEndian.PutUint16(buffer[offset+i*2:], value)
	}
	return windows.SetFileInformationByHandle(handle, windows.FileRenameInfo, &buffer[0], uint32(len(buffer)))
}

func ordinaryDirectoryExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("reclaim path is linked or not a directory")
	}
	return true, nil
}

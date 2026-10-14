//go:build windows

// Package onboarding owns machine-wide, non-secret first-use records.
package onboarding

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Tests may set this only at link time. Runtime environment variables and RPC
// inputs cannot redirect the product's machine-wide marker location.
var machineDirectoryOverride string

func MachineDirectory() (string, error) {
	if machineDirectoryOverride != "" {
		return filepath.Clean(machineDirectoryOverride), nil
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "WinOne", "onboarding"), nil
}

// PrepareDirectory grants inherited Modify to Builtin Users only when creating
// the final directory. Existing directories and all ancestor ACLs are untouched.
func PrepareDirectory(directory string) error {
	release, err := lockDirectory(directory, true)
	if err == nil {
		release()
	}
	return err
}

// LockDirectory verifies and pins every directory component for a marker
// operation. No write/delete sharing prevents retargeting or renaming a checked
// parent while the caller opens the marker. Call release after closing it.
func LockDirectory(directory string) (release func(), err error) {
	return lockDirectory(directory, false)
}

func lockDirectory(directory string, create bool) (func(), error) {
	directory = filepath.Clean(directory)
	volume := filepath.VolumeName(directory)
	if !filepath.IsAbs(directory) || len(volume) != 2 || volume[1] != ':' {
		return nil, errors.New("首次提示目录必须是本机绝对路径")
	}
	parts := strings.Split(strings.TrimPrefix(directory, volume+string(filepath.Separator)), string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "" {
		return nil, errors.New("首次提示目录不能是磁盘根目录")
	}
	for _, part := range parts {
		if part == "" || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, ":\x00") {
			return nil, errors.New("首次提示目录路径无效")
		}
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1301bf;;;BU)")
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	var handles []windows.Handle
	var once sync.Once
	release := func() {
		once.Do(func() {
			for i := len(handles) - 1; i >= 0; i-- {
				_ = windows.CloseHandle(handles[i])
			}
		})
	}
	current := volume + string(filepath.Separator)
	paths := []string{current}
	for _, part := range parts {
		current = filepath.Join(current, part)
		paths = append(paths, current)
	}
	for index, path := range paths {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			release()
			return nil, err
		}
		open := func() (windows.Handle, error) {
			return windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		}
		handle, err := open()
		if create && index > 0 && (errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)) {
			var security *windows.SecurityAttributes
			if index == len(paths)-1 {
				security = &sa
			}
			err = windows.CreateDirectory(name, security)
			if err == nil || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
				handle, err = open()
			}
		}
		if err != nil {
			release()
			return nil, fmt.Errorf("无法检查首次提示目录 %s：%w", path, err)
		}
		handles = append(handles, handle)
		var info windows.ByHandleFileInformation
		if err = windows.GetFileInformationByHandle(handle, &info); err != nil {
			release()
			return nil, err
		}
		if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			release()
			return nil, errors.New("首次提示目录不能经过链接或其他重解析点")
		}
	}
	return release, nil
}

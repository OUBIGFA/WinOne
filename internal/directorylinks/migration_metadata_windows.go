//go:build windows

package directorylinks

import (
	"errors"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Apply directory timestamps and mutable attributes through a non-following
// handle. No path-based metadata write may follow a replaced junction.
func setMigrationDirectoryTimes(path string, entry migrationEntry, root string, rootID Identity) error {
	pins, err := pinMigrationAncestors(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer closeMigrationPins(pins)
	name, err := windows.UTF16PtrFromString(nativePath(path))
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES|windows.FILE_WRITE_ATTRIBUTES, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	f, err := readFileState(h, path, false)
	if err != nil {
		return err
	}
	defer f.close()
	if !f.directory || f.reparse {
		return errors.New("副本目录在设置时间戳前发生变化")
	}
	r, err := openState(root, false, false)
	if err != nil {
		return err
	}
	same := !r.reparse && sameIdentity(r.identity, rootID)
	r.close()
	if !same {
		return errors.New("副本根目录已被替换")
	}
	rawTime := func(value windows.Filetime) int64 {
		return int64(uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime))
	}
	var basic struct {
		Created, Accessed, Written, Changed int64
		Attributes                          uint32
		Padding                             uint32
	}
	basic.Created = rawTime(entry.Created)
	basic.Accessed = rawTime(entry.Accessed)
	basic.Written = rawTime(entry.Written)
	basic.Attributes = windows.FILE_ATTRIBUTE_DIRECTORY | entry.Attributes&(windows.FILE_ATTRIBUTE_READONLY|windows.FILE_ATTRIBUTE_HIDDEN|windows.FILE_ATTRIBUTE_SYSTEM|windows.FILE_ATTRIBUTE_ARCHIVE|windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED)
	return windows.SetFileInformationByHandle(f.handle, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&basic)), uint32(unsafe.Sizeof(basic)))
}

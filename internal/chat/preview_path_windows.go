//go:build windows

package chat

import (
	"errors"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
)

// Ask Windows to resolve the opened object. This also handles directory
// junctions whose reparse attributes are not exposed as ModeSymlink by ReadDir.
func resolvePreviewPath(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	size, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
	if err != nil {
		return "", err
	}
	if size == 0 || size >= uint32(len(buffer)) {
		return "", errors.New("文件路径无效或过长")
	}
	resolved := windows.UTF16ToString(buffer[:size])
	if strings.HasPrefix(strings.ToUpper(resolved), `\\?\UNC\`) {
		resolved = `\\` + resolved[8:]
	} else {
		resolved = strings.TrimPrefix(resolved, `\\?\`)
	}
	return filepath.Clean(resolved), nil
}

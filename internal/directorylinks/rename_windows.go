//go:build windows

package directorylinks

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RenameRequest carries the entry and parent identities observed by Browse.
// A stale row must not rename a replacement object or a redirected directory.
type RenameRequest struct {
	Path           string   `json:"path"`
	Name           string   `json:"name"`
	EntryIdentity  Identity `json:"entryIdentity"`
	ParentIdentity Identity `json:"parentIdentity"`
}

type RenameResult struct {
	Path    string   `json:"path"`
	Records []Record `json:"records"`
	Warning string   `json:"warning,omitempty"`
}

// Rename changes one directory entry in its current parent, never its target
// or contents. Both opening and renaming are relative to the verified parent
// handle: an ancestor junction changed mid-operation cannot redirect the write.
func (m *Manager) Rename(ctx context.Context, request RenameRequest) (RenameResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := RenameResult{Records: []Record{}}
	if m.migrating {
		return result, errMigrationBusy
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	path, err := localPath(request.Path)
	if err != nil {
		return result, err
	}
	if filepath.Dir(path) == path || !validLinkName(request.Name) || len(utf16.Encode([]rune(request.Name))) > 255 || strings.ContainsFunc(request.Name, func(r rune) bool { return r < 32 }) {
		return result, errors.New("名称无效；请仅输入文件名或文件夹名，不使用保留名称、路径分隔符或末尾空格/句点")
	}
	if request.EntryIdentity.File == "" || request.ParentIdentity.File == "" {
		return result, errors.New("缺少文件身份，请刷新目标文件夹后重试")
	}
	parent, err := openState(filepath.Dir(path), true, false)
	if err != nil {
		return result, fmt.Errorf("无法打开所在文件夹，请刷新后重试：%w", err)
	}
	defer parent.close()
	if !parent.directory || !sameIdentity(parent.identity, request.ParentIdentity) {
		return result, errors.New("所在文件夹已变化，请刷新目标文件夹后重试")
	}
	entry, err := openRenameEntry(parent.handle, path)
	if err != nil {
		return result, fmt.Errorf("无法重命名；请确认项目仍存在、未被占用且有修改权限：%w", err)
	}
	defer entry.close()
	if !sameIdentity(entry.identity, request.EntryIdentity) {
		return result, errors.New("项目已被替换，请刷新目标文件夹后重试")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	// NT treats a different hard-link name of the same file as a successful
	// no-op. Reject existing sibling names explicitly as well as asking the
	// kernel never to replace; only a case-only spelling of this entry is exempt.
	targetPath := filepath.Join(filepath.Dir(path), request.Name)
	existing, targetErr := openRelativeEntry(parent.handle, targetPath, false)
	if targetErr == nil {
		sameName := strings.EqualFold(filepath.Base(path), request.Name) && sameIdentity(existing.identity, entry.identity)
		existing.close()
		if !sameName {
			return result, errors.New("所在文件夹已有同名项目，不能覆盖；请换一个名称")
		}
	} else if !errors.Is(targetErr, windows.STATUS_OBJECT_NAME_NOT_FOUND) && !errors.Is(targetErr, windows.STATUS_NO_SUCH_FILE) {
		return result, fmt.Errorf("无法核实新名称是否可用，请刷新后重试：%w", targetErr)
	}
	if err := renameEntry(entry.handle, parent.handle, request.Name); err != nil {
		if errors.Is(err, windows.STATUS_OBJECT_NAME_COLLISION) {
			return result, errors.New("所在文件夹已有同名项目，不能覆盖；请换一个名称")
		}
		return result, fmt.Errorf("重命名失败；请检查名称、占用及修改权限后重试：%w", err)
	}
	result.Path = filepath.Join(filepath.Dir(path), request.Name)
	actualPath := filepath.Join(parent.finalPath, filepath.Base(path))
	actualNewPath := filepath.Join(parent.finalPath, request.Name)
	for index, record := range m.records {
		if record.Status == "removed" {
			continue
		}
		next, moved := renamedEntryPath(record.LinkPath, path, result.Path)
		if !moved {
			next, moved = renamedEntryPath(record.LinkPath, actualPath, actualNewPath)
		}
		if !moved {
			continue
		}
		// Preserve replaced/unverified historical entries instead of silently
		// assigning them to the object that now happens to use this name.
		observed, readErr := openState(next, false, false)
		if readErr != nil {
			continue
		}
		matches := sameIdentity(observed.identity, record.EntryIdentity)
		observed.close()
		if !matches {
			continue
		}
		if record.Name == filepath.Base(record.LinkPath) {
			record.Name = filepath.Base(next)
		}
		record.LinkPath = next
		// Stored targets are not rewritten: renaming a real target may break a
		// symlink, and the inspection must report that actual outcome.
		record = checked(record)
		m.records[index] = record
		result.Records = append(result.Records, record)
	}
	if len(result.Records) > 0 {
		if err := m.save(); err != nil {
			// The filesystem operation succeeded. Report the separate persistence
			// failure without offering a duplicate rename of the old path.
			result.Warning = fmt.Sprintf("已重命名，但链接历史未能保存：%v。请修复历史目录的写入权限，并重新检查链接。", err)
		}
	}
	return result, nil
}

func openRenameEntry(parent windows.Handle, path string) (*fileState, error) {
	return openRelativeEntry(parent, path, true)
}

func openRelativeEntry(parent windows.Handle, path string, rename bool) (*fileState, error) {
	name, err := windows.NewNTUnicodeString(filepath.Base(path))
	if err != nil {
		return nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	access, share := uint32(windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE), uint32(windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE)
	if rename {
		access |= windows.DELETE
		share &^= windows.FILE_SHARE_DELETE
	}
	err = windows.NtCreateFile(&handle, access, &attributes, &status, nil, 0,
		share, windows.FILE_OPEN,
		windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return nil, err
	}
	return readFileState(handle, path, false)
}

// FILE_RENAME_INFORMATION uses a UTF-16 name relative to RootDirectory. The
// zero Replace field is essential: collision handling belongs to the kernel,
// not a racy existence check. Same-directory case-only renames are supported.
// https://learn.microsoft.com/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information
func renameEntry(entry, parent windows.Handle, name string) error {
	units := utf16.Encode([]rune(name))
	var layout struct {
		Replace uint32
		Root    windows.Handle
		Length  uint32
		Name    [1]uint16
	}
	offset := int(unsafe.Offsetof(layout.Name))
	buffer := make([]byte, offset+len(units)*2)
	header := (*struct {
		Replace uint32
		Root    windows.Handle
		Length  uint32
	})(unsafe.Pointer(&buffer[0]))
	header.Root, header.Length = parent, uint32(len(units)*2)
	for index, unit := range units {
		binary.LittleEndian.PutUint16(buffer[offset+index*2:], unit)
	}
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(entry, &status, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
}

func renamedEntryPath(path, before, after string) (string, bool) {
	if strings.EqualFold(path, before) {
		return after, true
	}
	if len(path) > len(before) && strings.EqualFold(path[:len(before)], before) && path[len(before)] == filepath.Separator {
		return after + path[len(before):], true
	}
	return path, false
}

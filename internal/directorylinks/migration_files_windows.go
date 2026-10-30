//go:build windows

package directorylinks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Migration copies only ordinary local NTFS trees. Reparse points are refused,
// not followed or silently skipped; CopyFileEx preserves NTFS streams, attributes
// and timestamps. Every file is content-verified before the namespace switch.
type migrationEntry struct {
	Relative   string
	Identity   Identity
	Directory  bool
	Size       int64
	Bytes      int64
	Links      uint32
	Modified   int64
	Attributes uint32
	Created    windows.Filetime
	Accessed   windows.Filetime
	Written    windows.Filetime
	Hash       string
}
type migrationProgress func(MigrationProgress)

func migrationLocalPath(path string) (string, error) {
	p, err := localPath(path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(p, `\\`) || len(filepath.VolumeName(p)) != 2 {
		return "", errors.New("搬迁只支持本机磁盘目录，不支持网络或设备路径")
	}
	for _, part := range strings.Split(strings.TrimPrefix(p, filepath.VolumeName(p)), string(filepath.Separator)) {
		if part != "" && !validLinkName(part) {
			return "", errors.New("目录路径包含保留名称或不明确的末尾字符")
		}
	}
	return p, nil
}
func migrationWithin(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
func migrationDanger(path, own string) ([]string, error) {
	if filepath.Dir(path) == path {
		return nil, errors.New("不能搬迁磁盘根目录")
	}
	windowsDir, err := windows.GetWindowsDirectory()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// Block both moving protected directories and moving an ancestor containing
	// them. Deep application-data folders remain explicit warning-level choices.
	protected := []string{windowsDir, filepath.Dir(exe), own}
	for _, name := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramData", "USERPROFILE", "APPDATA", "LOCALAPPDATA"} {
		if value := os.Getenv(name); filepath.IsAbs(value) {
			if strings.EqualFold(path, filepath.Clean(value)) || migrationWithin(value, path) {
				return nil, errors.New("不能整体搬迁系统、程序或用户配置根目录")
			}
		}
	}
	for _, p := range protected {
		if filepath.IsAbs(p) && (migrationWithin(path, p) || migrationWithin(p, path)) {
			return nil, errors.New("此目录属于 Windows 或正在运行的 WinOne，不能搬迁")
		}
	}
	base := strings.ToLower(filepath.Base(path))
	if base == "system volume information" || base == "$recycle.bin" || base == "windowsapps" {
		return nil, errors.New("此目录由 Windows 管理，不能搬迁")
	}
	warnings := []string{}
	for _, name := range []string{"APPDATA", "LOCALAPPDATA", "ProgramFiles", "ProgramFiles(x86)", "ProgramData"} {
		if p := os.Getenv(name); filepath.IsAbs(p) && migrationWithin(path, p) {
			warnings = append(warnings, "应用或配置目录：请先退出相关程序。占用检查无法保证服务、驱动或内存映射不会继续写入。")
			break
		}
	}
	return warnings, nil
}

// Hold each ordinary ancestor without write/delete sharing while operating on
// a canonical path. A renamed ancestor or a newly inserted junction cannot
// redirect a later path-based copy/create to another tree.
func pinMigrationAncestors(path string) ([]*fileState, error) {
	path, err := migrationLocalPath(path)
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for current := path; ; current = filepath.Dir(current) {
		paths = append(paths, current)
		if filepath.Dir(current) == current {
			break
		}
	}
	pins := []*fileState{}
	for i := len(paths) - 1; i >= 0; i-- {
		name, _ := windows.UTF16PtrFromString(nativePath(paths[i]))
		h, e := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if e != nil {
			closeMigrationPins(pins)
			return nil, fmt.Errorf("目录被占用或权限不足 %s：%w", paths[i], e)
		}
		f, e := readFileState(h, paths[i], false)
		if e != nil {
			closeMigrationPins(pins)
			return nil, e
		}
		if !f.directory || f.reparse {
			f.close()
			closeMigrationPins(pins)
			return nil, fmt.Errorf("目录路径经过链接或非普通目录：%s", paths[i])
		}
		pins = append(pins, f)
	}
	return pins, nil
}
func closeMigrationPins(pins []*fileState) {
	for i := len(pins) - 1; i >= 0; i-- {
		pins[i].close()
	}
}

func scanMigrationTree(ctx context.Context, root string, hash bool, progress migrationProgress) ([]migrationEntry, int64, error) {
	entries := []migrationEntry{}
	var bytes int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if len(entries) >= 100000 {
			return errors.New("目录超过本次安全检查的 100000 个项目上限，请选择较小的目录")
		}
		f, err := openState(path, false, false)
		if err != nil {
			return err
		}
		defer f.close()
		if f.reparse {
			return fmt.Errorf("目录中包含重解析链接，不能作为普通数据复制：%s", path)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !f.directory && !info.Mode().IsRegular() {
			return fmt.Errorf("目录中包含不支持的对象：%s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		var native windows.ByHandleFileInformation
		if err = windows.GetFileInformationByHandle(f.handle, &native); err != nil {
			return err
		}
		entry := migrationEntry{Relative: relative, Identity: f.identity, Directory: f.directory, Links: f.links, Size: info.Size(), Modified: info.ModTime().UnixNano(), Attributes: native.FileAttributes, Created: native.CreationTime, Accessed: native.LastAccessTime, Written: native.LastWriteTime}
		if !entry.Directory {
			streams, total, streamErr := migrationStreams(path)
			if streamErr != nil {
				return streamErr
			}
			entry.Bytes = total
			if entry.Size < 0 || bytes > math.MaxInt64-entry.Bytes {
				return errors.New("目录总大小无效")
			}
			bytes += entry.Bytes
			// Exclusive read detects writable handles before any source is moved.
			file, err := openMigrationFile(path)
			if err != nil {
				return fmt.Errorf("文件被占用或不可读 %s：%w", path, err)
			}
			if hash {
				entry.Hash, err = hashMigrationStreams(ctx, path, file, streams)
			}
			closeErr := file.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
		entries = append(entries, entry)
		if progress != nil && len(entries)%128 == 0 {
			progress(MigrationProgress{Phase: "checking", Files: len(entries), Bytes: bytes})
		}
		return nil
	})
	return entries, bytes, err
}
func openMigrationFile(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(nativePath(path))
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		windows.CloseHandle(h)
		return nil, errors.Join(errors.New("文件类型在检查期间发生变化"), err)
	}
	return os.NewFile(uintptr(h), path), nil
}

type migrationReader struct {
	ctx context.Context
	r   io.Reader
}

func (r migrationReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func hashMigrationFile(ctx context.Context, file *os.File) (string, error) {
	h := sha256.New()
	if _, err := io.CopyBuffer(h, migrationReader{ctx, file}, make([]byte, 256<<10)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// WalkDir supplies lexical order. Store only this fingerprint with a preview,
// not a potentially huge tree: an idle settings page must release scan memory.
func migrationSnapshot(entries []migrationEntry) string {
	h := sha256.New()
	for _, e := range entries {
		size, modified := e.Size, e.Modified
		if e.Directory {
			size = 0
			modified = 0
		}
		fmt.Fprintf(h, "%s\x00%s\x00%d\x00%s\x00%t\x00%d\x00%d\x00%d\x00%d\x00%d\n", e.Relative, e.Identity.VolumeID, e.Identity.Volume, e.Identity.File, e.Directory, size, e.Bytes, modified, e.Attributes, e.Links)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func migrationFreeSpace(path string, required int64) error {
	name, err := windows.UTF16PtrFromString(nativePath(path))
	if err != nil {
		return err
	}
	var available uint64
	if err = windows.GetDiskFreeSpaceEx(name, &available, nil, nil); err != nil {
		return err
	}
	margin := required / 10
	if margin < 4<<20 {
		margin = 4 << 20
	}
	if required < 0 || uint64(required) > available || uint64(margin) > available-uint64(required) {
		return errors.New("目标磁盘可用空间不足（需要数据大小及至少 10% / 4 MiB 的余量）")
	}
	return nil
}

var migrationCopyFileEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("CopyFileExW")

type migrationCopyState struct {
	ctx    context.Context
	notify func(int64)
}

var migrationCopyStates sync.Map
var migrationCopySequence atomic.Uint64

// A single callback is registered for the process. windows.NewCallback cannot
// be freed, so registering it per file would exhaust the Go callback table.
var migrationCopyCallback = windows.NewCallback(func(total, totalDone, stream, streamDone, number, reason, sourceHandle, targetHandle, data uintptr) uintptr {
	value, ok := migrationCopyStates.Load(data)
	if !ok {
		return 1
	}
	state := value.(migrationCopyState)
	if state.ctx.Err() != nil {
		return 1
	}
	if state.notify != nil {
		state.notify(int64(totalDone))
	}
	return 0
})

func copyMigrationFile(ctx context.Context, source, target string, onBytes func(int64)) error {
	from, err := windows.UTF16PtrFromString(nativePath(source))
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(nativePath(target))
	if err != nil {
		return err
	}
	var cancelled uint32
	id := uintptr(migrationCopySequence.Add(1))
	migrationCopyStates.Store(id, migrationCopyState{ctx, onBytes})
	defer migrationCopyStates.Delete(id)
	ok, _, callErr := migrationCopyFileEx.Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), migrationCopyCallback, id, uintptr(unsafe.Pointer(&cancelled)), 1)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if ok == 0 {
		return callErr
	}
	return nil
}

func verifyMigrationTrees(ctx context.Context, source, target string, expected []migrationEntry, progress migrationProgress) (string, error) {
	current, _, err := scanMigrationTree(ctx, source, true, progress)
	if err != nil {
		return "", err
	}
	copied, _, err := scanMigrationTree(ctx, target, true, nil)
	if err != nil {
		return "", err
	}
	if len(current) != len(expected) || len(copied) != len(expected) {
		return "", errors.New("复制校验失败：项目数量发生变化")
	}
	by := map[string]migrationEntry{}
	for _, e := range copied {
		by[e.Relative] = e
	}
	old := map[string]migrationEntry{}
	for _, e := range expected {
		old[e.Relative] = e
	}
	digest := sha256.New()
	sort.Slice(current, func(i, j int) bool { return current[i].Relative < current[j].Relative })
	for _, e := range current {
		before, ok := old[e.Relative]
		copy, found := by[e.Relative]
		if !ok || !found || !sameIdentity(e.Identity, before.Identity) || e.Directory != before.Directory || e.Size != before.Size && !e.Directory || e.Bytes != before.Bytes || e.Links != before.Links || e.Modified != before.Modified && !e.Directory || e.Directory != copy.Directory || !e.Directory && (e.Size != copy.Size || e.Hash != copy.Hash) {
			return "", fmt.Errorf("复制校验失败，源或副本已变化：%s", e.Relative)
		}
		fmt.Fprintf(digest, "%s\x00%t\x00%d\x00%s\n", e.Relative, e.Directory, e.Size, e.Hash)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

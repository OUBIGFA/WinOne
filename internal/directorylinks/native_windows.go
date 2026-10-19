//go:build windows

package directorylinks

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const tagSymlink = uint32(0xa000000c)
const tagJunction = uint32(0xa0000003)

type fileState struct {
	handle    windows.Handle
	identity  Identity
	directory bool
	reparse   bool
	links     uint32
	finalPath string
}

func nativePath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + path[2:]
	}
	return `\\?\` + path
}

func displayPath(path string) string {
	if strings.HasPrefix(strings.ToUpper(path), `\\?\UNC\`) || strings.HasPrefix(strings.ToUpper(path), `\??\UNC\`) {
		return `\\` + path[8:]
	}
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\??\`) {
		return path[4:]
	}
	return path
}

// Open a pinned object with minimal rights; content/list access is a separate
// explicit probe, so attribute access cannot masquerade as usable data.
func openState(path string, follow, readable bool) (*fileState, error) {
	name, err := windows.UTF16PtrFromString(nativePath(path))
	if err != nil {
		return nil, err
	}
	access := uint32(windows.FILE_READ_ATTRIBUTES)
	if readable {
		access |= windows.GENERIC_READ
	}
	flags := uint32(windows.FILE_FLAG_BACKUP_SEMANTICS)
	if !follow {
		flags |= windows.FILE_FLAG_OPEN_REPARSE_POINT
	}
	h, err := windows.CreateFile(name, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, err
	}
	return readFileState(h, path, follow)
}

// Both browsing and handle-relative rename use this identity reader. It owns
// h from here, including closing it on any metadata/identity failure.
func readFileState(h windows.Handle, path string, follow bool) (*fileState, error) {
	f := &fileState{handle: h}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		f.close()
		return nil, err
	}
	f.identity = Identity{Volume: info.VolumeSerialNumber, File: fmt.Sprintf("%08x%08x", info.FileIndexHigh, info.FileIndexLow), Created: uint64(info.CreationTime.HighDateTime)<<32 | uint64(info.CreationTime.LowDateTime)}
	// Prefer the native 128-bit identity (required for ReFS). Keep the NTFS
	// low-64 representation compatible with histories created by earlier builds.
	var full [24]byte
	if e := windows.GetFileInformationByHandleEx(h, windows.FileIdInfo, &full[0], uint32(len(full))); e == nil {
		f.identity.VolumeID = fmt.Sprintf("%016x", binary.LittleEndian.Uint64(full[:8]))
		hi := binary.LittleEndian.Uint64(full[16:24])
		if hi == 0 {
			f.identity.File = fmt.Sprintf("%016x", binary.LittleEndian.Uint64(full[8:16]))
		} else {
			f.identity.File = hex.EncodeToString(full[8:24])
		}
	} else {
		_, fs, e := fileSystemInfo(f)
		if e != nil || !strings.EqualFold(fs, "NTFS") {
			f.close()
			return nil, errors.Join(errors.New("无法核实此文件系统的文件身份"), e)
		}
	}
	f.directory = info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	f.reparse = info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
	f.links = info.NumberOfLinks
	if follow {
		var pathBuffer [32768]uint16
		n, e := windows.GetFinalPathNameByHandle(h, &pathBuffer[0], uint32(len(pathBuffer)), 0)
		if e != nil || n >= uint32(len(pathBuffer)) {
			f.close()
			return nil, errors.Join(errors.New("无法解析实际目标路径"), e)
		}
		f.finalPath = displayPath(windows.UTF16ToString(pathBuffer[:n]))
	}
	identityPath := path
	if follow {
		identityPath = f.finalPath
	}
	if strings.HasPrefix(identityPath, `\\`) && !strings.HasPrefix(identityPath, `\\?\`) {
		f.identity.Scope = strings.ToLower(filepath.VolumeName(identityPath))
	}
	return f, nil
}

func (f *fileState) close() {
	if f != nil && f.handle != 0 {
		windows.CloseHandle(f.handle)
		f.handle = 0
	}
}
func (f *fileState) objectType() string {
	if f.directory {
		return "directory"
	}
	return "file"
}
func sameIdentity(a, b Identity) bool {
	return sameVolume(a, b) && a.File != "" && a.File == b.File
}
func sameVolume(a, b Identity) bool {
	return a.Volume == b.Volume && a.Scope == b.Scope && (a.VolumeID == "" || b.VolumeID == "" || a.VolumeID == b.VolumeID)
}

// Creation time is editable metadata, not an extra file identity component.
func sameObserved(a, b Identity) bool { return sameIdentity(a, b) }

func fileSystemInfo(f *fileState) (uint32, string, error) {
	var fs [32]uint16
	var flags uint32
	if err := windows.GetVolumeInformationByHandle(f.handle, nil, 0, nil, nil, &flags, &fs[0], uint32(len(fs))); err != nil {
		return 0, "", err
	}
	return flags, windows.UTF16ToString(fs[:]), nil
}

func readReparse(f *fileState) (uint32, string, bool, error) {
	var raw [16384]byte
	var count uint32
	if err := windows.DeviceIoControl(f.handle, windows.FSCTL_GET_REPARSE_POINT, nil, 0, &raw[0], uint32(len(raw)), &count, nil); err != nil {
		return 0, "", false, err
	}
	if count < 8 {
		return 0, "", false, errors.New("重解析数据不完整")
	}
	tag := binary.LittleEndian.Uint32(raw[:4])
	size := int(binary.LittleEndian.Uint16(raw[4:6])) + 8
	base := 16
	relative := false
	if tag == tagSymlink {
		base = 20
		if count < 20 {
			return tag, "", false, errors.New("符号链接数据不完整")
		}
		relative = binary.LittleEndian.Uint32(raw[16:20])&1 != 0
	} else if tag != tagJunction {
		return tag, "", false, errors.New("不支持的重解析类型")
	}
	if size > int(count) || size < base {
		return tag, "", relative, errors.New("重解析数据长度无效")
	}
	offset, length := int(binary.LittleEndian.Uint16(raw[8:10])), int(binary.LittleEndian.Uint16(raw[10:12]))
	if offset%2 != 0 || length%2 != 0 || length == 0 || base+offset+length > size {
		return tag, "", relative, errors.New("重解析目标长度无效")
	}
	units := make([]uint16, length/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[base+offset+i*2:])
	}
	target := string(utf16.Decode(units))
	if strings.ContainsRune(target, 0) {
		return tag, "", relative, errors.New("重解析目标包含空字符")
	}
	if !relative {
		target = displayPath(target)
	}
	return tag, target, relative, nil
}

func createNative(p Plan) error {
	switch p.Kind {
	case "hardlink":
		return os.Link(p.Source, p.LinkPath)
	case "symlink":
		return os.Symlink(p.TargetText, p.LinkPath)
	case "junction":
		return createJunction(p.LinkPath, p.TargetText)
	}
	return errors.New("链接类型无效")
}

func createJunction(path, target string) error {
	raw, err := junctionData(target)
	if err != nil {
		return err
	}
	// Refuse existing entries. If native setup fails, report the new empty
	// directory explicitly; never recursively clean a user-selected path.
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	name, _ := windows.UTF16PtrFromString(nativePath(path))
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return fmt.Errorf("新建联接目录后无法打开 %s：%w", path, err)
	}
	defer windows.CloseHandle(h)
	var count uint32
	if err := windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &raw[0], uint32(len(raw)), nil, 0, &count, nil); err != nil {
		return fmt.Errorf("未能设置联接，请检查新建空目录 %s：%w", path, err)
	}
	return nil
}

func junctionData(target string) ([]byte, error) {
	sub := utf16.Encode([]rune(`\??\` + target))
	printName := utf16.Encode([]rune(target))
	raw := make([]byte, 16+(len(sub)+len(printName)+2)*2)
	if len(raw) > 16384 {
		return nil, errors.New("目录联接的目标路径过长")
	}
	binary.LittleEndian.PutUint32(raw, tagJunction)
	binary.LittleEndian.PutUint16(raw[4:], uint16(len(raw)-8))
	binary.LittleEndian.PutUint16(raw[10:], uint16(len(sub)*2))
	binary.LittleEndian.PutUint16(raw[12:], uint16((len(sub)+1)*2))
	binary.LittleEndian.PutUint16(raw[14:], uint16(len(printName)*2))
	for i, u := range sub {
		binary.LittleEndian.PutUint16(raw[16+i*2:], u)
	}
	for i, u := range printName {
		binary.LittleEndian.PutUint16(raw[16+(len(sub)+1+i)*2:], u)
	}
	return raw, nil
}

func missing(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

func localPath(path string) (string, error) {
	path = displayPath(strings.TrimSpace(path))
	volume := filepath.VolumeName(path)
	if !filepath.IsAbs(path) || volume == "" || strings.ContainsAny(strings.TrimPrefix(path, volume), "\x00\r\n:*?\"<>|") {
		return "", errors.New("请选择有效的绝对路径")
	}
	return filepath.Clean(path), nil
}

func validLinkName(name string) bool {
	// Windows 11 accepts some DOS device names with extensions, but older
	// systems and file tools still interpret them as devices. Check the stem
	// as well so created and renamed entries stay ordinary filesystem names.
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.TrimRight(stem, " ")
	if stem != "" && !filepath.IsLocal(stem) {
		return false
	}
	return name != "" && filepath.IsLocal(name) && filepath.Base(name) == name && !strings.ContainsAny(name, "<>:\"/\\|?*\x00\r\n") && !strings.HasSuffix(name, ".") && !strings.HasSuffix(name, " ")
}

// The handle-bound file identity remains the authority even if path spelling
// changes through an ancestor junction. This prevents stale-preview creation.
func verifyPlan(p Plan) error {
	if !validLinkName(filepath.Base(p.LinkPath)) || p.SourceIdentity.File == "" || p.ParentIdentity.File == "" {
		return errors.New("链接预览身份不完整")
	}
	if _, err := localPath(p.Source); err != nil {
		return err
	}
	if _, err := localPath(p.LinkPath); err != nil {
		return err
	}
	s, err := openState(p.Source, true, false)
	if err != nil {
		return err
	}
	defer s.close()
	parent, err := openState(filepath.Dir(p.LinkPath), true, false)
	if err != nil {
		return err
	}
	defer parent.close()
	if !parent.directory || s.objectType() != p.ObjectType || (p.Kind == "hardlink" && (s.directory || !sameVolume(s.identity, parent.identity))) || (p.Kind == "junction" && !s.directory) {
		return errors.New("链接对象类型或卷已变化")
	}
	if p.Kind != "hardlink" && p.Kind != "junction" && p.Kind != "symlink" {
		return errors.New("链接操作无效")
	}
	want := p.Source
	if p.Relative {
		if p.Kind != "symlink" {
			return errors.New("此类型不支持相对目标")
		}
		want, err = filepath.Rel(filepath.Dir(p.LinkPath), p.Source)
		if err != nil {
			return err
		}
	}
	if !strings.EqualFold(filepath.Clean(want), filepath.Clean(p.TargetText)) {
		return errors.New("链接目标与预览不一致")
	}
	if !sameObserved(s.identity, p.SourceIdentity) || !sameObserved(parent.identity, p.ParentIdentity) {
		return errors.New("源文件或链接位置已变化，请重新预览")
	}
	if _, err := os.Lstat(p.LinkPath); !errors.Is(err, os.ErrNotExist) {
		return errors.New("链接路径已存在或无法确认，请重新预览")
	}
	return nil
}

// Execute is the only operation exposed to the fixed elevation helper. It
// accepts a server-authenticated preview and rechecks its identities locally.
func Execute(p Plan) error {
	if err := verifyPlan(p); err != nil {
		return err
	}
	return createNative(p)
}

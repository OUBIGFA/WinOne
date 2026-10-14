//go:build windows

package directorylinks

import (
	"context"
	"crypto/rand"
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

type ScanIssue struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}
type ScanResult struct {
	Records   []Record    `json:"records"`
	Issues    []ScanIssue `json:"issues"`
	Scanned   int         `json:"scanned"`
	Truncated bool        `json:"truncated"`
	Cancelled bool        `json:"cancelled"`
}

// Scan registers links only within the explicitly selected directory. A
// junction/symlink is inspected as an entry, never recursed into as a shortcut
// for traversing another directory or disk.
func (m *Manager) Scan(ctx context.Context, directory string, recursive bool) (ScanResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := ScanResult{Records: []Record{}, Issues: []ScanIssue{}}
	path, err := localPath(directory)
	if err != nil {
		return result, err
	}
	root, err := openState(path, true, false)
	if err != nil {
		return result, err
	}
	defer root.close()
	if !root.directory {
		return result, errors.New("请选择需要查找的文件夹")
	}
	path = root.finalPath
	known := map[string]int{}
	for i, r := range m.records {
		known[strings.ToLower(r.LinkPath)] = i
	}
	err = filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			result.Issues = append(result.Issues, ScanIssue{current, walkErr.Error()})
			return nil
		}
		if current == path {
			return nil
		}
		result.Scanned++
		if result.Scanned > 100000 || len(m.records) >= maxRecords {
			result.Truncated = true
			return fs.SkipAll
		}
		if i, ok := known[strings.ToLower(current)]; ok {
			m.records[i] = checked(m.records[i])
			result.Records = append(result.Records, m.records[i])
			if entry.IsDir() && (!recursive || m.records[i].Kind == "junction" || m.records[i].Kind == "symlink") {
				return fs.SkipDir
			}
			return nil
		}
		f, e := openState(current, false, false)
		if e != nil {
			result.Issues = append(result.Issues, ScanIssue{current, e.Error()})
			return nil
		}
		defer f.close()
		r := Record{ID: rand.Text(), Name: filepath.Base(current), LinkPath: current, TargetPath: current, ObjectType: f.objectType(), EntryIdentity: f.identity, RegisteredAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if f.reparse {
			tag, target, relative, e := readReparse(f)
			if e != nil {
				result.Issues = append(result.Issues, ScanIssue{current, e.Error()})
				return nil
			}
			r.Kind = "symlink"
			if tag == tagJunction {
				r.Kind = "junction"
			}
			r.TargetText = target
			r.Relative = relative
			r.TargetPath = target
			if relative {
				r.TargetPath = filepath.Join(filepath.Dir(current), target)
			}
		} else if !f.directory && strings.EqualFold(filepath.Ext(current), ".lnk") {
			r.Kind = "shortcut"
			target, e := shortcutTarget(current)
			if e != nil {
				result.Issues = append(result.Issues, ScanIssue{current, e.Error()})
				return nil
			}
			r.TargetPath = target
			r.TargetText = target
		} else if !f.directory && f.links > 1 {
			r.Kind = "hardlink"
			r.TargetPath = otherHardLink(current, f.identity)
			r.TargetText = r.TargetPath
		} else {
			if f.directory && !recursive {
				return fs.SkipDir
			}
			return nil
		}
		if target, e := openState(r.TargetPath, true, false); e == nil {
			r.TargetIdentity = target.identity
			if r.Kind == "shortcut" {
				r.ObjectType = target.objectType()
			}
			target.close()
		}
		r = checked(r)
		known[strings.ToLower(current)] = len(m.records)
		m.records = append(m.records, r)
		result.Records = append(result.Records, r)
		if f.reparse && entry.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
	if errors.Is(err, context.Canceled) {
		result.Cancelled = true
		err = nil
	}
	return result, errors.Join(err, m.save())
}

func otherHardLink(path string, identity Identity) string {
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	var buffer [32768]uint16
	size := uint32(len(buffer))
	name, _ := windows.UTF16PtrFromString(nativePath(path))
	h, _, _ := kernel.NewProc("FindFirstFileNameW").Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&buffer[0])))
	if h == uintptr(windows.InvalidHandle) {
		return path
	}
	defer windows.FindClose(windows.Handle(h))
	for {
		candidate := filepath.VolumeName(path) + windows.UTF16ToString(buffer[:])
		if !strings.EqualFold(filepath.Clean(candidate), filepath.Clean(path)) {
			f, err := openState(candidate, true, false)
			if err == nil {
				same := sameIdentity(f.identity, identity)
				f.close()
				if same {
					return candidate
				}
			}
		}
		size = uint32(len(buffer))
		ok, _, _ := kernel.NewProc("FindNextFileNameW").Call(h, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&buffer[0])))
		if ok == 0 {
			break
		}
	}
	return path
}

// Match Symbolic11's existing-shortcut inspection using the OS COM resolver.
// Reading TargetPath never runs the target or saves/modifies the .lnk file.
func shortcutTarget(path string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if e, ok := err.(*ole.OleError); !ok || e.Code() != 1 {
			return "", err
		}
	}
	defer ole.CoUninitialize()
	object, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return "", err
	}
	defer object.Release()
	shell, err := object.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return "", err
	}
	defer shell.Release()
	shortcut, err := oleutil.CallMethod(shell, "CreateShortcut", path)
	if err != nil {
		return "", err
	}
	defer shortcut.Clear()
	value, err := oleutil.GetProperty(shortcut.ToIDispatch(), "TargetPath")
	if err != nil {
		return "", err
	}
	defer value.Clear()
	target := value.ToString()
	if !filepath.IsAbs(target) {
		return "", errors.New("快捷方式未指向可识别的文件系统路径")
	}
	return target, nil
}

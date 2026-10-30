//go:build windows

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unsafe"

	"github.com/egoist/mygo"

	"winone/desktopmygo/core"
)

// dialogs are the window's native pickers (core.Dialogs).
type dialogs struct{ w *window }

func filters(f core.Filter) []mygo.FileFilter {
	if len(f.Extensions) == 0 {
		return nil
	}
	return []mygo.FileFilter{{Name: f.Name, Extensions: f.Extensions}}
}

func (d dialogs) PickFile(title string, filter core.Filter) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: d.w.win, Title: title, Filters: filters(filter)})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func (d dialogs) PickFiles(title string, filter core.Filter) ([]string, error) {
	return mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: d.w.win, Title: title, Filters: filters(filter), Multiple: true})
}

func (d dialogs) PickFolder(title string) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: d.w.win, Title: title, Directory: true})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func (d dialogs) PickFolders(title string) ([]string, error) {
	return mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: d.w.win, Title: title, Directory: true, Multiple: true})
}

func (d dialogs) SaveFile(title string, filter core.Filter, defaultName string) (string, error) {
	return mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: d.w.win, Title: title, Filters: filters(filter), DefaultPath: defaultName})
}

// placement keeps the window's position, size and maximized state in the
// profile (settings-window.json), the file and format of the previous
// window, so an update keeps where the user left it.
type placement struct {
	directory string
	saved     *placementRecord
}

type placementRecord struct {
	Version   int      `json:"version"`
	Normal    [4]int32 `json:"normal"`
	Maximized bool     `json:"maximized"`
}

const placementFile = "settings-window.json"

func (r placementRecord) valid() bool {
	width, height := int64(r.Normal[2])-int64(r.Normal[0]), int64(r.Normal[3])-int64(r.Normal[1])
	return r.Version == 1 && width >= 1 && width <= 32768 && height >= 1 && height <= 32768
}

type windowPlacement struct {
	Length, Flags, ShowCmd uint32
	MinPosition            [2]int32
	MaxPosition            [2]int32
	NormalPosition         [4]int32
}

var (
	procGetWindowPlacement = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement = user32.NewProc("SetWindowPlacement")
	procIsZoomed           = user32.NewProc("IsZoomed")
)

const wpfRestoreToMaximized = 0x0002

// restore applies the saved placement to the hidden window and reports
// whether it was maximized.
func (p *placement) restore(hwnd uintptr) bool {
	data, err := os.ReadFile(filepath.Join(p.directory, placementFile))
	if err != nil || len(data) > 4096 {
		return false
	}
	var saved placementRecord
	if json.Unmarshal(data, &saved) != nil || !saved.valid() {
		return false
	}
	value := windowPlacement{ShowCmd: 0, NormalPosition: saved.Normal} // SW_HIDE
	value.Length = uint32(unsafe.Sizeof(value))
	if ok, _, _ := procSetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&value))); ok == 0 {
		return false
	}
	p.saved = &saved
	return saved.Maximized
}

// save records the current placement when it changed.
func (p *placement) save(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	var value windowPlacement
	value.Length = uint32(unsafe.Sizeof(value))
	if ok, _, _ := procGetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&value))); ok == 0 {
		return
	}
	zoomed, _, _ := procIsZoomed.Call(hwnd)
	iconic, _, _ := procIsIconic.Call(hwnd)
	record := placementRecord{Version: 1, Normal: value.NormalPosition,
		Maximized: zoomed != 0 || iconic != 0 && value.Flags&wpfRestoreToMaximized != 0}
	if !record.valid() || p.saved != nil && *p.saved == record {
		return
	}
	data, _ := json.Marshal(record)
	temporary := filepath.Join(p.directory, ".settings-window-"+strconv.FormatInt(time.Now().UnixNano(), 36)+".tmp")
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		reportError(fmt.Errorf("无法保存窗口位置：%w", err))
		return
	}
	if err := os.Rename(temporary, filepath.Join(p.directory, placementFile)); err != nil {
		_ = os.Remove(temporary)
		reportError(fmt.Errorf("无法保存窗口位置：%w", err))
		return
	}
	p.saved = &record
}

// runProbe is the installer's startup check (--check-ui). The MyGo
// window does not provide it yet; the installer keeps using the window
// that does until the switch (docs/MyGo前端迁移.md, stage 4).
func runProbe(args []string) error {
	return errors.New("启动检查尚未接入 MyGo 窗口")
}

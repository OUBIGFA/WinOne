//go:build windows

package tray

import (
	"bytes"
	"crypto/rand"
	"errors"
	"image/png"
	"sort"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"winone/modules/wintray/internal/traybox"
)

// HostedMenuEntry is a presentation-only snapshot for the WinOne shell.
// Its random lifetime ID never grants access to a reused PID or HWND.
type HostedMenuEntry struct {
	ID, Name, ExePath string
	Image             []byte
}

// SetExternalMenu is called before adding programs in WinOne-managed mode.
// Hidden owner windows remain for dispatch; no independent tray UI is created.
func (h *Host) SetExternalMenu(enabled bool) { h.externalMenu = enabled }

func (item *hostedItem) prepareExternalMenu() error {
	item.menuID = "hosted:" + rand.Text()
	img, owned := programIcon(item.info.ExePath)
	if owned {
		defer img.Dispose()
	}
	if img != nil {
		bitmap, err := walk.NewBitmapFromImageWithSize(img, walk.Size{Width: 20, Height: 20})
		if err != nil {
			return err
		}
		defer bitmap.Dispose()
		pixels, err := bitmap.ToImage()
		if err != nil {
			return err
		}
		var data bytes.Buffer
		if err = png.Encode(&data, pixels); err != nil {
			return err
		}
		item.menuImage = data.Bytes()
	}
	return nil
}

// MenuEntries runs on the worker UI thread, like the native collector snapshot.
func (h *Host) MenuEntries() []HostedMenuEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries := []HostedMenuEntry{}
	for _, item := range h.items {
		if item.closed || item.menuID == "" {
			continue
		}
		entries = append(entries, HostedMenuEntry{item.menuID, item.info.Name, item.info.ExePath, bytes.Clone(item.menuImage)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

// ActivateMenu accepts only a live item and one of the shell's fixed actions.
// Release/quit remain asynchronous requests; existing watchers confirm removal.
func (h *Host) ActivateMenu(id, action string) error {
	h.mu.Lock()
	var found *hostedItem
	for _, item := range h.items {
		if !item.closed && item.menuID == id && id != "" {
			found = item
			break
		}
	}
	h.mu.Unlock()
	if found == nil {
		return traybox.ErrIconGone
	}
	// Pin the original process for the complete action. PID reuse can happen
	// after the watcher notices exit but before its UI cleanup callback runs.
	process, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, found.info.ProcessID)
	if err != nil {
		return traybox.ErrIconGone
	}
	defer windows.CloseHandle(process)
	var created, exited, kernel, user windows.Filetime
	state, waitErr := windows.WaitForSingleObject(process, 0)
	if err = windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil || waitErr != nil || state != uint32(windows.WAIT_TIMEOUT) || created != found.createdAt {
		return traybox.ErrIconGone
	}
	switch action {
	case "", "open":
		if !found.showWindow(true) {
			return errors.New("程序窗口尚未就绪")
		}
	case "hide":
		hwnd := found.window()
		if hwnd == 0 || !showHostedWindowAsync(hwnd, win.SW_HIDE) {
			return errors.New("程序窗口未能隐藏")
		}
	case "release":
		if !found.releaseHosting() {
			return errors.New("程序窗口未能恢复，已保留托管")
		}
	case "quit":
		return found.quitProgram()
	default:
		return errors.New("不支持的程序操作")
	}
	return nil
}

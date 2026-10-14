//go:build windows

package directorylinks

import (
	"errors"
	"golang.org/x/sys/windows"
	"path/filepath"
)

// Reveal selects the chosen filesystem entry in Explorer. It never executes a
// file target, shell shortcut, URL or command supplied by a record.
func (m *Manager) Reveal(id, side string) error {
	m.mu.Lock()
	i := m.index(id)
	if i < 0 {
		m.mu.Unlock()
		return errors.New("链接记录不存在")
	}
	r := m.records[i]
	m.mu.Unlock()
	path := r.LinkPath
	if side == "target" {
		path = r.TargetPath
	} else if side != "link" {
		return errors.New("位置类型无效")
	}
	path, err := localPath(path)
	if err != nil {
		return err
	}
	windowsDir, err := windows.GetWindowsDirectory()
	if err != nil {
		return err
	}
	exe, _ := windows.UTF16PtrFromString(filepath.Join(windowsDir, "explorer.exe"))
	verb, _ := windows.UTF16PtrFromString("open")
	params, _ := windows.UTF16PtrFromString("/select," + windows.ComposeCommandLine([]string{path}))
	return windows.ShellExecute(0, verb, exe, params, nil, windows.SW_SHOWNORMAL)
}

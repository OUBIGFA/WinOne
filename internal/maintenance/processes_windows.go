//go:build windows

package maintenance

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

func IsProductProcessName(name string) bool {
	switch strings.ToLower(name) {
	case "winone.exe", "winone.host.exe", "winone.wintray.exe", "winone.wintray.recovery.exe", "winone.ai.exe", "winone.capture.exe", "winone.capture.regions.exe", "winone.alwaysontop.exe", "winone.results.exe", "winone.diagrams.exe", "winone.selection.exe", "winone.stroke.exe":
		return true
	}
	return false
}

// RequireStopped retains the explicit machine-wide check used by system tests.
func RequireStopped() error {
	return requireStopped("")
}

// RequireStoppedAt checks only binaries in the installation being maintained.
// The caller's installation/profile leases separately prevent another install
// from using the same profile. An unrelated copy must not block this update.
func RequireStoppedAt(installation string) error {
	if !filepath.IsAbs(installation) {
		return errors.New("安装目录必须是绝对路径")
	}
	resolved, err := filepath.EvalSymlinks(installation)
	if err != nil {
		return err
	}
	return requireStopped(resolved)
}

func processInInstallation(pid uint32, installation string) (bool, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return false, nil // Process exited after the Toolhelp snapshot.
	}
	if err != nil {
		return false, fmt.Errorf("打开进程身份句柄：%w", err)
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return false, processErrorUnlessExited(process, fmt.Errorf("查询进程映像：%w", err))
	}
	path, err := filepath.EvalSymlinks(windows.UTF16ToString(buffer[:size]))
	if err != nil {
		return false, processErrorUnlessExited(process, fmt.Errorf("解析进程路径：%w", err))
	}
	relative, err := filepath.Rel(installation, path)
	if err != nil {
		return false, nil // A different volume cannot be inside this installation.
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}

// Suppress a stale snapshot error only when the same pinned process is proven
// exited. An unreadable live process or failed wait must still block maintenance.
func processErrorUnlessExited(process windows.Handle, cause error) error {
	if state, err := windows.WaitForSingleObject(process, 0); err == nil && state == windows.WAIT_OBJECT_0 {
		return nil
	}
	return cause
}

// Only ERROR_NO_MORE_FILES proves that enumeration finished successfully.
func requireStopped(installation string) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return fmt.Errorf("无法检查运行中的程序：%w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if entry.ProcessID != uint32(os.Getpid()) && IsProductProcessName(name) {
			inside := true
			if installation != "" {
				inside, err = processInInstallation(entry.ProcessID, installation)
				if err != nil {
					return fmt.Errorf("无法核实 WinOne 进程 %d 的安装位置：%w", entry.ProcessID, err)
				}
			}
			if inside {
				return fmt.Errorf("请先退出此安装的 WinOne 及其模块：%s（PID %d）", name, entry.ProcessID)
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("无法完成程序检查：%w", err)
	}
	return nil
}

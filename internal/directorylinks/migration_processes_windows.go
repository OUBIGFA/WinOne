//go:build windows

package directorylinks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Process-image inspection is read-only and best effort. Failure to query a
// protected process is not proof of no use; exclusive file opens and the
// namespace transaction remain separate checks. No process is ever closed.
func checkMigrationProcesses(ctx context.Context, root string) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		if e := ctx.Err(); e != nil {
			return e
		}
		h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if e == nil {
			buffer := make([]uint16, 32768)
			size := uint32(len(buffer))
			e = windows.QueryFullProcessImageName(h, 0, &buffer[0], &size)
			windows.CloseHandle(h)
			if e == nil {
				path := windows.UTF16ToString(buffer[:size])
				if migrationWithin(filepath.Clean(path), root) {
					return fmt.Errorf("目录中有正在运行的程序（PID %d），请先退出后重新预览", entry.ProcessID)
				}
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	return nil
}

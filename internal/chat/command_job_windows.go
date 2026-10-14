//go:build windows

package chat

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"sync"
	"time"
	"unsafe"
)

// RetainCommandJob is called before init, while the worker cannot spawn user
// commands. The retained handle lets a command caller verify all descendants
// have exited before reporting success or releasing its execution claim.
func (r *Runner) RetainCommandJob() (func(context.Context) error, error) {
	var job windows.Handle
	process := windows.CurrentProcess()
	if err := windows.DuplicateHandle(process, r.job, process, &job, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	var once sync.Once
	var result error
	return func(ctx context.Context) error {
		once.Do(func() {
			defer windows.CloseHandle(job)
			if err := windows.TerminateJobObject(job, 1); err != nil {
				result = err
				return
			}
			// JOBOBJECT_BASIC_ACCOUNTING_INFORMATION, fixed Win32 layout.
			var info struct {
				Times                                 [4]int64
				PageFaults, Total, Active, Terminated uint32
			}
			for {
				if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
					result = err
					return
				}
				if info.Active == 0 {
					return
				}
				select {
				case <-ctx.Done():
					result = errors.New("脚本进程组尚未全部退出")
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
		})
		return result
	}, nil
}

//go:build windows

package main

import (
	"errors"
	"golang.org/x/sys/windows"
	"time"
	"unsafe"
)

// Only the dedicated, disposable startup-probe process may join this job.
// Its WebView descendants cannot outlive an aborted probe. A normal product
// window must not use this: its independently running Host must be preserved.
type probeProcessGroup struct{ handle windows.Handle }

func ownProbeProcessGroup() (*probeProcessGroup, error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(handle, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	if err = windows.AssignProcessToJobObject(handle, windows.CurrentProcess()); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	return &probeProcessGroup{handle: handle}, nil
}

// JOB_OBJECT_BASIC_ACCOUNTING_INFORMATION, as defined by the Windows SDK.
type probeJobAccounting struct {
	TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
	TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
}

func (g *probeProcessGroup) complete() error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		var info probeJobAccounting
		if err := windows.QueryInformationJobObject(g.handle, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
			return err
		}
		if info.ActiveProcesses == 1 {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("WebView2 startup-probe processes did not exit")
		}
		time.Sleep(25 * time.Millisecond)
	}
	// The owner is the sole remaining member. Disarm before its normal exit.
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	if _, err := windows.SetInformationJobObject(g.handle, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return err
	}
	err := windows.CloseHandle(g.handle)
	if err == nil {
		g.handle = 0
	}
	return err
}

func (g *probeProcessGroup) abort() {
	if g.handle != 0 {
		windows.CloseHandle(g.handle)
	}
}

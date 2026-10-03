//go:build windows

package startup

import (
	"errors"
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"runtime"
)

// Only GetTask's explicit file-not-found HRESULT proves absence. Localized schtasks
// output and failed queries cannot distinguish absence from denied access.
func taskNotFound(err error) bool {
	var failure *ole.OleError
	if !errors.As(err, &failure) {
		return false
	}
	code := uint32(failure.Code())
	if code == 0x80020009 {
		if details, ok := failure.SubError().(interface{ SCODE() uint32 }); ok {
			code = details.SCODE()
		}
	}
	return code == 0x80070002 // HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND)
}

func scheduledTaskExists(name string) (bool, error) {
	if name == "" {
		return false, errors.New("task name is empty")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	initialized := err == nil
	if err != nil {
		var failure *ole.OleError
		if !errors.As(err, &failure) {
			return false, err
		}
		switch uint32(failure.Code()) {
		case 1:
			initialized = true // S_FALSE still acquires an initialization reference.
		case 0x80010106: // Reuse the thread's existing apartment without uninitializing it.
		default:
			return false, err
		}
	}
	if initialized {
		defer ole.CoUninitialize()
	}
	unknown, err := oleutil.CreateObject("Schedule.Service")
	if err != nil {
		return false, err
	}
	defer unknown.Release()
	service, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return false, err
	}
	defer service.Release()
	connected, err := oleutil.CallMethod(service, "Connect")
	if connected != nil {
		connected.Clear()
	}
	if err != nil {
		return false, err
	}
	folderValue, err := oleutil.CallMethod(service, "GetFolder", `\`)
	if folderValue != nil {
		defer folderValue.Clear()
	}
	if err != nil {
		return false, err
	}
	if folderValue == nil || folderValue.ToIDispatch() == nil {
		return false, errors.New("task folder unavailable")
	}
	task, err := oleutil.CallMethod(folderValue.ToIDispatch(), "GetTask", name)
	if task != nil {
		defer task.Clear()
	}
	if err != nil {
		if taskNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if task == nil || task.ToIDispatch() == nil {
		return false, errors.New("task query returned no object")
	}
	return true, nil
}

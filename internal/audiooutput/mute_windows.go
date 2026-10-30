//go:build windows

// Package audiooutput adapts Windows Core Audio for the existing voice mute
// guard. It only changes the default output's temporary mute bit, never volume.
package audiooutput

import (
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"syscall"
	"unsafe"
)

type object struct{ table *[16]uintptr }

func (o *object) call(index int, args ...uintptr) uintptr {
	values := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	result, _, _ := syscall.SyscallN(o.table[index], values...)
	runtime.KeepAlive(o)
	return result
}
func check(result uintptr) error {
	if int32(result) < 0 {
		return fmt.Errorf("Windows 音频操作失败 (HRESULT 0x%08x)", uint32(result))
	}
	return nil
}

// Mute reads the default render endpoint. A non-nil desired value additionally
// sets its mute state. COM's thread affinity lasts until all objects are released.
// Interfaces: IMMDeviceEnumerator::GetDefaultAudioEndpoint, IMMDevice::Activate,
// IAudioEndpointVolume::GetMute/SetMute (Microsoft Core Audio API).
func Mute(desired *bool) (bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	result, _, _ := ole.NewProc("CoInitializeEx").Call(0, 0)
	if err := check(result); err != nil {
		return false, err
	}
	defer ole.NewProc("CoUninitialize").Call()
	clsid, _ := windows.GUIDFromString("{BCDE0395-E52F-467C-8E3D-C4579291692E}")
	enumeratorID, _ := windows.GUIDFromString("{A95664D2-9614-4F35-A746-DE8DB63617E6}")
	volumeID, _ := windows.GUIDFromString("{5CDF2C82-841E-4546-9722-0CF74078229A}")
	var enumerator, device, volume *object
	result, _, _ = ole.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&enumeratorID)), uintptr(unsafe.Pointer(&enumerator)))
	if err := check(result); err != nil {
		return false, err
	}
	defer enumerator.call(2)
	if err := check(enumerator.call(4, 0, 1, uintptr(unsafe.Pointer(&device)))); err != nil {
		return false, err
	}
	defer device.call(2)
	if err := check(device.call(3, uintptr(unsafe.Pointer(&volumeID)), 23, 0, uintptr(unsafe.Pointer(&volume)))); err != nil {
		return false, err
	}
	defer volume.call(2)
	var muted int32
	if err := check(volume.call(15, uintptr(unsafe.Pointer(&muted)))); err != nil {
		return false, err
	}
	if desired != nil {
		var next uintptr
		if *desired {
			next = 1
		}
		if err := check(volume.call(14, next, 0)); err != nil {
			return muted != 0, err
		}
		return *desired, nil
	}
	return muted != 0, nil
}

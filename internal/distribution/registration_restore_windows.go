//go:build windows

package distribution

import (
	"bytes"
	"encoding/binary"
	"errors"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

// RestoreRegistration preserves raw types and bytes in the explicitly named
// current-user key. Callers hold their maintenance lease and durable operation
// journal. Registry value writes are not a multi-value atomic transaction;
// failures are returned with partial progress left for the journal to recover.
func RestoreRegistration(directory, keyPath string, original, expectedCurrent RegistrationSnapshot) error {
	for _, snapshot := range []RegistrationSnapshot{original, expectedCurrent} {
		if err := validateRegistrationRestore(directory, keyPath, snapshot); err != nil {
			return err
		}
	}
	current, err := CaptureRegistration(directory, keyPath)
	if err != nil {
		return err
	}
	if sameRegistration(current, original) {
		return nil
	}
	if !sameRegistration(current, expectedCurrent) {
		return errors.New("registration changed outside the upgrade")
	}
	if err = replaceRegistrationValues(keyPath, original, current); err != nil {
		return err
	}
	restored, err := CaptureRegistration(directory, keyPath)
	if err != nil {
		return err
	}
	if !sameRegistration(restored, original) {
		return errors.New("registration restoration verification failed")
	}
	return nil
}

func sameRegistration(a, b RegistrationSnapshot) bool {
	if a.Exists != b.Exists || !strings.EqualFold(a.KeyPath, b.KeyPath) || len(a.Values) != len(b.Values) {
		return false
	}
	values := map[string]RegistrationValue{}
	for _, value := range a.Values {
		values[strings.ToLower(value.Name)] = value
	}
	for _, value := range b.Values {
		other, ok := values[strings.ToLower(value.Name)]
		if !ok || other.Type != value.Type || !bytes.Equal(other.Data, value.Data) {
			return false
		}
	}
	return true
}

func validateRegistrationRestore(directory, keyPath string, snapshot RegistrationSnapshot) error {
	if snapshot.SchemaVersion != 1 || keyPath == "" || strings.HasPrefix(keyPath, `\`) || !strings.EqualFold(snapshot.KeyPath, keyPath) || len(snapshot.Values) > 64 {
		return errors.New("invalid registration snapshot destination")
	}
	if !snapshot.Exists {
		if len(snapshot.Values) != 0 {
			return errors.New("absent registration contains values")
		}
		return nil
	}
	seen := map[string]bool{}
	total := 0
	location := ""
	for _, value := range snapshot.Values {
		name := strings.ToLower(value.Name)
		if seen[name] || strings.ContainsRune(value.Name, 0) || len(value.Name) > 16383 || len(value.Data) > 65536 {
			return errors.New("invalid registration snapshot value")
		}
		seen[name] = true
		total += len(value.Data)
		if total > 1<<20 {
			return errors.New("registration snapshot exceeds limit")
		}
		switch value.Type {
		case registry.NONE, registry.BINARY:
		case registry.DWORD:
			if len(value.Data) != 4 {
				return errors.New("invalid DWORD snapshot")
			}
		case registry.QWORD:
			if len(value.Data) != 8 {
				return errors.New("invalid QWORD snapshot")
			}
		case registry.SZ, registry.EXPAND_SZ, registry.MULTI_SZ:
			if len(value.Data)%2 != 0 {
				return errors.New("invalid UTF-16 snapshot")
			}
		default:
			return errors.New("unsupported registration value type")
		}
		if name == "installlocation" {
			if value.Type != registry.SZ && value.Type != registry.EXPAND_SZ {
				return errors.New("invalid registration location type")
			}
			text := make([]uint16, len(value.Data)/2)
			for i := range text {
				text[i] = binary.LittleEndian.Uint16(value.Data[i*2:])
			}
			location = windows.UTF16ToString(text)
		}
	}
	if !filepath.IsAbs(location) {
		return errors.New("snapshot location is not absolute")
	}
	wanted, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	wanted, err = filepath.EvalSymlinks(wanted)
	if err != nil {
		return err
	}
	actual, err := filepath.EvalSymlinks(location)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(wanted), filepath.Clean(actual)) {
		return errors.New("snapshot belongs to another installation")
	}
	return nil
}

func writeRegistrationValue(key registry.Key, value RegistrationValue) error {
	name, err := windows.UTF16PtrFromString(value.Name)
	if err != nil {
		return err
	}
	var data *byte
	if len(value.Data) > 0 {
		data = &value.Data[0]
	}
	result, _, _ := windows.NewLazySystemDLL("advapi32.dll").NewProc("RegSetValueExW").Call(uintptr(key), uintptr(unsafe.Pointer(name)), 0, uintptr(value.Type), uintptr(unsafe.Pointer(data)), uintptr(len(value.Data)))
	runtime.KeepAlive(value.Data)
	runtime.KeepAlive(name)
	if result != 0 {
		return windows.Errno(result)
	}
	return nil
}

// Select the same registry view as capture even if this library is built by a
// 32-bit helper. A predefined HKCU handle alone does not select that view.
func deleteRegistrationKey64(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	result, _, _ := windows.NewLazySystemDLL("advapi32.dll").NewProc("RegDeleteKeyExW").Call(uintptr(registry.CURRENT_USER), uintptr(unsafe.Pointer(name)), uintptr(registry.WOW64_64KEY), 0)
	runtime.KeepAlive(name)
	if result != 0 {
		return windows.Errno(result)
	}
	return nil
}

// Used after validating the bound snapshots and the observed current state.
func replaceRegistrationValues(keyPath string, desired, current RegistrationSnapshot) error {
	observed, err := captureRegistration("", keyPath, false)
	if err != nil {
		return err
	}
	if !sameRegistration(observed, current) {
		return errors.New("registration changed before mutation")
	}
	if !desired.Exists {
		if err := deleteRegistrationKey64(keyPath); err != nil {
			return err
		}
	} else {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.READ|registry.SET_VALUE|registry.WOW64_64KEY)
		if err != nil {
			return err
		}
		defer key.Close()
		wanted := map[string]bool{}
		for _, value := range desired.Values {
			if err := writeRegistrationValue(key, value); err != nil {
				return err
			}
			wanted[strings.ToLower(value.Name)] = true
		}
		for _, value := range current.Values {
			if !wanted[strings.ToLower(value.Name)] {
				if err := key.DeleteValue(value.Name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

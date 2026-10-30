//go:build windows

package app

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                    = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW      = user32.NewProc("RegisterClassExW")
	procUnregisterClassW      = user32.NewProc("UnregisterClassW")
	procCreateWindowExW       = user32.NewProc("CreateWindowExW")
	procDestroyWindow         = user32.NewProc("DestroyWindow")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procFindWindowExW         = user32.NewProc("FindWindowExW")
	procGetWindowThreadPID    = user32.NewProc("GetWindowThreadProcessId")
	procAllowSetForeground    = user32.NewProc("AllowSetForegroundWindow")
	procSendMessageTimeoutW   = user32.NewProc("SendMessageTimeoutW")
	procSetWindowLongPtrW     = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtrW     = user32.NewProc("GetWindowLongPtrW")
	procIsWindowVisible       = user32.NewProc("IsWindowVisible")
	procIsIconic              = user32.NewProc("IsIconic")
	procShowWindow            = user32.NewProc("ShowWindow")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow   = user32.NewProc("GetForegroundWindow")
	procAttachThreadInput     = user32.NewProc("AttachThreadInput")
	procBringWindowToTop      = user32.NewProc("BringWindowToTop")
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandleW      = kernel32.NewProc("GetModuleHandleW")
	procQueryFullProcessImage = kernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	wmCopyData      = 0x004A
	copyInstance    = 1542
	maxNotification = 65536
	hwndMessage     = ^uintptr(2) // HWND_MESSAGE
	gwlpUserData    = ^uintptr(20) // GWLP_USERDATA (-21)
	smtoBlock       = 0x0001
	smtoAbortIfHung = 0x0002
	swRestore       = 9
	swShow          = 5
)

// names returns the window identity the Host (windowinstance.FindPID,
// residency) and the previous desktops share: the Wails single-instance
// mutex and message-only window, and the presence event.
func names(pipe string) (mutex, class, title, presence string) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(pipe)))
	base := "wails-app-winone-desktop-" + hash
	return base + "sim", base + "-sic", base + "-siw", `Local\WinOne-Desktop-` + hash
}

// instance owns the settings window's identity for one Host pipe.
type instance struct {
	mutex        windows.Handle
	class, title *uint16
	notification uintptr
	// wake runs on the UI thread when another launch asks for the window;
	// settings is true for --open-settings.
	wake func(settings bool)
}

var (
	current      *instance
	wndProc      uintptr
	registerOnce sync.Once
)

// claimInstance takes the identity for pipe. A nil instance with a nil
// error means an existing, verified window accepted the request: this
// launch is done. A window that is still starting or does not answer is an
// error, never permission to open a second window for the same Host.
func claimInstance(pipe string, openSettings bool) (*instance, error) {
	mutexName, className, titleName, _ := names(pipe)
	name, _ := windows.UTF16PtrFromString(mutexName)
	mutex, err := windows.CreateMutex(nil, false, name)
	if mutex == 0 {
		return nil, err
	}
	class, _ := windows.UTF16PtrFromString(className)
	title, _ := windows.UTF16PtrFromString(titleName)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		defer windows.CloseHandle(mutex)
		deadline := time.Now().Add(5 * time.Second)
		for {
			window, _, _ := procFindWindowExW.Call(hwndMessage, 0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)))
			if window != 0 {
				return nil, notifyExisting(window, openSettings)
			}
			if time.Now().After(deadline) {
				return nil, errors.New("设置窗口尚未就绪，请稍后重试。")
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	return &instance{mutex: mutex, class: class, title: title}, nil
}

// attach creates the message-only window that receives other launches'
// requests, pointing it at target, the settings window. It runs on the UI
// thread, whose message loop delivers the requests.
func (i *instance) attach(target uintptr, wake func(settings bool)) error {
	registerOnce.Do(func() { wndProc = windows.NewCallback(notificationProc) })
	module, _, _ := procGetModuleHandleW.Call(0)
	class := struct {
		Size, Style                        uint32
		Proc                               uintptr
		ClassExtra, WindowExtra            int32
		Instance, Icon, Cursor, Background uintptr
		Menu, Name                         *uint16
		SmallIcon                          uintptr
	}{Proc: wndProc, Instance: module, Name: i.class}
	class.Size = uint32(unsafe.Sizeof(class))
	if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return err
	}
	window, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(i.class)), uintptr(unsafe.Pointer(i.title)), 0, 0, 0, 0, 0, hwndMessage, 0, module, 0)
	if window == 0 {
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(i.class)), module)
		return err
	}
	// An elevated launcher activates the settings window found through
	// this one (activateRedirected); keep its handle here, as before.
	procSetWindowLongPtrW.Call(window, gwlpUserData, target)
	i.notification, i.wake = window, wake
	current = i
	return nil
}

// close releases the identity. Run it on the UI thread.
func (i *instance) close() {
	if i.notification != 0 {
		procDestroyWindow.Call(i.notification)
		module, _, _ := procGetModuleHandleW.Call(0)
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(i.class)), module)
		i.notification = 0
	}
	if current == i {
		current = nil
	}
	windows.CloseHandle(i.mutex)
}

type copyDataStruct struct {
	Data   uintptr
	Size   uint32
	Buffer *uint16
}

func notificationProc(window, message, wparam, lparam uintptr) uintptr {
	if message == wmCopyData && lparam != 0 {
		// lparam is the sender's COPYDATASTRUCT, valid during this call.
		data := *(**copyDataStruct)(unsafe.Pointer(&lparam))
		// Wails' sender declares one extra byte: read whole UTF-16 units
		// within the declared size and require a terminator inside it.
		if data.Data != copyInstance || data.Buffer == nil || data.Size < 2 || data.Size > maxNotification {
			return 0
		}
		units := unsafe.Slice(data.Buffer, data.Size/2)
		end := -1
		for i, unit := range units {
			if unit == 0 {
				end = i
				break
			}
		}
		if end < 0 {
			return 0
		}
		var request struct {
			Args []string `json:"Args"`
		}
		if json.Unmarshal([]byte(windows.UTF16ToString(units[:end])), &request) != nil {
			return 0
		}
		settings, ok := wakeRequest(request.Args)
		if !ok || current == nil || current.notification != window || current.wake == nil {
			return 0
		}
		current.wake(settings)
		return 1
	}
	result, _, _ := procDefWindowProcW.Call(window, message, wparam, lparam)
	return result
}

// notifyExisting forwards this launch to the verified window of the same
// installation and Windows account.
func notifyExisting(window uintptr, openSettings bool) error {
	process, err := verifiedProcess(window)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	var pid uint32
	procGetWindowThreadPID.Call(window, uintptr(unsafe.Pointer(&pid)))
	procAllowSetForeground.Call(uintptr(pid))
	args := []string{}
	if openSettings {
		args = append(args, "--open-settings")
	}
	payload, _ := json.Marshal(map[string]any{"Args": args, "WorkingDirectory": ""})
	text, _ := windows.UTF16FromString(string(payload))
	data := copyDataStruct{Data: copyInstance, Size: uint32(len(text) * 2), Buffer: &text[0]}
	var result uintptr
	sent, _, _ := procSendMessageTimeoutW.Call(window, wmCopyData, 0, uintptr(unsafe.Pointer(&data)), smtoAbortIfHung|smtoBlock, 2000, uintptr(unsafe.Pointer(&result)))
	if sent == 0 {
		return errors.New("已有设置窗口未响应，请稍后重试。")
	}
	if result != 1 {
		return errors.New("已有窗口未接受打开请求。")
	}
	return nil
}

// verifiedProcess opens the process owning window after checking that it
// runs this executable, for this Windows account, and is alive.
func verifiedProcess(window uintptr) (windows.Handle, error) {
	var pid uint32
	procGetWindowThreadPID.Call(window, uintptr(unsafe.Pointer(&pid)))
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return 0, err
	}
	fail := func(err error) (windows.Handle, error) { windows.CloseHandle(process); return 0, err }
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if ok, _, err := procQueryFullProcessImage.Call(uintptr(process), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size))); ok == 0 {
		return fail(err)
	}
	actual, err := canonical(windows.UTF16ToString(buffer[:size]))
	if err != nil {
		return fail(err)
	}
	executable, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	expected, err := canonical(executable)
	if err != nil {
		return fail(err)
	}
	if !strings.EqualFold(actual, expected) {
		return fail(errors.New("此配置已由另一安装目录的窗口打开。"))
	}
	same, err := sameUser(process)
	if err != nil {
		return fail(err)
	}
	if !same {
		return fail(errors.New("窗口属于其他 Windows 账户。"))
	}
	if event, _ := windows.WaitForSingleObject(process, 0); event != uint32(windows.WAIT_TIMEOUT) {
		return fail(errors.New("设置窗口已经退出，请重试。"))
	}
	var now uint32
	procGetWindowThreadPID.Call(window, uintptr(unsafe.Pointer(&now)))
	if now != pid {
		return fail(errors.New("设置窗口已变化，请重试。"))
	}
	return process, nil
}

func canonical(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

func sameUser(process windows.Handle) (bool, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return false, err
	}
	defer token.Close()
	other, err := token.GetTokenUser()
	if err != nil {
		return false, err
	}
	mine, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return false, err
	}
	return windows.EqualSid(other.User.Sid, mine.User.Sid), nil
}

// activateRedirected hands this launch's foreground right to the window an
// authorized administrator start opened, once it exists. A lower-integrity
// launcher cannot send it WM_COPYDATA (UIPI); the elevated launch already
// asked for the window, so this only activates it.
func activateRedirected(pipe string) error {
	_, className, titleName, _ := names(pipe)
	class, _ := windows.UTF16PtrFromString(className)
	title, _ := windows.UTF16PtrFromString(titleName)
	deadline := time.Now().Add(8 * time.Second)
	for {
		window, _, _ := procFindWindowExW.Call(hwndMessage, 0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)))
		if window != 0 {
			process, err := verifiedProcess(window)
			if err != nil {
				return err
			}
			windows.CloseHandle(process)
			var pid, targetPID uint32
			procGetWindowThreadPID.Call(window, uintptr(unsafe.Pointer(&pid)))
			procAllowSetForeground.Call(uintptr(pid))
			target, _, _ := procGetWindowLongPtrW.Call(window, gwlpUserData)
			procGetWindowThreadPID.Call(target, uintptr(unsafe.Pointer(&targetPID)))
			if visible, _, _ := procIsWindowVisible.Call(target); targetPID == pid && visible != 0 {
				if iconic, _, _ := procIsIconic.Call(target); iconic != 0 {
					procShowWindow.Call(target, swRestore)
				}
				procSetForegroundWindow.Call(target)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("管理员窗口尚未就绪，请再次打开 WinOne。")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

//go:build windows

package app

import (
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"

	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/shell"
	internalhost "winone/internal/host"
)

// window is the settings window and its lifetime: closing keeps it in the
// background when the settings say so, unsaved changes and running work
// ask first, and the Host's tray exit (WM_CLOSE with wParam 1) or its
// normal exit closes it for good.
type window struct {
	options  Options
	launch   plan
	client   *host.Client
	win      *mygo.Window
	hwnd     uintptr
	shell    *shell.Shell
	instance *instance
	place    *placement

	keepRunning bool
	// exit is set by the Host's tray exit and by a Host that ended: the
	// next close ends the process instead of hiding the window.
	exit     atomic.Bool
	approved bool
	shown    bool
	stop     chan struct{}
}

// runWindow opens the settings window and runs until the process ends.
func runWindow(o Options, launch plan) error {
	inst, err := claimInstance(o.Pipe, o.OpenSettings)
	if err != nil || inst == nil {
		return err
	}
	releasePresence, err := markPresence(o.Pipe)
	if err != nil {
		inst.close()
		return err
	}
	defer releasePresence()
	client, err := host.NewClient(o.Pipe)
	if err != nil {
		inst.close()
		return err
	}
	w := &window{options: o, launch: launch, client: client, instance: inst, keepRunning: true, stop: make(chan struct{})}
	env := &core.Env{Host: client, Dialogs: dialogs{w: w}}
	env.Post = func(fn func()) {
		if w.win != nil && !w.win.IsDestroyed() {
			w.win.Update(fn)
		}
	}
	w.shell = shell.New(env, w, shell.Options{
		Directory:      o.Directory,
		EnsureHost:     func() (internalhost.Snapshot, error) { return ensureHost(client, o.Pipe, o.Directory, launch) },
		StartupWarning: launch.warning,
		OpenSettings:   o.OpenSettings,
	})
	w.place = &placement{directory: o.Directory}
	mygo.App.WhenReady(w.open)
	mygo.App.OnQuit(func() {
		close(w.stop)
		client.Cancel()
		w.instance.close()
	})
	return mygo.App.Run()
}

func (w *window) open() {
	w.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "WinOne",
		Width:             1280,
		Height:            840,
		MinWidth:          800,
		MinHeight:         560,
		TitleBarStyle:     mygo.TitleBarHidden,
		HideWindowButtons: true,
		Hidden:            true,
		BackgroundColor:   "light-dark(#fcfcfc, #191919)",
		Content:           ui.View(w.shell.View),
	})
	w.hwnd = w.win.NativeHandle()
	subclass(w.hwnd, &w.exit)
	if err := w.instance.attach(w.hwnd, w.wake); err != nil {
		reportError(err)
	}
	maximized := w.place.restore(w.hwnd)
	w.win.OnClose(w.onClose)
	// Native content draws its first frame at once; only a web page's
	// window waits for OnReadyToShow, which MyGo fires for pages alone.
	w.shown = true
	w.win.Show()
	if maximized {
		w.win.Maximize()
	}
	activate(w.hwnd)
	// A Host that exits normally (tray exit, update) takes the window
	// with it; a crash leaves the drafts on screen.
	w.client.OnServer(func(server *host.ServerProcess) {
		go func() {
			code, ended := server.Wait(w.stop)
			if ended && code == 0 && !w.client.Cancelled() {
				w.win.Update(func() {
					w.exit.Store(true)
					w.win.Close()
				})
			}
		}()
	})
	w.shell.Start()
}

// wake runs when another launch asks for the window.
func (w *window) wake(settings bool) {
	w.reveal()
	w.shell.Wake(settings)
	w.win.Invalidate()
}

func (w *window) reveal() {
	if w.win == nil {
		return
	}
	w.shown = true
	if w.win.IsMinimized() {
		w.win.Restore()
	}
	w.win.Show()
	activate(w.hwnd)
}

func (w *window) onClose(e *mygo.CloseEvent) {
	exit := w.exit.Load()
	if w.approved {
		w.approved = false
	} else if !w.shell.RequestClose() {
		// The shell asks about the unsaved changes or waits for the work.
		e.PreventDefault()
		w.reveal()
		w.win.Invalidate()
		return
	}
	w.place.save(w.hwnd)
	if w.keepRunning && !exit {
		e.PreventDefault()
		w.win.Hide()
		return
	}
	w.client.Cancel()
}

// Minimize implements shell.WindowPort.
func (w *window) Minimize() { w.win.Minimize() }

// ToggleMaximize implements shell.WindowPort.
func (w *window) ToggleMaximize() { w.win.ToggleMaximize() }

// IsMaximized implements shell.WindowPort.
func (w *window) IsMaximized() bool { return w.win != nil && w.win.IsMaximized() }

// Close implements shell.WindowPort.
func (w *window) Close() { w.win.Close() }

// ApproveClose implements shell.WindowPort: the user discarded changes.
func (w *window) ApproveClose() {
	if w.keepRunning && !w.exit.Load() {
		// The window stays in the background: reopening must not show
		// the discarded draft.
		w.shell.Discarded()
	}
	w.approved = true
	w.win.Close()
}

// CancelClose implements shell.WindowPort.
func (w *window) CancelClose() { w.exit.Store(false) }

// ApplySettings implements shell.WindowPort.
func (w *window) ApplySettings(theme string, keepRunning bool) {
	w.keepRunning = keepRunning
	switch theme {
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

var (
	procCallWindowProcW = user32.NewProc("CallWindowProcW")
	subclassed          uintptr
	exitFlag            *atomic.Bool
	subclassProc        = windows.NewCallback(func(hwnd, message, wparam, lparam uintptr) uintptr {
		// The Host's tray exit sends WM_CLOSE with wParam 1: an exit, not a
		// close to the background. MyGo's close event does not carry it.
		if message == 0x0010 && wparam == 1 && exitFlag != nil {
			exitFlag.Store(true)
		}
		result, _, _ := procCallWindowProcW.Call(subclassed, hwnd, message, wparam, lparam)
		return result
	})
)

const gwlpWndProc = ^uintptr(3) // GWLP_WNDPROC (-4)

func subclass(hwnd uintptr, exit *atomic.Bool) {
	exitFlag = exit
	subclassed, _, _ = procSetWindowLongPtrW.Call(hwnd, gwlpWndProc, subclassProc)
}

var (
	procSetFocus        = user32.NewProc("SetFocus")
	procSetActiveWindow = user32.NewProc("SetActiveWindow")
	procIsHungAppWindow = user32.NewProc("IsHungAppWindow")
)

// activate brings the window to the front. The launch that opened it owns
// the foreground right; when Windows still refuses, attach to the
// foreground queue just for the activation, as the previous window did.
func activate(hwnd uintptr) {
	if ok, _, _ := procSetForegroundWindow.Call(hwnd); ok != 0 {
		procSetFocus.Call(hwnd)
		return
	}
	foreground, _, _ := procGetForegroundWindow.Call()
	thread := windows.GetCurrentThreadId()
	other, _, _ := procGetWindowThreadPID.Call(foreground, 0)
	attached := false
	if other != 0 && uint32(other) != thread {
		if hung, _, _ := procIsHungAppWindow.Call(foreground); hung == 0 {
			ok, _, _ := procAttachThreadInput.Call(uintptr(thread), other, 1)
			attached = ok != 0
		}
	}
	procBringWindowToTop.Call(hwnd)
	procSetActiveWindow.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
	procSetFocus.Call(hwnd)
	if attached {
		procAttachThreadInput.Call(uintptr(thread), other, 0)
	}
}

func reportError(err error) {
	if err != nil {
		println("WinOne:", err.Error())
	}
}

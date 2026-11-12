//go:build windows

package app

import (
	"errors"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"

	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/shell"
	"winone/desktopmygo/web"
	internalhost "winone/internal/host"
)

// window owns the settings UI and its lifetime. Closing releases the UI
// process after the unsaved-change/running-work guard; the Host and its
// enabled modules keep their own lifetimes.
type window struct {
	options  Options
	launch   plan
	client   *host.Client
	win      *mygo.Window
	hwnd     uintptr
	shell    *shell.Shell
	instance *instance
	place    *placement
	startup  *startupTrace

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
	w := &window{options: o, launch: launch, client: client, instance: inst, stop: make(chan struct{}), startup: newStartupTrace()}
	w.startup.stage("window-setup")
	env := &core.Env{Host: client, Dialogs: dialogs{w: w}}
	env.Post = func(fn func()) {
		if w.win != nil && !w.win.IsDestroyed() {
			w.win.Update(fn)
		}
	}
	env.WebViews = func(o mygo.WebViewOptions) (*mygo.WebView, error) {
		if w.win == nil || w.win.IsDestroyed() {
			return nil, errors.New(i18n.T("web.unavailable"))
		}
		return w.win.NewWebView(o)
	}
	// The pages of web views reach Go through this scheme, which the
	// window handles only if it exists when the window opens.
	if err := web.Register(); err != nil {
		inst.close()
		return err
	}
	w.startup.instrument(env)
	w.shell = shell.New(env, w, shell.Options{
		Directory: o.Directory,
		EnsureHost: func() (internalhost.Snapshot, error) {
			snapshot, err := ensureHost(client, o.Pipe, o.Directory, launch)
			if w.startup != nil && err == nil {
				w.startup.snapshot.Store(true)
				w.startup.stage("host-snapshot")
			}
			return snapshot, err
		},
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

// settingsWindowOptions keeps the installed window and its native startup
// probe on the same frame, dimensions and production content path.
func settingsWindowOptions(content mygo.Content) mygo.WindowOptions {
	return mygo.WindowOptions{
		Title:             "WinOne",
		Width:             1280,
		Height:            840,
		MinWidth:          800,
		MinHeight:         560,
		TitleBarStyle:     mygo.TitleBarHidden,
		HideWindowButtons: true,
		Hidden:            true,
		BackgroundColor:   "light-dark(#fcfcfc, #191919)",
		Content:           content,
	}
}

func (w *window) open() {
	w.startup.stage("native-window-create")
	w.win = mygo.NewWindow(settingsWindowOptions(ui.View(w.startup.content(w))))
	w.startup.stage("native-window-created")
	// Leave unmatched native drops to MyGo's trusted-page delivery. WebView
	// pages receive the original paths as `mygo:file-drop`; native DropArea
	// targets consume their own drops first in SurfaceEvent.
	w.win.OnFileDrop(func(*mygo.FileDropEvent) {})
	w.hwnd = w.win.NativeHandle()
	if err := w.instance.attach(w.hwnd, w.wake); err != nil {
		reportError(err)
	}
	maximized := w.place.restore(w.hwnd)
	w.win.OnClose(w.onClose)
	// Explorer's commands queue prefills while the window is open: read
	// them as it gains the focus, as the React window did.
	w.win.OnFocus(func() { w.shell.Focused() })
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
				w.win.Update(w.win.Close)
			}
		}()
	})
	w.startup.stage("native-window-shown")
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
	if w.approved {
		w.approved = false
	} else if !w.shell.RequestClose() {
		// The shell asks about the unsaved changes or waits for the work.
		e.PreventDefault()
		w.reveal()
		w.win.Invalidate()
		return
	}
	// The guard has accepted closing. Hide before placement I/O or page
	// teardown so slower WebView disposal cannot leave a frozen UI
	// onscreen. Do not prevent this close: resources and the process still
	// end below, even if a legacy profile has keepUIRunning=true.
	w.win.Hide()
	w.place.save(w.hwnd)
	// The page releases its Host resources while the Host is still reachable.
	w.shell.Discarded()
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
	w.approved = true
	w.win.Close()
}

// CancelClose implements shell.WindowPort. A declined close has changed
// neither visibility nor lifetime, so there is no exit flag to reset.
func (w *window) CancelClose() {}

// ApplySettings implements shell.WindowPort.
func (w *window) ApplySettings(theme string) {
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

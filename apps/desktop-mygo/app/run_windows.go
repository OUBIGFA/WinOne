//go:build windows

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"winone/desktopmygo/host"
	"winone/internal/adminstartup"
	internalhost "winone/internal/host"
	"winone/internal/logintask"
	"winone/internal/maintenance"
	"winone/internal/residency"
	"winone/internal/transport"
)

// plan is how this launch reaches the Host after the administrator check.
type plan struct {
	// standard starts the Host without a remembered administrator start.
	standard bool
	// warning reports why a remembered administrator start was not used.
	warning string
}

// Main runs WinOne.exe with args (without the program name) and returns
// the process exit code.
func Main(args []string) int {
	executable, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	// The scheduled administrator task and its consent prompt start this
	// same program; the shared Go implementation validates and runs them.
	if handled, err := adminstartup.RunAuthorizationHelper(args, executable); handled {
		return exitCode(err)
	}
	if handled, err := adminstartup.RunAuthorizedStartHelper(args, executable); handled {
		return exitCode(err)
	}
	if IsProbe(args) {
		return exitCode(runProbe(args))
	}
	defaultPipe, err := transport.PipeName()
	if err != nil {
		return fail(err)
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return fail(errors.New("无法确定本机配置目录。"))
	}
	options, err := ParseOptions(args, defaultPipe, local)
	if err != nil {
		return fail(err)
	}
	// Match the installer's admission: refuse to start (without a dialog,
	// which would keep the program locked) while maintenance runs.
	lease, err := maintenance.AcquireRuntimeForInstallation(options.Directory, filepath.Dir(executable))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer lease.Close()
	redirected, startErr := adminstartup.Try(context.Background(), options.Directory, options.Pipe, executable, logintask.InteractiveLaunch, options.Standard, options.CaptureDraft)
	launch := plan{standard: options.Standard || startErr != nil}
	if startErr != nil {
		launch.warning = "本次未以管理员身份启动：" + startErr.Error()
	}
	if redirected {
		if startErr != nil {
			return fail(startErr)
		}
		if err := enqueueIntent(options, launch); err != nil {
			return fail(err)
		}
		// A delegated screenshot has its own lifetime; only opening the
		// window passes the foreground right on to the elevated one.
		if options.CaptureDraft == "" {
			return exitCode(activateRedirected(options.Pipe))
		}
		return 0
	}
	if err := enqueueIntent(options, launch); err != nil {
		return fail(err)
	}
	if options.CaptureDraft != "" {
		return exitCode(resumeCapture(options, launch))
	}
	return exitCode(runWindow(options, launch))
}

// enqueueIntent hands Explorer's directory-link or interconnect request to
// the Host before the window shows it.
func enqueueIntent(o Options, launch plan) error {
	if !o.DirectoryLinks && !o.Interconnect {
		return nil
	}
	client, err := host.NewClient(o.Pipe)
	if err != nil {
		return err
	}
	if _, err := ensureHost(client, o.Pipe, o.Directory, launch); err != nil {
		return err
	}
	if o.Interconnect {
		_, err = client.Call("interconnect.open", map[string]any{"paths": o.LinkSources})
	} else {
		_, err = client.Call("directory-links.open", map[string]any{"paths": o.LinkSources, "options": o.LinkOptions})
	}
	return err
}

// resumeCapture runs a screenshot draft's editor through the capture
// module's own window and waits for it, without a settings window.
func resumeCapture(o Options, launch plan) error {
	client, err := host.NewClient(o.Pipe)
	if err != nil {
		return err
	}
	snapshot, err := ensureHost(client, o.Pipe, o.Directory, launch)
	if err != nil {
		return err
	}
	locale := snapshot.Locale
	if locale == "" {
		locale = "zh-CN"
	}
	_, err = client.Job("capture", "draft.resume", map[string]string{"expectedId": o.CaptureDraft, "language": locale}, 0)
	if host.Code(err) == "cancelled" {
		return nil
	}
	return err
}

// ensureHost returns the Host's snapshot, starting the Host next to this
// program when no Host serves the pipe. A Host that exited normally is not
// restarted behind the user's back: the client refuses it.
func ensureHost(client *host.Client, pipe, directory string, launch plan) (internalhost.Snapshot, error) {
	read := func() (internalhost.Snapshot, error) {
		var snapshot internalhost.Snapshot
		err := client.CallInto("snapshot", nil, &snapshot)
		return snapshot, err
	}
	snapshot, initial := read()
	if initial == nil {
		return snapshot, nil
	}
	if client.Cancelled() {
		return snapshot, initial
	}
	if may, err := client.MayStartServer(); err != nil || !may {
		if err != nil {
			return snapshot, err
		}
		return snapshot, initial
	}
	// Another launcher may have created the pipe meanwhile; only a pipe
	// that does not exist starts a Host.
	name, _ := windows.UTF16PtrFromString(pipe)
	if err := waitNamedPipe(name, 100); err == nil || !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return read()
	}
	executable, err := os.Executable()
	if err != nil {
		return snapshot, err
	}
	command := exec.Command(filepath.Join(filepath.Dir(executable), "WinOne.Host.exe"), "--pipe", pipe, "--data-dir", directory)
	if launch.standard {
		command.Args = append(command.Args, "--standard-session")
	}
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := command.Start(); err != nil {
		return snapshot, fmt.Errorf("无法启动后台：%w", err)
	}
	exited := make(chan struct{})
	go func() { _ = command.Wait(); close(exited) }()
	deadline := time.Now().Add(8 * time.Second)
	for {
		snapshot, err = read()
		if err == nil || client.Cancelled() {
			return snapshot, err
		}
		select {
		case <-exited:
			// Another legitimate launcher may have won the pipe; the
			// snapshot above already tried it.
			return snapshot, fmt.Errorf("后台未就绪，请查看诊断日志：%w", err)
		default:
		}
		if time.Now().After(deadline) {
			return snapshot, fmt.Errorf("后台未就绪，请查看诊断日志：%w", err)
		}
		time.Sleep(40 * time.Millisecond)
	}
}

var procWaitNamedPipeW = kernel32.NewProc("WaitNamedPipeW")

func waitNamedPipe(name *uint16, timeout uint32) error {
	if ok, _, err := procWaitNamedPipeW.Call(uintptr(unsafe.Pointer(name)), uintptr(timeout)); ok == 0 {
		return err
	}
	return nil
}

func exitCode(err error) int {
	if err != nil {
		return fail(err)
	}
	return 0
}

var procMessageBoxW = user32.NewProc("MessageBoxW")

// fail reports a startup error the way the previous window did: a message
// box, since a window program has no console.
func fail(err error) int {
	fmt.Fprintln(os.Stderr, err)
	text, _ := windows.UTF16PtrFromString(err.Error())
	title, _ := windows.UTF16PtrFromString("WinOne")
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10) // MB_OK | MB_ICONERROR
	return 1
}

// markPresence records that a settings window is starting for pipe, as the
// Host checks before exiting (residency.WindowOpen).
func markPresence(pipe string) (func(), error) { return residency.MarkWindow(pipe) }

func decodeSnapshot(raw json.RawMessage) (internalhost.Snapshot, error) {
	var snapshot internalhost.Snapshot
	err := json.Unmarshal(raw, &snapshot)
	return snapshot, err
}

//go:build windows

package main

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"winone/internal/host"
	"winone/internal/logintask"
	"winone/internal/maintenance"
	"winone/internal/processidentity"
	"winone/internal/residency"
	"winone/internal/transport"
)

type administratorStartupState struct{ host, administrator, window bool }

func authorizedStartCommand(executable string, mode logintask.LaunchMode) (string, []string, error) {
	if !filepath.IsAbs(executable) || !strings.EqualFold(filepath.Base(executable), "WinOne.exe") {
		return "", nil, errors.New("管理员启动文件无效")
	}
	switch mode {
	case logintask.InteractiveLaunch:
		return executable, []string{"--open-settings"}, nil
	case logintask.LoginLaunch:
		return filepath.Join(filepath.Dir(executable), "WinOne.Host.exe"), []string{"--autorun"}, nil
	default:
		return "", nil, errors.New("不支持的管理员启动方式")
	}
}
func waitAuthorizedStartup(ctx context.Context, probe func(context.Context) (administratorStartupState, error), interval time.Duration) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, err := probe(ctx)
		if err != nil {
			return err
		}
		if (state.host && state.administrator) || (!state.host && !state.window) {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func readAdministratorStartupState(ctx context.Context, pipe, expectedHost string) (administratorStartupState, error) {
	var state administratorStartupState
	var process *processidentity.Process
	defer func() {
		if process != nil {
			process.Close()
		}
	}()
	response, err := transport.CallWithVerifiedServer(ctx, pipe, host.Request{Version: 1, ID: "administrator-start", Method: "snapshot"}, 250*time.Millisecond, func(pid uint32) error {
		var err error
		process, err = processidentity.Open(pid, expectedHost)
		if err != nil {
			return err
		}
		state.administrator = process.Elevated
		return nil
	})
	if err == nil {
		if response.Error != nil {
			return state, errors.New(response.Error.Message)
		}
		state.host = true
	} else if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		state = administratorStartupState{}
	} else if errors.Is(err, processidentity.ErrExited) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, windows.ERROR_PIPE_BUSY) || errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED) {
		// A busy or closing pipe is not proof that the old Host has exited.
		state = administratorStartupState{host: true}
	} else {
		return state, err
	}
	state.window, err = residency.WindowOpen(pipe)
	return state, err
}

func runAuthorizedStartHelper(args []string, executable string) (bool, error) {
	if len(args) == 0 || args[0] != "--authorized-start" {
		return false, nil
	}
	if len(args) != 2 {
		return true, errors.New("管理员启动参数无效")
	}
	if args[1] == string(logintask.RevokeAuthorization) {
		if !windows.GetCurrentProcessToken().IsElevated() {
			return true, errors.New("管理员启动任务未获得授权")
		}
		launcher, err := logintask.NewAdministratorLauncher(executable)
		if err != nil {
			return true, err
		}
		return true, launcher.Remove()
	}
	target, arguments, err := authorizedStartCommand(executable, logintask.LaunchMode(args[1]))
	if err != nil {
		return true, err
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return true, errors.New("管理员启动任务未获得授权")
	}
	pipe, err := transport.PipeName()
	if err != nil {
		return true, err
	}
	lease, err := maintenance.AcquireRuntimeForInstallation(filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne"), filepath.Dir(executable))
	if err != nil {
		return true, err
	}
	defer lease.Close()
	releasePresence, err := residency.MarkAdministratorStart(pipe)
	if err != nil {
		return true, err
	}
	defer releasePresence()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	handoff := &administratorHandoff{pid: uint32(os.Getpid())}
	expectedHost := filepath.Join(filepath.Dir(executable), "WinOne.Host.exe")
	if err = waitAuthorizedStartup(ctx, func(ctx context.Context) (administratorStartupState, error) {
		state, err := readAdministratorStartupState(ctx, pipe, expectedHost)
		if err != nil {
			return state, err
		}
		if state.host && !state.administrator {
			err = handoff.observe(func(method string, params any) (administratorHandoffSnapshot, error) {
				return callAdministratorHandoff(ctx, pipe, expectedHost, method, params)
			})
			if errors.Is(err, processidentity.ErrExited) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED) {
				err = nil
			}
		}
		return state, err
	}, 100*time.Millisecond); err != nil {
		return true, err
	}
	if err = handoff.ready(); err != nil {
		return true, err
	}
	command := exec.Command(target, arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err = command.Start(); err != nil {
		return true, err
	}
	if err = command.Process.Release(); err != nil {
		return true, err
	}
	// Keep the handoff marker alive across the gap between old Host exit and
	// new Host readiness, so a duplicate ordinary launch cannot take its place.
	for {
		state, err := readAdministratorStartupState(ctx, pipe, expectedHost)
		if err != nil {
			return true, err
		}
		if state.host && state.administrator {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

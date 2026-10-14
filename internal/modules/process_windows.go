//go:build windows

// Package modules supervises feature processes without importing their implementations.
package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"winone/internal/diagnostics"
	"winone/internal/host"
	"winone/internal/transport"
)

type Process struct {
	mu        sync.Mutex
	state     host.Module
	pipe      string
	cmd       *exec.Cmd
	pid       uint32
	verified  bool
	exited    chan struct{}
	exitError error
}

// Start waits for a real protocol response before reporting readiness.
func Start(ctx context.Context, id, name, exe, pipe, dataDir string, autorun bool) *Process {
	return start(ctx, id, name, exe, pipe, dataDir, autorun, false)
}

// StartElevated is reserved for the user-requested AlwaysOnTop privilege transition.
func StartElevated(ctx context.Context, id, name, exe, pipe, dataDir string) *Process {
	return start(ctx, id, name, exe, pipe, dataDir, false, true)
}

func start(ctx context.Context, id, name, exe, pipe, dataDir string, autorun, elevated bool, extraArgs ...string) *Process {
	p := &Process{state: host.Module{ID: id, Name: name, State: "starting", Detail: "正在启动"}, pipe: pipe, exited: make(chan struct{})}
	fail := func(err error) *Process {
		p.state.State = "faulted"
		p.state.Detail = err.Error()
		close(p.exited)
		return p
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fail(err)
	}
	log, err := diagnostics.NewModuleWriter(dataDir, id)
	if err != nil {
		return fail(err)
	}
	args := []string{"--pipe", pipe, "--data-dir", dataDir, "--parent", strconv.Itoa(os.Getpid())}
	if autorun {
		args = append(args, "--autorun")
	}
	args = append(args, extraArgs...)
	var wait func() error
	if elevated {
		p.pid, wait, err = elevatedLaunch(exe, args)
		if err != nil {
			log.Close()
			return fail(err)
		}
	} else {
		cmd := exec.Command(exe, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
		cmd.Stderr = log
		cmd.Stdout = log
		if err = cmd.Start(); err != nil {
			log.Close()
			return fail(err)
		}
		p.cmd = cmd
		p.pid = uint32(cmd.Process.Pid)
		wait = cmd.Wait
	}
	go func() {
		err := wait()
		exitError := err
		err = errors.Join(err, log.Close())
		p.mu.Lock()
		defer p.mu.Unlock()
		p.exitError = exitError
		p.state.State = "stopped"
		p.state.Detail = "已停止"
		if err != nil {
			p.state.State = "faulted"
			p.state.Detail = err.Error()
		}
		close(p.exited)
	}()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		probeCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		response, probeErr := transport.Call(probeCtx, pipe, host.Request{Version: 1, ID: "ready", Method: "snapshot"})
		cancel()
		if probeErr == nil && response.Error == nil {
			if err := p.acceptIdentity(response); err != nil {
				p.mu.Lock()
				p.state.State = "faulted"
				p.state.Detail = err.Error()
				p.mu.Unlock()
				return p
			}
			p.mu.Lock()
			if p.state.State == "starting" {
				p.state.State = "ready"
				p.state.Detail = "已连接"
			}
			p.mu.Unlock()
			return p
		}
		select {
		case <-p.exited:
			return p
		case <-ctx.Done():
			p.mu.Lock()
			p.state.State = "faulted"
			p.state.Detail = ctx.Err().Error()
			p.mu.Unlock()
			return p
		case <-deadline.C:
			p.mu.Lock()
			p.state.State = "faulted"
			p.state.Detail = "模块启动超时"
			p.mu.Unlock()
			return p
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (p *Process) Status() host.Module { p.mu.Lock(); defer p.mu.Unlock(); return p.state }

// GrantForeground transfers a user-initiated menu action to the worker's native windows.
func (p *Process) GrantForeground() {
	if p.pid != 0 {
		windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow").Call(uintptr(p.pid))
	}
}

func (p *Process) Handle(request host.Request) host.Response {
	p.mu.Lock()
	verified := p.verified
	pid, detail := p.pid, p.state.Detail
	p.mu.Unlock()
	// No child exists after a failed CreateProcess or a cancelled launch.
	// Probing its pipe would hide the actual startup error with file-not-found.
	if !verified && pid == 0 {
		if detail == "" {
			detail = "组件未启动"
		}
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_unavailable", Message: detail}}
	}
	if !verified {
		probe, err := transport.Call(context.Background(), p.pipe, host.Request{Version: 1, ID: "verify-process", Method: "snapshot"})
		if err == nil && probe.Error != nil {
			err = errors.New(probe.Error.Message)
		}
		if err == nil {
			err = p.acceptIdentity(probe)
		}
		if err != nil {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_identity", Message: err.Error()}}
		}
	}
	response, err := transport.Call(context.Background(), p.pipe, request)
	if err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_unavailable", Message: err.Error()}}
	}
	p.mu.Lock()
	if p.state.State == "faulted" {
		select {
		case <-p.exited:
		default:
			p.state.State = "ready"
			p.state.Detail = "已连接"
		}
	}
	p.mu.Unlock()
	return response
}

func (p *Process) acceptIdentity(response host.Response) error {
	var identity struct {
		PID uint32 `json:"processId"`
	}
	if json.Unmarshal(response.Result, &identity) != nil || identity.PID == 0 {
		return errors.New("组件缺少进程标识，请更新完整组件包")
	}
	if identity.PID != p.pid {
		return errors.New("组件仍由另一会话使用，请先完成该会话的任务")
	}
	p.mu.Lock()
	p.verified = true
	p.mu.Unlock()
	return nil
}

// Close lets the module restore windows and icons before the Host exits.
func (p *Process) Close() error {
	return p.close(false)
}

// CloseConfirmed is for explicit recovery probes. Unlike an ordinary idle
// close, an already-ended process cannot prove that native state was restored.
func (p *Process) CloseConfirmed() error { return p.close(true) }

func (p *Process) close(requireAcknowledgement bool) error {
	select {
	case <-p.exited:
		if requireAcknowledgement {
			return errors.New("模块在确认资源恢复前已退出")
		}
		return nil
	default:
	}
	response := p.Handle(host.Request{Version: 1, ID: "shutdown", Method: "shutdown"})
	if response.Error != nil {
		if requireAcknowledgement {
			return errors.New(response.Error.Message)
		}
		select {
		case <-p.exited:
			return nil
		default:
		}
		return errors.New(response.Error.Message)
	}
	select {
	case <-p.exited:
		p.mu.Lock()
		exitError := p.exitError
		p.mu.Unlock()
		if requireAcknowledgement && exitError != nil {
			return fmt.Errorf("模块确认后异常退出，无法确认恢复完成：%w", exitError)
		}
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("%s 尚未完成恢复，请重试退出", p.Status().Name)
	}
}

//go:build windows

// Package modules supervises feature processes without importing their implementations.
package modules

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"winone/internal/host"
	"winone/internal/transport"
)

type Process struct {
	mu     sync.Mutex
	state  host.Module
	pipe   string
	cmd    *exec.Cmd
	exited chan struct{}
}

// Start waits for a real protocol response before reporting readiness.
func Start(ctx context.Context, id, name, exe, pipe, dataDir string, autorun bool) *Process {
	p := &Process{state: host.Module{ID: id, Name: name, State: "starting", Detail: "正在启动"}, pipe: pipe, exited: make(chan struct{})}
	fail := func(err error) *Process {
		p.state.State = "faulted"
		p.state.Detail = err.Error()
		close(p.exited)
		return p
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fail(err)
	}
	log, err := os.OpenFile(filepath.Join(dataDir, "worker.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fail(err)
	}
	args := []string{"--pipe", pipe, "--data-dir", dataDir, "--parent", strconv.Itoa(os.Getpid())}
	if autorun {
		args = append(args, "--autorun")
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	cmd.Stderr = log
	cmd.Stdout = log
	if err = cmd.Start(); err != nil {
		log.Close()
		return fail(err)
	}
	p.cmd = cmd
	go func() {
		err := cmd.Wait()
		log.Close()
		p.mu.Lock()
		defer p.mu.Unlock()
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
	if p.cmd != nil && p.cmd.Process != nil {
		windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow").Call(uintptr(p.cmd.Process.Pid))
	}
}

func (p *Process) Handle(request host.Request) host.Response {
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

// Close lets the module restore windows and icons before the Host exits.
func (p *Process) Close() error {
	select {
	case <-p.exited:
		return nil
	default:
	}
	response := p.Handle(host.Request{Version: 1, ID: "shutdown", Method: "shutdown"})
	if response.Error != nil {
		select {
		case <-p.exited:
			return nil
		default:
		}
		return errors.New(response.Error.Message)
	}
	select {
	case <-p.exited:
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("%s 尚未完成恢复，请重试退出", p.Status().Name)
	}
}

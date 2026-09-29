//go:build windows

package modules

import (
	"context"
	"os"
	"sync"
	"winone/internal/host"
)

// Lazy starts an on-demand worker only for an explicit command, not for status display.
type Lazy struct {
	mu                        sync.Mutex
	ctx                       context.Context
	id, name, exe, pipe, data string
	process                   *Process
	closing                   bool
}

func NewLazy(ctx context.Context, id, name, exe, pipe, data string) *Lazy {
	return &Lazy{ctx: ctx, id: id, name: name, exe: exe, pipe: pipe, data: data}
}
func (l *Lazy) Status() host.Module {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.process != nil {
		s := l.process.Status()
		if s.State != "stopped" {
			return s
		}
	}
	if _, err := os.Stat(l.exe); err != nil {
		return host.Module{ID: l.id, Name: l.name, State: "unavailable", Detail: "组件尚未安装"}
	}
	return host.Module{ID: l.id, Name: l.name, State: "available", Detail: "可用，按需启动"}
}
func (l *Lazy) Handle(request host.Request) host.Response {
	l.mu.Lock()
	if l.closing {
		l.mu.Unlock()
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "stopping", Message: "模块正在退出"}}
	}
	exited := l.process == nil
	if l.process != nil {
		select {
		case <-l.process.exited:
			exited = true
		default:
		}
	}
	if exited {
		if request.Method == "jobs.get" {
			l.mu.Unlock()
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "stopped", Message: "模块已退出，任务状态不可用"}}
		}
		l.process = Start(l.ctx, l.id, l.name, l.exe, l.pipe, l.data, false)
	}
	p := l.process
	l.mu.Unlock()
	p.GrantForeground()
	return p.Handle(request)
}
func (l *Lazy) Close() error {
	l.mu.Lock()
	l.closing = true
	p := l.process
	l.mu.Unlock()
	if p == nil {
		return nil
	}
	err := p.Close()
	if err != nil {
		l.mu.Lock()
		l.closing = false
		l.mu.Unlock()
	}
	return err
}

// Resume permits on-demand restart when another module prevents the overall application exit.
func (l *Lazy) Resume() { l.mu.Lock(); l.closing = false; l.mu.Unlock() }

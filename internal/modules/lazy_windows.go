//go:build windows

package modules

import (
	"context"
	"encoding/json"
	"errors"
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
	// starting is closed when a worker launch, which waits up to eight
	// seconds for the worker, ends. The launch runs without mu, so Status
	// (the Host's snapshot reads every module's) answers meanwhile; while
	// it is set no other caller launches or replaces the process.
	starting    chan struct{}
	everStarted bool
	closing     bool
	inflight    sync.WaitGroup
	privilege   PrivilegeState
	autorun     bool
	launchArgs  []string
}

func NewLogonLazy(ctx context.Context, id, name, exe, pipe, data string, autorun bool) *Lazy {
	l := NewLazy(ctx, id, name, exe, pipe, data)
	l.autorun = autorun
	return l
}

func NewLazy(ctx context.Context, id, name, exe, pipe, data string, launchArgs ...string) *Lazy {
	return &Lazy{ctx: ctx, id: id, name: name, exe: exe, pipe: pipe, data: data, launchArgs: append([]string(nil), launchArgs...)}
}
func (l *Lazy) Status() host.Module {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.starting != nil {
		return host.Module{ID: l.id, Name: l.name, State: "starting", Detail: "正在启动"}
	}
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
	if l.id == "always-on-top" && (request.Method == "privilege.start" || request.Method == "privilege.status") {
		return l.handlePrivilege(request)
	}
	l.mu.Lock()
	// A launch in progress serves this request too, once it ends.
	waitedForStart := false
	for l.starting != nil {
		waitedForStart = true
		wait := l.starting
		l.mu.Unlock()
		<-wait
		l.mu.Lock()
	}
	if waitedForStart && l.process != nil {
		if status := l.process.Status(); status.State != "ready" {
			l.mu.Unlock()
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_unavailable", Message: status.Detail}}
		}
	}
	if l.closing || l.privilege.State == "running" {
		l.mu.Unlock()
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "stopping", Message: "模块正在退出"}}
	}
	// Close and privilege changes wait for in-flight work, the launch included.
	l.inflight.Add(1)
	defer l.inflight.Done()
	exited := l.process == nil
	if l.process != nil {
		select {
		case <-l.process.exited:
			exited = true
		default:
		}
	}
	if exited {
		if request.Method == "capture.activate" {
			l.mu.Unlock()
			return host.Response{ID: request.ID, Result: json.RawMessage(`{"active":false}`)}
		}
		if request.Method == "jobs.get" {
			l.mu.Unlock()
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "stopped", Message: "模块已退出，任务状态不可用"}}
		}
		autorun, args := l.autorun, l.launchArgs
		if l.id == "wintray" && (request.Method == "deactivate" || request.Method == "uninstall.prepare") {
			autorun, args = false, []string{"--recover-only"}
		}
		l.autorun = false
		done := make(chan struct{})
		l.starting = done
		l.mu.Unlock()
		started := start(l.ctx, l.id, l.name, l.exe, l.pipe, l.data, autorun, false, args...)
		l.mu.Lock()
		l.process, l.starting = started, nil
		close(done)
		// Creation is immutable once start returns. Keep the history even if a
		// later launch fails; absence of a current process is not cleanup proof.
		if started.pid != 0 {
			l.everStarted = true
		}
		// A failed launch must fail this command. Retrying identity immediately
		// used to execute the original shortcut after its startup had timed out.
		if status := started.Status(); status.State != "ready" {
			l.mu.Unlock()
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_unavailable", Message: status.Detail}}
		}
	}
	p := l.process
	l.mu.Unlock()
	p.GrantForeground()
	return p.Handle(request)
}
func (l *Lazy) Close() error {
	return l.close(false)
}

func (l *Lazy) CloseConfirmed() error { return l.close(true) }

// HasStarted reports whether any worker process was created by this instance.
// Coordinators may distinguish a rejected executable from an unconfirmed exit.
func (l *Lazy) HasStarted() bool { l.mu.Lock(); defer l.mu.Unlock(); return l.everStarted }

func (l *Lazy) close(requireAcknowledgement bool) error {
	l.mu.Lock()
	if l.privilege.State == "running" {
		l.mu.Unlock()
		return errors.New("请先完成或取消管理员授权")
	}
	l.closing = true
	l.mu.Unlock()
	// In-flight requests, a launch among them, finish before the process is
	// read: the launch may only now have set it.
	l.inflight.Wait()
	l.mu.Lock()
	p := l.process
	l.mu.Unlock()
	if p == nil {
		if requireAcknowledgement {
			return errors.New("模块未启动，无法确认资源恢复")
		}
		return nil
	}
	var err error
	if requireAcknowledgement {
		err = p.CloseConfirmed()
	} else {
		err = p.Close()
	}
	if err != nil {
		l.mu.Lock()
		l.closing = false
		l.mu.Unlock()
	}
	return err
}

// Resume permits on-demand restart when another module prevents the overall application exit.
func (l *Lazy) Resume() { l.mu.Lock(); l.closing = false; l.mu.Unlock() }

// SuppressAutorun prevents a later settings/history operation from replaying a
// deferred login activation. User actions are not another logon event.
func (l *Lazy) SuppressAutorun() { l.mu.Lock(); l.autorun = false; l.mu.Unlock() }

// Running allows residency checks without starting an idle feature process.
func (l *Lazy) Running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.privilege.State == "running" || l.starting != nil {
		return true
	}
	if l.process == nil {
		return false
	}
	select {
	case <-l.process.exited:
		return false
	default:
		return true
	}
}

// ExitSignal observes the current process without starting it or polling the
// operating system. A nil channel means no process has been created yet.
func (l *Lazy) ExitSignal() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.process == nil {
		return nil
	}
	return l.process.exited
}

// HandleRunning dispatches only to an existing worker. Configuration refreshes
// must not restart an idle process or race privilege transitions into a new one.
func (l *Lazy) HandleRunning(request host.Request) (host.Response, bool) {
	l.mu.Lock()
	if l.privilege.State == "running" {
		l.mu.Unlock()
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "busy", Message: "模块状态正在切换"}}, true
	}
	if l.process == nil {
		l.mu.Unlock()
		return host.Response{}, false
	}
	select {
	case <-l.process.exited:
		l.mu.Unlock()
		return host.Response{}, false
	default:
	}
	if l.closing {
		l.mu.Unlock()
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "busy", Message: "模块正在退出"}}, true
	}
	p := l.process
	l.inflight.Add(1)
	l.mu.Unlock()
	defer l.inflight.Done()
	return p.Handle(request), true
}

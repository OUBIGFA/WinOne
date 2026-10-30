// Package host is the desktop window's client of the WinOne Host: the same
// named-pipe protocol, method names, time budgets and job semantics the
// previous Rust window (apps/desktop-rust/src/transport.rs) used, so every
// page sends exactly the requests its React version did.
package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
	internalhost "winone/internal/host"
	"winone/internal/transport"
)

// Error is a failure the Host or this client reports. Pages show Error()
// as the React window showed String(error): the message, then the code.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message + " (" + e.Code + ")" }

func newError(code, message string) *Error { return &Error{Code: code, Message: message} }

// Code returns the Host error code of err, or "".
func Code(err error) string {
	var value *Error
	if errors.As(err, &value) {
		return value.Code
	}
	return ""
}

// Client sends requests to one Host pipe. It is safe for concurrent use.
type Client struct {
	pipe     string
	sequence atomic.Uint64
	ctx      context.Context
	cancel   context.CancelFunc

	mu     sync.Mutex
	server *ServerProcess
	// bound is told about each newly bound Host process, so the window can
	// close when that Host exits normally (tray exit, maintenance).
	bound func(*ServerProcess)
}

// NewClient validates the pipe the same way the launcher does: only a
// local WinOne pipe is accepted.
func NewClient(pipe string) (*Client, error) {
	if !strings.HasPrefix(pipe, `\\.\pipe\WinOne-`) || strings.ContainsRune(pipe, 0) || len(pipe) > 256 {
		return nil, newError("invalid", "仅支持本机 WinOne 管道")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{pipe: pipe, ctx: ctx, cancel: cancel}, nil
}

// Pipe returns the Host pipe name.
func (c *Client) Pipe() string { return c.pipe }

// Cancel stops every request in flight and refuses new ones, as the window
// closes.
func (c *Client) Cancel() { c.cancel() }

// Cancelled reports whether Cancel was called.
func (c *Client) Cancelled() bool { return c.ctx.Err() != nil }

// OnServer registers fn for each Host process the client binds to.
func (c *Client) OnServer(fn func(*ServerProcess)) {
	c.mu.Lock()
	c.bound = fn
	server := c.server
	c.mu.Unlock()
	if fn != nil && server != nil {
		fn(server)
	}
}

func (c *Client) nextID() string {
	return fmt.Sprintf("mygo-%d-%d", os.Getpid(), c.sequence.Add(1))
}

// callBudget is the Rust client's Client::call budget, by method.
func callBudget(method string) time.Duration {
	switch {
	case method == "chat.export" || method == "capture.ai.archive.export.save" || method == "capture.ai.archive.read":
		return 150 * time.Second
	case strings.HasPrefix(method, "chat.") || method == "capture.ai.usage.export.save":
		return 30 * time.Second
	case strings.HasPrefix(method, "capture.") || strings.HasPrefix(method, "always-on-top."):
		return 15 * time.Second
	}
	return 5 * time.Second
}

// Call sends one request and returns its result. Mutations are never
// retried automatically.
func (c *Client) Call(method string, params any) (json.RawMessage, error) {
	return c.request(c.nextID(), method, params, time.Now().Add(callBudget(method)))
}

// CallInto is Call decoding the result into out.
func (c *Client) CallInto(method string, params, out any) error {
	return decodeInto(c.Call(method, params))(out)
}

func decodeInto(raw json.RawMessage, err error) func(any) error {
	return func(out any) error {
		if err != nil {
			return err
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return newError("protocol", err.Error())
		}
		return nil
	}
}

// CallCompleting submits a mutation once, then follows the configuration
// job the Host may return for it until the job finishes, within the same
// budgets as the Rust client's call_completing.
func (c *Client) CallCompleting(method string, params any) (json.RawMessage, error) {
	id := c.nextID()
	budget, firstWait := 30*time.Second, 15*time.Second
	switch method {
	case "capture.ai.archive.export.save", "capture.ai.archive.read":
		budget, firstWait = 150*time.Second, 150*time.Second
	case "capture.ai.models.test":
		budget, firstWait = 100*time.Second, 100*time.Second
	case "capture.ai.usage.export.save", "capture.ai.magpie.catalog", "capture.ai.accounts.catalog",
		"capture.ai.accounts.list", "capture.ai.accounts.models", "capture.ai.accounts.quota", "capture.ai.accounts.checkin":
		firstWait = 30 * time.Second
	}
	deadline := time.Now().Add(budget)
	first, err := c.request(id, method, params, minTime(deadline, time.Now().Add(firstWait)))
	if err != nil {
		return nil, err
	}
	var ticket struct {
		ID string `json:"configurationJob"`
	}
	if json.Unmarshal(first, &ticket) != nil || ticket.ID == "" {
		return first, nil
	}
	for {
		if err := c.check(deadline); err != nil {
			return nil, err
		}
		raw, err := c.request(id+"-configuration", "configuration.jobs.get", map[string]string{"id": ticket.ID}, deadline)
		if err != nil {
			return nil, err
		}
		var job struct {
			ID       string                 `json:"id"`
			State    string                 `json:"state"`
			Response *internalhost.Response `json:"response"`
		}
		if err := json.Unmarshal(raw, &job); err != nil {
			return nil, newError("protocol", err.Error())
		}
		if job.ID != ticket.ID {
			return nil, newError("protocol", "配置任务标识不匹配")
		}
		switch job.State {
		case "running":
			if err := c.sleep(150*time.Millisecond, deadline); err != nil {
				return nil, err
			}
		case "completed", "failed":
			if job.Response == nil {
				return nil, newError("protocol", "配置任务缺少结果")
			}
			if (job.State == "failed") != (job.Response.Error != nil) {
				return nil, newError("protocol", "配置任务状态与结果不一致")
			}
			if job.Response.ID != id {
				return nil, newError("protocol", "后台响应标识不匹配")
			}
			if job.Response.Error != nil {
				return nil, &Error{Code: job.Response.Error.Code, Message: job.Response.Error.Message}
			}
			return job.Response.Result, nil
		default:
			return nil, newError("protocol", "无法确认配置任务状态")
		}
	}
}

// CallCompletingInto is CallCompleting decoding the result into out.
func (c *Client) CallCompletingInto(method string, params, out any) error {
	return decodeInto(c.CallCompleting(method, params))(out)
}

// Job runs a module job: the first call starts it, then <namespace>.jobs.get
// follows it until it completes, fails or is cancelled. limit zero waits as
// long as the window lives (an interactive job, as a screenshot editor).
func (c *Client) Job(namespace, method string, params any, limit time.Duration) (json.RawMessage, error) {
	var deadline time.Time
	if limit > 0 {
		deadline = time.Now().Add(limit)
	}
	raw, err := c.Call(namespace+"."+method, params)
	if err != nil {
		return nil, err
	}
	expected := ""
	for {
		var job struct {
			ID     string          `json:"id"`
			State  string          `json:"state"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(raw, &job); err != nil {
			return nil, newError("protocol", err.Error())
		}
		if expected != "" && job.ID != expected {
			return nil, newError("protocol", "任务标识不匹配")
		}
		switch job.State {
		case "completed":
			return job.Result, nil
		case "cancelled":
			return nil, newError("cancelled", "操作已取消")
		case "failed":
			message := job.Error
			if message == "" {
				message = "操作失败"
			}
			return nil, newError("operation_failed", message)
		case "running":
			if job.ID == "" {
				return nil, newError("protocol", "任务缺少标识")
			}
			expected = job.ID
		default:
			return nil, newError("protocol", "无法确认任务状态")
		}
		if err := c.sleep(250*time.Millisecond, deadline); err != nil {
			return nil, err
		}
		if raw, err = c.Call(namespace+".jobs.get", map[string]string{"id": expected}); err != nil {
			return nil, err
		}
	}
}

func (c *Client) sleep(d time.Duration, deadline time.Time) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-c.ctx.Done():
		return newError("cancelled", "操作已取消")
	case <-timer.C:
	}
	if !deadline.IsZero() {
		return c.check(deadline)
	}
	return nil
}

func (c *Client) check(deadline time.Time) error {
	if c.Cancelled() {
		return newError("cancelled", "操作已取消")
	}
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return newError("timeout", "后台响应超时，请检查后重试")
	}
	return nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (c *Client) request(id, method string, params any, deadline time.Time) (json.RawMessage, error) {
	if err := c.check(deadline); err != nil {
		return nil, err
	}
	var data json.RawMessage
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			return nil, newError("protocol", err.Error())
		}
		data = encoded
	}
	response, err := transport.CallWithVerifiedServer(c.ctx, c.pipe, internalhost.Request{Version: 1, ID: id, Method: method, Params: data}, time.Until(deadline), c.bind)
	if err != nil {
		var rpc *Error
		if errors.As(err, &rpc) {
			return nil, rpc
		}
		if c.Cancelled() {
			return nil, newError("cancelled", "操作已取消")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, newError("timeout", "后台请求已取消或超时")
		}
		return nil, newError("transport", "无法连接后台："+err.Error())
	}
	if response.Error != nil {
		return nil, &Error{Code: response.Error.Code, Message: response.Error.Message}
	}
	if response.Result == nil {
		return nil, newError("protocol", "后台响应缺少结果或包含冲突状态")
	}
	return response.Result, nil
}

// ServerProcess is the Host process a connection reached. A Host that
// exited normally is never silently replaced: requests then fail and the
// window closes. A crashed Host (non-zero exit) may be started again.
type ServerProcess struct {
	pid    uint32
	handle windows.Handle
}

// PID returns the Host process identifier.
func (s *ServerProcess) PID() uint32 { return s.pid }

// exit reports whether the process ended, and its exit code.
func (s *ServerProcess) exit() (bool, uint32, error) {
	event, err := windows.WaitForSingleObject(s.handle, 0)
	if err != nil {
		return false, 0, err
	}
	if event == uint32(windows.WAIT_TIMEOUT) {
		return false, 0, nil
	}
	var code uint32
	if err := windows.GetExitCodeProcess(s.handle, &code); err != nil {
		return true, 0, err
	}
	return true, code, nil
}

// Wait blocks until the Host exits or stop is closed. It returns the exit
// code and whether the process actually ended.
func (s *ServerProcess) Wait(stop <-chan struct{}) (uint32, bool) {
	done := make(chan struct{})
	go func() {
		select {
		case <-stop:
		case <-done:
		}
	}()
	defer close(done)
	for {
		event, err := windows.WaitForSingleObject(s.handle, 250)
		if err != nil {
			return 0, false
		}
		if event == windows.WAIT_OBJECT_0 {
			var code uint32
			if windows.GetExitCodeProcess(s.handle, &code) != nil {
				return 0, false
			}
			return code, true
		}
		select {
		case <-stop:
			return 0, false
		default:
		}
	}
}

// MayStartServer reports whether ensuring the Host may start a new
// process: none was bound yet, or the bound one crashed.
func (c *Client) MayStartServer() (bool, error) {
	c.mu.Lock()
	server := c.server
	c.mu.Unlock()
	if server == nil {
		return true, nil
	}
	ended, code, err := server.exit()
	if err != nil {
		return false, err
	}
	return ended && code != 0, nil
}

// Server returns the bound Host process, if any.
func (c *Client) Server() *ServerProcess {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.server
}

func (c *Client) bind(pid uint32) error {
	c.mu.Lock()
	if current := c.server; current != nil {
		ended, code, err := current.exit()
		if err != nil {
			c.mu.Unlock()
			return err
		}
		if !ended && current.pid == pid {
			c.mu.Unlock()
			return nil
		}
		if !(ended && code != 0) {
			c.mu.Unlock()
			return newError("connection", "后台已退出，请重新打开 WinOne")
		}
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	// A replaced (crashed) Host keeps its handle: a watcher may still hold
	// it, and the process lives as long as the window, so the leak is one
	// handle per Host crash at most.
	server := &ServerProcess{pid: pid, handle: handle}
	c.server = server
	notify := c.bound
	c.mu.Unlock()
	if notify != nil {
		notify(server)
	}
	return nil
}

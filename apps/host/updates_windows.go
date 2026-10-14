//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"winone/internal/configjson"
	"winone/internal/host"
	"winone/internal/processidentity"
	"winone/internal/updates"
)

// Only the build specifies the publisher. Neither RPC input nor imported
// settings can redirect update checks to another repository.
var updateRepository string
var updateVersion = "development"

type updateBuildInfo struct {
	Version    string `json:"version"`
	Configured bool   `json:"configured"`
}
type updateOwner interface {
	Wait(context.Context) error
	Close()
}
type updateControlSnapshot struct {
	ID string `json:"id"`
	updates.Reply
}
type updateCheck struct {
	id                 string
	owner              uint32
	cancel             context.CancelFunc
	done               chan struct{}
	running, cancelled bool
	result             updates.Reply
}
type updateControl struct {
	ctx     context.Context
	info    updateBuildInfo
	run     func(context.Context) (updates.Result, error)
	observe func(uint32) (updateOwner, error)
	session updates.Session
	mu      sync.Mutex
	current *updateCheck
	closed  bool
}

func newUpdateControl(ctx context.Context, info updateBuildInfo, run func(context.Context) (updates.Result, error), observe func(uint32) (updateOwner, error)) *updateControl {
	return &updateControl{ctx: ctx, info: info, run: run, observe: observe}
}
func registerUpdates(service *host.Service, ctx context.Context, executable string) func() {
	checker := updates.Checker{Repository: updateRepository, CurrentVersion: updateVersion}
	control := newUpdateRPC(ctx, executable, checker)
	service.RegisterModule("updates", func() host.Module { return host.Module{ID: "updates", State: "ready"} }, control.handle)
	return control.close
}
func newUpdateRPC(ctx context.Context, executable string, checker updates.Checker) *updateControl {
	return newUpdateControl(ctx, updateBuildInfo{Version: checker.CurrentVersion, Configured: checker.Repository != ""}, checker.Check, func(pid uint32) (updateOwner, error) {
		return processidentity.Open(pid, filepath.Join(filepath.Dir(executable), "WinOne.exe"))
	})
}
func cancelledUpdate() updates.Reply {
	return updates.Reply{Result: updates.Result{State: "failed"}, ErrorCode: "cancelled"}
}
func (c *updateControl) snapshotLocked(job *updateCheck) updateControlSnapshot {
	result := job.result
	if job.running {
		result = updates.Reply{Result: updates.Result{State: "checking", CurrentVersion: c.info.Version}}
	}
	return updateControlSnapshot{ID: job.id, Reply: result}
}
func (c *updateControl) start(pid uint32, id string) (updateControlSnapshot, error) {
	c.mu.Lock()
	if c.current != nil && c.current.id == id && c.current.owner == pid {
		result := c.snapshotLocked(c.current)
		c.mu.Unlock()
		return result, nil
	}
	unavailable := c.closed || c.ctx.Err() != nil || c.current != nil && c.current.running
	c.mu.Unlock()
	if unavailable {
		return updateControlSnapshot{}, errors.New("更新检查正在进行或已关闭")
	}
	owner, err := c.observe(pid)
	if err != nil {
		return updateControlSnapshot{}, err
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.mu.Lock()
	// Identity inspection runs outside the lock. A matching request may have
	// started and even completed meanwhile; reuse it rather than issuing HTTP twice.
	if c.current != nil && c.current.id == id && c.current.owner == pid {
		result := c.snapshotLocked(c.current)
		c.mu.Unlock()
		cancel()
		owner.Close()
		return result, nil
	}
	if c.closed || ctx.Err() != nil || c.current != nil && c.current.running {
		c.mu.Unlock()
		cancel()
		owner.Close()
		return updateControlSnapshot{}, errors.New("更新检查正在进行或已关闭")
	}
	job := &updateCheck{id: id, owner: pid, cancel: cancel, done: make(chan struct{}), running: true}
	c.current = job
	result := c.snapshotLocked(job)
	c.mu.Unlock()
	go c.check(ctx, job, owner)
	return result, nil
}
func (c *updateControl) check(ctx context.Context, job *updateCheck, owner updateOwner) {
	defer close(job.done)
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		defer owner.Close()
		_ = owner.Wait(ctx)
		if ctx.Err() == nil {
			c.mu.Lock()
			job.cancelled = true
			c.mu.Unlock()
			job.cancel()
		}
	}()
	reply := c.session.Check(ctx, job.id, c.run)
	job.cancel()
	<-watched
	// A cancelled response cannot restore a stale release link, even when the
	// network completed between the cancel request and storing its result.
	c.mu.Lock()
	defer c.mu.Unlock()
	if job.cancelled {
		reply = cancelledUpdate()
	}
	job.result = reply
	job.running = false
}
func (c *updateControl) close() {
	c.mu.Lock()
	c.closed = true
	job := c.current
	if job != nil {
		job.cancelled = true
		job.cancel()
		if !job.running {
			job.result = cancelledUpdate()
		}
	}
	c.mu.Unlock()
	if job != nil {
		<-job.done
	}
}
func (c *updateControl) handle(request host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "updates", Message: err.Error()}}
	}
	respond := func(value any) host.Response {
		raw, err := json.Marshal(value)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: raw}
	}
	if request.Method == "info" {
		return respond(c.info)
	}
	var params struct {
		ID string `json:"id"`
	}
	if err := configjson.DecodeObject(request.Params, &params); err != nil {
		return fail(err)
	}
	if params.ID == "" || len(params.ID) > 128 || strings.ContainsAny(params.ID, "\x00\r\n") || request.PeerPID == 0 {
		return fail(errors.New("更新检查标识无效"))
	}
	if request.Method == "check" {
		result, err := c.start(request.PeerPID, params.ID)
		if err != nil {
			return fail(err)
		}
		return respond(result)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	job := c.current
	if job == nil || job.id != params.ID || job.owner != request.PeerPID {
		return fail(errors.New("更新检查已变化，请重新检查"))
	}
	switch request.Method {
	case "status":
		return respond(c.snapshotLocked(job))
	case "cancel":
		job.cancelled = true
		job.cancel()
		if !job.running {
			job.result = cancelledUpdate()
		}
		return respond(c.snapshotLocked(job))
	case "release":
		if job.running || job.cancelled || job.result.State != "available" || job.result.ReleaseURL == "" {
			return fail(errors.New("没有已确认的更新发布页"))
		}
		return respond(map[string]string{"url": job.result.ReleaseURL})
	default:
		return fail(errors.New("不支持的更新操作"))
	}
}

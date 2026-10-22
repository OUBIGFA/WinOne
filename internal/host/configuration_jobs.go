package host

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ConfigurationJob struct {
	ID       string    `json:"id"`
	State    string    `json:"state"`
	Response *Response `json:"response,omitempty"`
}

// A stuck volume must not accumulate unlimited goroutines or completion data.
const maxBackgroundJobs = 16

// ConfigurationQueue keeps slow work out of short pipe calls. Settings changes
// are exclusive; ordinary filesystem jobs share action admission with Chat and
// other modules. A slow disk must never put the entire Host into maintenance.
type ConfigurationQueue struct {
	mu         sync.Mutex
	actions    sync.RWMutex
	next       interface{ Handle(Request) Response }
	active     bool
	background int
	jobs       map[string]ConfigurationJob
	completed  []string
	sequence   uint64
	session    string
}

func NewConfigurationQueue(next interface{ Handle(Request) Response }) *ConfigurationQueue {
	return &ConfigurationQueue{next: next, jobs: map[string]ConfigurationJob{}, session: fmt.Sprint(time.Now().UnixNano())}
}
func (q *ConfigurationQueue) Busy() bool { q.mu.Lock(); defer q.mu.Unlock(); return q.active }

// AcquireMaintenance atomically closes mutation admission for shutdown or
// installation maintenance. It never waits on an admitted UI action: callers
// report busy and retry after that action finishes. Successful shutdown retains
// the lease until process exit; failed preparation must release it.
func (q *ConfigurationQueue) AcquireMaintenance() (func(), bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active || !q.actions.TryLock() {
		return nil, false
	}
	q.active = true
	var once sync.Once
	return func() {
		once.Do(func() {
			q.mu.Lock()
			defer q.mu.Unlock()
			q.actions.Unlock()
			q.active = false
		})
	}, true
}

// AcquireAction lets admitted commands finish before a transaction and rejects
// new actions while settings change. Status reads do not need a lease.
func (q *ConfigurationQueue) AcquireAction() (func(), bool) {
	if q.Busy() {
		return nil, false
	}
	// Maintenance may start after the first Busy check and retain its lock
	// until exit. Do not leave a late pipe call waiting behind that lock.
	if !q.actions.TryRLock() {
		return nil, false
	}
	if q.Busy() {
		q.actions.RUnlock()
		return nil, false
	}
	return q.actions.RUnlock, true
}

func (q *ConfigurationQueue) Handle(request Request) Response {
	fail := func(code, message string) Response {
		return Response{ID: request.ID, Error: &RPCError{Code: code, Message: message}}
	}
	if request.Version != 1 || request.ID == "" {
		return fail("invalid", "无效的配置请求")
	}
	if request.Method == "configuration.jobs.get" {
		var args struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(request.Params, &args) != nil || args.ID == "" {
			return fail("invalid", "任务标识无效")
		}
		q.mu.Lock()
		job, exists := q.jobs[args.ID]
		q.mu.Unlock()
		if !exists {
			return fail("not_found", "配置任务不存在或已过期")
		}
		data, err := json.Marshal(job)
		if err != nil {
			return fail("internal", err.Error())
		}
		return Response{ID: request.ID, Result: data}
	}
	// These operations change this tool's own history/files, not the Host's
	// configuration. Keep their ticket protocol, but never take the global
	// write lock (AI streams can hold an action lease for minutes).
	background := false
	switch request.Method {
	case "directory-links.browse", "directory-links.list", "directory-links.preview", "directory-links.previewBatch", "directory-links.create", "directory-links.create.elevated", "directory-links.createBatch", "directory-links.createBatch.elevated", "directory-links.rename", "directory-links.inspect", "directory-links.inspectBatch", "directory-links.scan", "directory-links.remove", "directory-links.forget", "directory-links.forgetInvalid", "directory-links.exportPlan", "directory-links.reveal", "directory-links.menu.set":
		background = true
	case "settings.save", "capture.settings.save", "always-on-top.settings.save", "wintray.settings.save", "selection.settings.save", "quick-assistant.settings.save", "stroke.settings.save", "modules.setEnabled", "modules.recoverConfiguration", "modules.recoverPackage", "modules.installPackage", "modules.updatePackage", "modules.removePackage", "modules.cleanupPackages", "modules.resetWinTray", "profiles.import.apply", "packages.settings.save":
	default:
		return q.next.Handle(request)
	}
	q.mu.Lock()
	if q.active {
		q.mu.Unlock()
		return fail("configuration_busy", "配置正在保存，请等待完成")
	}
	if background {
		// Bound in-flight filesystem requests, including abandoned UI polls.
		// Cancellation/status remain direct calls and cannot be queued behind I/O.
		if q.background >= maxBackgroundJobs || !q.actions.TryRLock() {
			q.mu.Unlock()
			return fail("action_busy", "文件操作正在进行，请等待完成后重试")
		}
		q.background++
	} else {
		// Do not announce maintenance while waiting for a long AI request. A
		// pending RWMutex writer used to reject every later module action, even
		// though no configuration had changed. Refuse the save without a ticket
		// or side effect; existing work and unrelated requests remain usable.
		if !q.actions.TryLock() {
			q.mu.Unlock()
			return fail("configuration_busy", "操作正在进行，暂时无法保存配置；请等待完成后重试")
		}
		q.active = true
	}
	q.sequence++
	id := fmt.Sprintf("%s-%d", q.session, q.sequence)
	q.jobs[id] = ConfigurationJob{ID: id, State: "running"}
	q.mu.Unlock()
	// Copy caller-owned bytes before returning to the pipe handler.
	request.Params = append(json.RawMessage(nil), request.Params...)
	go func() {
		response := q.next.Handle(request)
		job := ConfigurationJob{ID: id, State: "completed", Response: &response}
		if response.Error != nil {
			job.State = "failed"
		}
		q.mu.Lock()
		defer q.mu.Unlock()
		q.jobs[id] = job
		q.completed = append(q.completed, id)
		for len(q.completed) > 32 {
			delete(q.jobs, q.completed[0])
			q.completed = q.completed[1:]
		}
		if background {
			q.background--
			q.actions.RUnlock()
		} else {
			q.actions.Unlock()
			q.active = false
		}
	}()
	data, _ := json.Marshal(map[string]string{"configurationJob": id})
	return Response{ID: request.ID, Result: data}
}

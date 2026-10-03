package modules

import (
	"encoding/json"
	"fmt"
	"sync"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modulestate"
)

type ControlJob struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}
type ModuleView struct {
	Manifest    catalog.Manifest `json:"manifest"`
	Enabled     bool             `json:"enabled"`
	PendingStop bool             `json:"pendingStop"`
	Runtime     host.Module      `json:"runtime"`
}
type ControlSnapshot struct {
	Revision uint64       `json:"revision"`
	Modules  []ModuleView `json:"modules"`
}
type Control struct {
	catalogSource func() []catalog.Manifest
	mu            sync.Mutex
	store         *modulestate.Store
	gates         map[string]*Gate
	jobs          map[string]ControlJob
	active        map[string]bool
	completed     []string
	sequence      uint64
}

func NewControl(store *modulestate.Store, gates map[string]*Gate, catalogSource func() []catalog.Manifest) *Control {
	if catalogSource == nil {
		panic("module catalog source is required")
	}
	copied := make(map[string]*Gate, len(gates))
	for id, g := range gates {
		copied[id] = g
	}
	return &Control{store: store, gates: copied, catalogSource: catalogSource, jobs: map[string]ControlJob{}, active: map[string]bool{}}
}
func (c *Control) Snapshot() ControlSnapshot {
	state := c.store.Snapshot()
	result := ControlSnapshot{Revision: state.Revision, Modules: []ModuleView{}}
	for _, manifest := range c.catalogSource() {
		if gate := c.gates[manifest.ID]; gate != nil {
			record := state.Modules[manifest.ID]
			result.Modules = append(result.Modules, ModuleView{manifest, record.Enabled, record.PendingStop, gate.Status()})
		}
	}
	return result
}
func (c *Control) Handle(request host.Request) host.Response {
	fail := func(message string) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_control", Message: message}}
	}
	if request.Version != 1 || request.ID == "" {
		return fail("无效的模块管理请求")
	}
	var result any
	switch request.Method {
	case "snapshot":
		result = c.Snapshot()
	case "catalog":
		result = c.catalogSource()
	case "setEnabled":
		var args struct {
			ID       string `json:"id"`
			Enabled  *bool  `json:"enabled"`
			Revision uint64 `json:"revision"`
		}
		if err := json.Unmarshal(request.Params, &args); err != nil || args.Enabled == nil {
			return fail("模块设置格式无效")
		}
		gate := c.gates[args.ID]
		registered := false
		for _, manifest := range c.catalogSource() {
			if manifest.ID == args.ID {
				registered = true
				break
			}
		}
		if gate == nil || !registered {
			return fail("模块未安装")
		}
		c.mu.Lock()
		if c.active[args.ID] {
			c.mu.Unlock()
			return fail("模块正在切换，请等待完成")
		}
		c.sequence++
		job := ControlJob{ID: fmt.Sprint(c.sequence), State: "running"}
		c.jobs[job.ID] = job
		c.active[args.ID] = true
		c.mu.Unlock()
		go func(job ControlJob) {
			err := gate.SetEnabled(args.Revision, *args.Enabled)
			c.mu.Lock()
			defer c.mu.Unlock()
			job.State = "completed"
			if err != nil {
				job.State = "failed"
				job.Error = err.Error()
			}
			c.jobs[job.ID] = job
			delete(c.active, args.ID)
			c.completed = append(c.completed, job.ID)
			for len(c.completed) > 32 {
				old := c.completed[0]
				c.completed = c.completed[1:]
				delete(c.jobs, old)
			}
		}(job)
		result = job
	case "jobs.get":
		var args struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(request.Params, &args) != nil {
			return fail("任务标识无效")
		}
		c.mu.Lock()
		job, ok := c.jobs[args.ID]
		c.mu.Unlock()
		if !ok {
			return fail("模块任务不存在或已过期")
		}
		result = job
	default:
		return fail("不支持的模块管理操作")
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fail(err.Error())
	}
	return host.Response{ID: request.ID, Result: data}
}
func (c *Control) Busy() bool { c.mu.Lock(); defer c.mu.Unlock(); return len(c.active) > 0 }

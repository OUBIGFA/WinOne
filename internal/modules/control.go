package modules

import (
	"encoding/json"
	"fmt"
	"sort"
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
	Blocked     bool             `json:"blocked,omitempty"`
	RecoveryID  string           `json:"recoveryId,omitempty"`
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
	registrations map[string]*host.FeatureRegistration
	jobs          map[string]ControlJob
	active        map[string]bool
	completed     []string
	sequence      uint64
	unavailable   map[string]ModuleView
}

func NewControl(store *modulestate.Store, gates map[string]*Gate, catalogSource func() []catalog.Manifest) *Control {
	if catalogSource == nil {
		panic("module catalog source is required")
	}
	copied := make(map[string]*Gate, len(gates))
	for id, g := range gates {
		copied[id] = g
	}
	return &Control{store: store, gates: copied, registrations: map[string]*host.FeatureRegistration{}, catalogSource: catalogSource, jobs: map[string]ControlJob{}, active: map[string]bool{}, unavailable: map[string]ModuleView{}}
}

// LifecycleGates gives the Host a stable view of the currently registered
// runtimes. Callers must use the configuration admission lease for mutations;
// the control mutex must not be held while a worker restores resources.
func (c *Control) LifecycleGates() map[string]*Gate {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]*Gate, len(c.gates))
	for id, gate := range c.gates {
		result[id] = gate
	}
	return result
}

// New unavailable entries are presentation only. An existing package gate is
// faulted until recovery retires it, retaining access to its shutdown lifecycle.
// Saved intent/recovery markers remain untouched.
func (c *Control) SetUnavailable(id string, name catalog.LocalizedName, detail string, recoveryID ...string) error {
	if !catalog.ValidOptionalID(id) {
		return fmt.Errorf("invalid unavailable module ID")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gates[id] != nil || c.registrations[id] != nil {
		gate := c.gates[id]
		if gate == nil || c.registrations[id] == nil || gate.packageSHA256 == "" {
			return fmt.Errorf("module runtime must be detached before quarantine")
		}
		gate.mu.Lock()
		gate.lastError = detail
		gate.mu.Unlock()
	}
	if name.Chinese == "" {
		name.Chinese = id
	}
	if name.English == "" {
		name.English = id
	}
	c.unavailable[id] = ModuleView{Manifest: catalog.Manifest{ID: id, Name: name, Distribution: "optional"}, Runtime: host.Module{ID: id, Name: name.Chinese, State: "unavailable", Detail: detail}, Blocked: true}
	if len(recoveryID) > 0 {
		entry := c.unavailable[id]
		entry.RecoveryID = recoveryID[0]
		c.unavailable[id] = entry
	}
	return nil
}

func (c *Control) ForgetUnavailable(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gates[id] != nil || c.registrations[id] != nil {
		return fmt.Errorf("module runtime is still registered")
	}
	delete(c.unavailable, id)
	return nil
}
func (c *Control) Snapshot() ControlSnapshot {
	c.mu.Lock()
	state := c.store.Snapshot()
	manifests := c.catalogSource()
	gates := make(map[string]*Gate, len(c.gates))
	for id, gate := range c.gates {
		gates[id] = gate
	}
	blocked := make([]ModuleView, 0, len(c.unavailable))
	blockedIDs := make(map[string]bool, len(c.unavailable))
	for id, entry := range c.unavailable {
		blockedIDs[id] = true
		record := state.Modules[id]
		entry.Enabled = record.Enabled
		entry.PendingStop = record.PendingStop
		blocked = append(blocked, entry)
	}
	c.mu.Unlock()
	result := ControlSnapshot{Revision: state.Revision, Modules: []ModuleView{}}
	for _, manifest := range manifests {
		if gate := gates[manifest.ID]; gate != nil && !blockedIDs[manifest.ID] {
			record := state.Modules[manifest.ID]
			result.Modules = append(result.Modules, ModuleView{Manifest: manifest, Enabled: record.Enabled, PendingStop: record.PendingStop, Runtime: gate.Status()})
		}
	}
	sort.Slice(blocked, func(i, j int) bool { return blocked[i].Manifest.ID < blocked[j].Manifest.ID })
	result.Modules = append(result.Modules, blocked...)
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
		c.mu.Lock()
		gate := c.gates[args.ID]
		registered := false
		for _, manifest := range c.catalogSource() {
			if manifest.ID == args.ID {
				registered = true
				break
			}
		}
		if gate == nil || !registered {
			c.mu.Unlock()
			return fail("模块未安装")
		}
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

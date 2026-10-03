package modules

import (
	"errors"
	"sync"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modulestate"
	"winone/internal/settings"
)

// LifecycleHooks are implemented by each adapter. Stop must return only after
// restoring owned resources, not merely after sending a process termination request.
type LifecycleHooks struct {
	Start         func() error
	Stop          func() error
	IntentChanged func() error
	Status        func() host.Module
	Handle        func(host.Request) host.Response
}
type Gate struct {
	operations            sync.RWMutex
	mu                    sync.Mutex
	id, name              string
	packageSHA256         string
	store                 *modulestate.Store
	hooks                 LifecycleHooks
	transition, lastError string
	actionGuard           func() (func(), bool)
}

func NewGate(id, name string, store *modulestate.Store, hooks LifecycleHooks) (*Gate, error) {
	if _, ok := store.Snapshot().Modules[id]; !ok {
		return nil, errors.New("模块未登记")
	}
	if hooks.Start == nil || hooks.Stop == nil || hooks.IntentChanged == nil || hooks.Status == nil || hooks.Handle == nil {
		return nil, errors.New("模块生命周期接口不完整")
	}
	return &Gate{id: id, name: name, store: store, hooks: hooks}, nil
}

// Optional-package loaders must use the digest returned by current package
// verification. A physical inventory entry without matching intent cannot run.
func NewPackageGate(manifest catalog.Manifest, digest string, store *modulestate.Store, hooks LifecycleHooks) (*Gate, error) {
	if store == nil {
		return nil, errors.New("module state store is required")
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if manifest.Distribution != "optional" || digest == "" || store.Snapshot().Modules[manifest.ID].PackageSHA256 != digest {
		return nil, errors.New("module package binding is incomplete or changed")
	}
	gate, err := NewGate(manifest.ID, manifest.Name.Chinese, store, hooks)
	if err == nil {
		gate.packageSHA256 = digest
	}
	return gate, err
}

func (g *Gate) matchesPackage(record modulestate.Record) bool {
	return g.packageSHA256 == "" || g.packageSHA256 == record.PackageSHA256
}
func (g *Gate) Status() host.Module {
	g.mu.Lock()
	transition, lastError := g.transition, g.lastError
	g.mu.Unlock()
	record := g.store.Snapshot().Modules[g.id]
	status := host.Module{ID: g.id, Name: g.name}
	switch {
	case !g.matchesPackage(record):
		status.State = "faulted"
		status.Detail = "模块包已变化，请重新加载"
	case transition != "":
		status.State = transition
		status.Detail = "正在更新模块状态"
	case record.PendingStop:
		status.State = "faulted"
		status.Detail = "模块关闭尚未完成，请恢复"
	case lastError != "":
		status.State = "faulted"
		status.Detail = lastError
	case !record.Enabled:
		status.State = "disabled"
		status.Detail = "已关闭"
	default:
		return g.hooks.Status()
	}
	return status
}
func (g *Gate) Enabled() bool {
	record := g.store.Snapshot().Modules[g.id]
	return record.Enabled && !record.PendingStop && g.matchesPackage(record)
}

// SetActionGuard is installed before serving commands or starting hotkey dispatch.
func (g *Gate) SetActionGuard(guard func() (func(), bool)) { g.actionGuard = guard }

func configurationRead(method string) bool {
	switch method {
	case "snapshot", "settings.get", "settings.save", "jobs.get", "lifetime", "icons", "privilege.status", "import.preview":
		return true
	default:
		return false
	}
}

func (g *Gate) Handle(request host.Request) host.Response {
	if g.actionGuard != nil && !configurationRead(request.Method) {
		release, allowed := g.actionGuard()
		if !allowed {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "configuration_busy", Message: "配置正在更新，请稍后操作"}}
		}
		defer release()
	}
	initial := g.store.Snapshot().Modules[g.id]
	if !initial.Enabled || initial.PendingStop || !g.matchesPackage(initial) {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_disabled", Message: "模块已关闭或正在恢复"}}
	}
	g.operations.RLock()
	defer g.operations.RUnlock()
	record := g.store.Snapshot().Modules[g.id]
	if !record.Enabled || record.PendingStop || !g.matchesPackage(record) {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_disabled", Message: "模块已关闭或正在恢复"}}
	}
	g.mu.Lock()
	lastError := g.lastError
	g.mu.Unlock()
	if lastError != "" {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_faulted", Message: lastError}}
	}
	return g.hooks.Handle(request)
}

// SetEnabled serializes native transitions and blocks every normal command route.
// The caller runs it as an asynchronous operation and reports its result to the UI.
func (g *Gate) SetEnabled(revision uint64, enabled bool) error {
	g.operations.Lock()
	defer g.operations.Unlock()
	current := g.store.Snapshot()
	if current.Revision != revision {
		return settings.ErrConflict
	}
	g.mu.Lock()
	lastError := g.lastError
	g.mu.Unlock()
	record := current.Modules[g.id]
	if !g.matchesPackage(record) {
		return errors.New("模块包已变化，请重新加载")
	}
	if record.Enabled == enabled && !record.PendingStop && lastError == "" {
		return nil
	}
	if _, err := g.store.SetIntent(revision, g.id, enabled); err != nil {
		return err
	}
	g.mu.Lock()
	g.lastError = ""
	g.transition = "stopping"
	if enabled {
		g.transition = "starting"
	}
	g.mu.Unlock()
	var result error
	defer func() {
		g.mu.Lock()
		g.transition = ""
		if result != nil {
			g.lastError = result.Error()
		}
		g.mu.Unlock()
	}()
	if result = g.hooks.IntentChanged(); result != nil {
		return result
	}
	if enabled {
		result = g.hooks.Start()
		return result
	}
	if result = g.hooks.Stop(); result != nil {
		return result
	}
	_, result = g.store.FinishStop(g.id)
	return result
}

//go:build windows

package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
)

func (p *installedPackages) removalPreview(request host.Request, store *configuration.HostStore) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_remove", Message: err.Error()}}
	}
	var args struct {
		ID             string `json:"id"`
		Kind           string `json:"kind,omitempty"`
		ManifestSHA256 string `json:"manifestSha256,omitempty"`
	}
	if err := configjson.DecodeObject(request.Params, &args); err != nil {
		return fail(err)
	}
	if args.Kind == "runtime" {
		return p.runtimeRemovalPreview(request, store, args.ID, args.ManifestSHA256)
	}
	if args.Kind != "" && args.Kind != "module" {
		return fail(errors.New("包种类无效"))
	}
	p.mu.RLock()
	entry := p.byID[args.ID]
	p.mu.RUnlock()
	if entry == nil {
		return fail(errors.New("模块未安装"))
	}
	if entry.failure != nil {
		return fail(errors.New("请先恢复模块，再卸载"))
	}
	state := p.state.Snapshot()
	record := state.Modules[args.ID]
	if record.Enabled || record.PendingStop {
		return fail(errors.New("请先关闭模块并完成资源恢复，再卸载"))
	}
	registry, err := modulepackage.OpenRegistry(filepath.Join(p.data, "packages"))
	if err != nil {
		return fail(err)
	}
	snapshot, err := registry.Snapshot()
	if err != nil {
		return fail(err)
	}
	if snapshot.Pending != nil {
		return fail(modulepackage.ErrRecoveryRequired)
	}
	installed := snapshot.Records[args.ID]
	if installed.ManifestSHA256 != record.PackageSHA256 || installed.ModuleID == "" {
		return fail(configuration.ErrConflict)
	}
	approval, err := p.recoveryApproval(installed)
	if err != nil {
		return fail(err)
	}
	_, source, err := p.dependencyStores(registry)
	if err != nil {
		return fail(err)
	}
	if err = requireUnusedPackage(p.ctx, source, installed, "module"); err != nil {
		return fail(err)
	}
	metadata := entry.metadata
	preview := &packagePreview{view: packagePreviewView{ID: rand.Text(), State: "ready", Metadata: &metadata, Publisher: approval.Publisher.Namespace, ManifestSHA256: installed.ManifestSHA256, RestoresSettings: true}, operation: "remove", owner: request.PeerPID, publisher: approval.Publisher, cancel: func() {}, expires: time.Now().Add(10 * time.Minute), configurationRevision: store.CommittedDocument().Revision, moduleRevision: state.Revision, registryRevision: snapshot.Revision}
	return p.cacheRemovalPreview(request, preview)
}

func (p *installedPackages) cacheRemovalPreview(request host.Request, preview *packagePreview) host.Response {
	p.previewMu.Lock()
	defer p.previewMu.Unlock()
	if p.previews == nil {
		p.previews = map[string]*packagePreview{}
	}
	for id, old := range p.previews {
		if time.Now().After(old.expires) || old.owner == request.PeerPID && !old.consumed {
			old.cancel()
			delete(p.previews, id)
		}
	}
	if len(p.previews) >= 8 {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_remove", Message: "操作预览过多，请稍后重试"}}
	}
	p.previews[preview.view.ID] = preview
	raw, _ := json.Marshal(preview.view)
	return host.Response{ID: request.ID, Result: raw}
}

func (p *installedPackages) removePreview(request host.Request, store *configuration.HostStore, service *host.Service, control *modules.Control) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_remove", Message: err.Error()}}
	}
	var args struct {
		ID string `json:"id"`
	}
	if err := configjson.DecodeObject(request.Params, &args); err != nil {
		return fail(err)
	}
	p.previewMu.Lock()
	preview := p.previews[args.ID]
	if preview == nil || preview.owner != request.PeerPID || (preview.operation != "remove" && preview.operation != "remove-runtime") || preview.consumed || preview.view.State != "ready" || time.Now().After(preview.expires) {
		p.previewMu.Unlock()
		return fail(errors.New("卸载预览已失效，请重新确认"))
	}
	copy := *preview
	preview.consumed = true
	p.previewMu.Unlock()
	defer func() { p.previewMu.Lock(); delete(p.previews, args.ID); p.previewMu.Unlock() }()
	if store.CommittedDocument().Revision != copy.configurationRevision || p.state.Snapshot().Revision != copy.moduleRevision {
		return fail(configuration.ErrConflict)
	}
	if err := store.RecoveryError(); err != nil {
		return fail(err)
	}
	if copy.operation == "remove-runtime" {
		reply := p.removeSharedRuntime(copy)
		reply.ID = request.ID
		return reply
	}
	id := copy.view.Metadata.Module.ID
	p.mu.RLock()
	entry := p.byID[id]
	p.mu.RUnlock()
	if entry == nil || entry.failure != nil {
		return fail(errors.New("模块状态已变化，请刷新"))
	}
	registry, err := modulepackage.OpenRegistry(filepath.Join(p.data, "packages"))
	if err != nil {
		return fail(err)
	}
	snapshot, err := registry.Snapshot()
	if err != nil {
		return fail(err)
	}
	if snapshot.Revision != copy.registryRevision || snapshot.Records[id].ManifestSHA256 != copy.view.ManifestSHA256 {
		return fail(configuration.ErrConflict)
	}
	if snapshot.Pending != nil {
		return fail(modulepackage.ErrRecoveryRequired)
	}
	record := p.state.Snapshot().Modules[id]
	if record.Enabled || record.PendingStop {
		return fail(errors.New("请先关闭模块并完成资源恢复"))
	}
	_, source, err := p.dependencyStores(registry)
	if err != nil {
		return fail(err)
	}
	if err = requireUnusedPackage(p.ctx, source, snapshot.Records[id], "module"); err != nil {
		return fail(err)
	}
	if err = entry.runtime.Unload(); err != nil {
		return fail(err)
	}
	change := modulepackage.ChangeRequest{ID: "remove-" + strings.ToLower(rand.Text()), RegistryRevision: snapshot.Revision, StateRevision: p.state.Snapshot().Revision, Current: snapshot.Records[id], Dependencies: source}
	if _, err = registry.BeginChange(p.ctx, change, p.state); err != nil {
		return fail(p.exposeIncompleteInstallation(id, entry.metadata, err, nil, store, control))
	}
	if err = p.removePrepared(entry, change, registry, store, control); err != nil {
		return fail(err)
	}
	warning := p.reclaimPackageFiles(registry, id, store)
	raw, _ := json.Marshal(map[string]any{"state": "completed", "moduleId": id, "warning": warning})
	return host.Response{ID: request.ID, Result: raw}
}

func (p *installedPackages) removePrepared(previous *installedPackage, change modulepackage.ChangeRequest, registry *modulepackage.Registry, store *configuration.HostStore, control *modules.Control) (err error) {
	id := previous.id
	var probe *installedPackage
	defer func() {
		if err != nil {
			var runtime *modulepackage.RuntimeModule
			if probe != nil {
				runtime = probe.runtime
			}
			err = p.exposeIncompleteInstallation(id, previous.metadata, err, runtime, store, control, change.ID)
		}
	}()
	if err = store.QuarantinePackage(id, errors.New("模块正在卸载")); err != nil {
		return err
	}
	p.replaceEntry(&installedPackage{id: id, metadata: previous.metadata, failure: errors.New("模块正在卸载"), pendingID: change.ID})
	hints := store.PackageHints()
	p.mu.Lock()
	p.hints = hints
	p.mu.Unlock()
	probe = p.inspectEntry(registry, id, &modulepackage.ChangePermit{ID: change.ID, RegistryRevision: change.RegistryRevision, RecoveryOnly: true}, false)
	if probe.failure != nil {
		return probe.failure
	}
	raw := store.CommittedDocument().Values[id]
	if len(raw) == 0 {
		return errors.New("模块已提交配置缺失")
	}
	if err = probe.runtime.ProjectSettings(raw, store.CommittedDocument().Revision); err != nil {
		return err
	}
	if _, err = p.state.SetIntent(p.state.Snapshot().Revision, id, false); err != nil {
		return err
	}
	if err = probe.runtime.ProbeRecovery(); err != nil {
		return err
	}
	if _, err = p.state.FinishStop(id); err != nil {
		return err
	}
	if err = store.RestorePackageAdapter(packageAdapter(probe), store.CommittedDocument().Revision); err != nil {
		return err
	}
	if err = store.UnregisterPackageModule(id, store.CommittedDocument().Revision); err != nil {
		return err
	}
	if _, err = registry.ApplyChange(p.ctx, change.ID, p.state, nil); err != nil {
		return err
	}
	finished, finishErr := registry.FinishChange(p.ctx, change.ID, p.state, nil)
	if finishErr != nil && (finished.Pending != nil || finished.LastChange == nil || finished.LastChange.Change.ID != change.ID || finished.LastChange.Outcome != "committed") {
		return finishErr
	}
	p.mu.Lock()
	delete(p.byID, id)
	delete(p.hints, id)
	for index, entry := range p.entries {
		if entry.id == id {
			p.entries = append(p.entries[:index], p.entries[index+1:]...)
			break
		}
	}
	p.mu.Unlock()
	if err = control.ForgetUnavailable(id); err != nil {
		return err
	}
	return nil
}

func (p *installedPackages) reclaimPackageFiles(registry *modulepackage.Registry, id string, store *configuration.HostStore) string {
	queue, err := registry.SyncReclamation(p.ctx)
	if err != nil {
		return "模块变更已完成，文件回收记录需要处理：" + err.Error()
	}
	for key, item := range queue.Items {
		if item.Package.ModuleID != id {
			continue
		}
		if item.MigrationSHA256 != "" {
			if err = store.RecyclePackageMigration(p.ctx, item.ChangeID, item.MigrationSHA256); err != nil {
				return "模块变更已完成，配置迁移备份等待清理：" + err.Error()
			}
		}
		approval, err := p.recoveryApproval(item.Package)
		if err != nil {
			return "模块变更已完成，保留文件等待清理：" + err.Error()
		}
		if _, err = registry.Reclaim(p.ctx, key, approval.Publisher); err != nil {
			return "模块变更已完成，保留文件等待清理：" + err.Error()
		}
	}
	return ""
}

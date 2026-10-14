//go:build windows

package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
)

func (p *installedPackages) updatePreview(request host.Request, store *configuration.HostStore, service *host.Service, control *modules.Control) (reply host.Response) {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_update", Message: err.Error()}}
	}
	var args struct {
		ID string `json:"id"`
	}
	if err := configjson.DecodeObject(request.Params, &args); err != nil {
		return fail(err)
	}
	p.previewMu.Lock()
	preview := p.previews[args.ID]
	if preview == nil || preview.owner != request.PeerPID || preview.operation != "update" || preview.previous == nil || preview.consumed || !preview.view.CanInstall || preview.view.State != "ready" || time.Now().After(preview.expires) {
		p.previewMu.Unlock()
		return fail(errors.New("更新预览已失效，请重新检查模块包"))
	}
	approved := *preview
	preview.consumed = true
	p.previewMu.Unlock()
	defer func() { preview.cancel(); p.previewMu.Lock(); delete(p.previews, args.ID); p.previewMu.Unlock() }()
	if store.CommittedDocument().Revision != approved.configurationRevision || p.state.Snapshot().Revision != approved.moduleRevision {
		return fail(configuration.ErrConflict)
	}
	if err := store.RecoveryError(); err != nil {
		return fail(err)
	}
	previous := *approved.previous
	p.mu.RLock()
	entry := p.byID[previous.ModuleID]
	p.mu.RUnlock()
	if entry == nil || entry.failure != nil || entry.runtime == nil {
		return fail(errors.New("现有模块状态已变化，请先恢复"))
	}
	publisher, err := p.trust(approved.publisher.Namespace)
	if err != nil {
		return fail(err)
	}
	if publisher != approved.publisher {
		return fail(modulepackage.ErrUntrustedPublisher)
	}
	registry, err := modulepackage.OpenRegistry(filepath.Join(p.data, "packages"))
	if err != nil {
		return fail(err)
	}
	snapshot, err := registry.Snapshot()
	if err != nil {
		return fail(err)
	}
	if snapshot.Revision != approved.registryRevision || snapshot.Records[previous.ModuleID] != previous {
		return fail(configuration.ErrConflict)
	}
	if snapshot.Pending != nil {
		return fail(modulepackage.ErrRecoveryRequired)
	}
	_, source, err := p.dependencyStores(registry)
	if err != nil {
		return fail(err)
	}
	options := modulepackage.StageOptions{Directory: filepath.Join(p.data, "packages"), Publisher: publisher, Environment: p.environment, ExpectedManifestSHA256: approved.view.ManifestSHA256, Dependencies: source}
	operation := "update-" + strings.ToLower(rand.Text())
	stage, err := registry.StageArchive(p.ctx, approved.path, options, operation)
	defer p.finishPackagePreparation(registry, stage, &reply)
	if err != nil {
		return fail(err)
	}
	definition, err := stage.LoadSettings(p.ctx)
	if err != nil {
		return fail(err)
	}
	document := store.CommittedDocument()
	candidateSettings, err := definition.Migrate(entry.metadata.Module.SettingsSchemaVersion, document.Values[previous.ModuleID])
	if err != nil {
		return fail(err)
	}
	change := modulepackage.ChangeRequest{ID: operation, RegistryRevision: snapshot.Revision, StateRevision: approved.moduleRevision, Current: previous, Replacement: &modulepackage.ReplacementStage{ID: filepath.Base(stage.Directory()), Options: options}, Dependencies: source}
	var migration *configuration.PackageMigrationSnapshot
	if target := stage.Manifest().Metadata().Module.SettingsSchemaVersion; target != entry.metadata.Module.SettingsSchemaVersion {
		record := configuration.PackageMigrationSnapshot{SchemaVersion: 1, OperationID: change.ID, ModuleID: previous.ModuleID, Revision: document.Revision, BeforeSchema: entry.metadata.Module.SettingsSchemaVersion, AfterSchema: target, BeforePackageSHA256: previous.ManifestSHA256, AfterPackageSHA256: stage.Manifest().Digest(), Before: document.Values[previous.ModuleID], After: candidateSettings}
		change.MigrationSHA256, err = configuration.PackageMigrationDigest(record)
		if err != nil {
			return fail(err)
		}
		if err = registry.BindPreparationMigration(p.ctx, filepath.Base(stage.Directory()), operation, change.MigrationSHA256); err != nil {
			return fail(err)
		}
		savedDigest, saveErr := store.SavePackageMigration(record)
		if saveErr != nil {
			return fail(saveErr)
		}
		if savedDigest != change.MigrationSHA256 {
			return fail(errors.New("配置迁移备份与准备记录不匹配"))
		}
		verified, readErr := store.LoadPackageMigration(change.ID, change.MigrationSHA256)
		if readErr != nil {
			return fail(readErr)
		}
		migration = &verified
	}
	begun, err := registry.BeginChange(p.ctx, change, p.state)
	if err != nil {
		if begun.Pending != nil && begun.Pending.ID == change.ID {
			err = p.exposeIncompleteInstallation(previous.ModuleID, entry.metadata, err, entry.runtime, store, control, change.ID)
		}
		return fail(err)
	}
	warning, err := p.applyPackageUpdate(entry, change, registry, options, migration, store, service, control)
	if err != nil {
		return fail(err)
	}
	raw, _ := json.Marshal(map[string]any{"state": "completed", "moduleId": previous.ModuleID, "version": stage.Manifest().Metadata().Version, "warning": warning})
	return host.Response{ID: request.ID, Result: raw}
}

// A candidate receives isolated readonly settings and cannot change enable
// intent. Migrated values become canonical only after its health/exit check;
// the package journal binds the recovery snapshot until the switch commits.
func (p *installedPackages) applyPackageUpdate(previous *installedPackage, change modulepackage.ChangeRequest, registry *modulepackage.Registry, approval modulepackage.StageOptions, migration *configuration.PackageMigrationSnapshot, store *configuration.HostStore, service *host.Service, control *modules.Control) (warning string, err error) {
	current := previous
	committed, canRollback := false, true
	defer func() {
		if err == nil {
			return
		}
		err = p.exposeIncompleteInstallation(previous.id, current.metadata, err, current.runtime, store, control, change.ID)
		if committed || !canRollback {
			return
		}
		p.mu.RLock()
		blocked := p.byID[previous.id]
		p.mu.RUnlock()
		if restored := p.recoverEntry(blocked, service, control, store); restored != nil {
			err = errors.Join(err, fmt.Errorf("旧版本恢复未完成：%w", restored))
			return
		}
		err = fmt.Errorf("更新失败，已恢复旧版本：%w", err)
		if cleanup := p.reclaimPackageFiles(registry, previous.id, store); cleanup != "" {
			err = errors.Join(err, errors.New(cleanup))
		}
	}()
	if err = store.QuarantinePackage(previous.id, errors.New("模块正在更新")); err != nil {
		return
	}
	if err = previous.runtime.LifecycleGate().SetEnabled(p.state.Snapshot().Revision, false); err != nil {
		canRollback = false
		return
	}
	if err = previous.runtime.Unload(); err != nil {
		canRollback = false
		return
	}
	current = &installedPackage{id: previous.id, metadata: previous.metadata, failure: errors.New("模块正在更新"), pendingID: change.ID}
	p.replaceEntry(current)
	if err = control.SetUnavailable(previous.id, previous.metadata.Module.Name, "模块正在更新", change.ID); err != nil {
		return
	}
	if _, err = registry.ApplyChange(p.ctx, change.ID, p.state, &approval); err != nil {
		return
	}
	hints := store.PackageHints()
	p.mu.Lock()
	p.hints = hints
	p.mu.Unlock()
	values := store.CommittedDocument().Values[previous.id]
	if migration != nil {
		values = migration.After
	}
	candidate := p.inspectEntry(registry, previous.id, &modulepackage.ChangePermit{ID: change.ID, RegistryRevision: change.RegistryRevision}, false, values)
	current = candidate
	if candidate.failure != nil {
		err = candidate.failure
		return
	}
	if migration == nil {
		if err = store.RestorePackageAdapter(packageAdapter(candidate), store.CommittedDocument().Revision); err != nil {
			return
		}
	} else {
		if candidate.metadata.Module.SettingsSchemaVersion != migration.AfterSchema {
			err = errors.New("候选配置版本与迁移备份不匹配")
			return
		}
		if err = candidate.runtime.ProjectSettings(values, migration.Revision); err != nil {
			return
		}
	}
	if _, err = p.state.SetIntent(p.state.Snapshot().Revision, previous.id, false); err != nil {
		return
	}
	restored, probeErr := candidate.runtime.ProbeCandidate()
	if restored {
		if _, err = p.state.FinishStop(previous.id); err != nil {
			return
		}
	} else {
		canRollback = false
	}
	if probeErr != nil {
		err = probeErr
		return
	}
	if migration != nil {
		if err = store.ApplyPackageMigration(*migration, true, candidate.definition.Validate); err != nil {
			return
		}
		if err = store.RestorePackageAdapter(packageAdapter(candidate), store.CommittedDocument().Revision); err != nil {
			return
		}
	}
	if err = candidate.runtime.LifecycleGate().SetEnabled(p.state.Snapshot().Revision, store.CommittedDocument().ModuleEnabled[previous.id]); err != nil {
		return
	}
	finished, finishErr := registry.FinishChange(p.ctx, change.ID, p.state, &approval)
	committed = finished.Pending == nil && finished.LastChange != nil && finished.LastChange.Change.ID == change.ID && finished.LastChange.Outcome == "committed"
	if finishErr != nil && !committed {
		err = finishErr
		return
	}
	if err = candidate.runtime.Bind(control, service); err != nil {
		return
	}
	candidate.pendingID = ""
	p.replaceEntry(candidate)
	warning = p.reclaimPackageFiles(registry, previous.id, store)
	if finishErr != nil && warning == "" {
		warning = "模块更新已完成，文件回收记录需要处理：" + finishErr.Error()
	}
	return
}

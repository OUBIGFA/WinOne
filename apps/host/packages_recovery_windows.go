//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
)

func packageAdapter(entry *installedPackage) configuration.ModuleAdapter {
	return configuration.ModuleAdapter{ID: entry.id, PackageSchemaVersion: entry.metadata.Module.SettingsSchemaVersion, Load: func() (json.RawMessage, error) { return entry.definition.Defaults(), nil }, Validate: entry.definition.Validate, Apply: entry.runtime.ProjectSettings, Migrate: entry.definition.Migrate}
}
func (p *installedPackages) replaceEntry(entry *installedPackage) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byID[entry.id] = entry
	for index, current := range p.entries {
		if current.id == entry.id {
			p.entries[index] = entry
			return
		}
	}
	p.entries = append(p.entries, entry)
}

// The configuration queue owns admission for this entire operation. No route
// is published until the verified worker's probe/shutdown and journal finish.
func (p *installedPackages) recoveryHandler(service *host.Service, control *modules.Control, store *configuration.HostStore) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		var args struct {
			ID         string `json:"id"`
			Revision   uint64 `json:"revision"`
			RecoveryID string `json:"recoveryId"`
		}
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_package", Message: err.Error()}}
		}
		if err := configjson.DecodeObject(request.Params, &args); err != nil {
			return fail(err)
		}
		if p.state.Snapshot().Revision != args.Revision {
			return fail(configuration.ErrConflict)
		}
		p.mu.RLock()
		entry := p.byID[args.ID]
		p.mu.RUnlock()
		if entry == nil {
			return fail(errors.New("模块恢复对象不存在，请刷新"))
		}
		if entry.failure == nil {
			return host.Response{ID: request.ID, Result: json.RawMessage(`{"state":"completed"}`)}
		}
		if entry.pendingID != args.RecoveryID {
			return fail(configuration.ErrConflict)
		}
		if err := p.recoverEntry(entry, service, control, store); err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: json.RawMessage(`{"state":"completed"}`)}
	}
}

func (p *installedPackages) recoverEntry(previous *installedPackage, service *host.Service, control *modules.Control, store *configuration.HostStore) (err error) {
	entry := &installedPackage{id: previous.id, metadata: previous.metadata, pendingID: previous.pendingID}
	adopted := false
	defer func() {
		if err == nil {
			return
		}
		if entry.runtime != nil {
			closeErr := entry.runtime.Close()
			err = errors.Join(err, closeErr)
			if closeErr == nil {
				entry.runtime = nil
			}
		}
		if adopted {
			err = errors.Join(err, store.QuarantinePackage(entry.id, err))
		}
		entry.failure = err
		p.replaceEntry(entry)
		err = errors.Join(err, control.SetUnavailable(entry.id, entry.metadata.Module.Name, err.Error(), entry.pendingID))
	}()
	if previous.runtime != nil {
		entry.runtime = previous.runtime
		if err = entry.runtime.CloseForRecovery(); err != nil {
			return err
		}
		entry.runtime = nil
	}
	registry, err := modulepackage.OpenRegistry(filepath.Join(p.data, "packages"))
	if err != nil {
		return err
	}
	snapshot, err := registry.Snapshot()
	if err != nil {
		return err
	}
	hints := store.PackageHints()
	p.mu.Lock()
	p.hints = hints
	p.inventoryError = nil
	p.mu.Unlock()
	var change *modulepackage.PackageChange
	if snapshot.Pending != nil && snapshot.Pending.Before.ModuleID == entry.id {
		copy := *snapshot.Pending
		change = &copy
	}
	if change != nil && change.ID != previous.pendingID {
		return configuration.ErrConflict
	}
	if change == nil && previous.pendingID != "" && (snapshot.LastChange == nil || snapshot.LastChange.Change.ID != previous.pendingID) {
		return configuration.ErrConflict
	}
	migration, migrationErr := packageMigrationForChange(store, change)
	if migrationErr != nil {
		return migrationErr
	}
	// First installation can stop after inventory registration but before its
	// disabled intent binding is durable. Re-verify that exact registered stage
	// and finish only a missing binding; never replace an active/different one.
	if change == nil {
		record, exists := snapshot.Records[entry.id]
		intent := p.state.Snapshot().Modules[entry.id]
		if exists && intent.PackageSHA256 == "" && !intent.Enabled && !intent.PendingStop {
			approval, approvalErr := p.recoveryApproval(record)
			if approvalErr != nil {
				return approvalErr
			}
			if _, err = registry.PrepareModule(p.ctx, record.StageID, approval, snapshot.Revision, p.state); err != nil {
				return err
			}
		}
	}
	if change != nil {
		if store.CommittedDocument().ModuleEnabled[entry.id] != change.PreviousState.Enabled {
			return errors.New("模块启用选择与待恢复记录不一致，请核对配置")
		}
		current := p.state.Snapshot().Modules[entry.id]
		if change.Phase == "prepared" && current == change.PreviousState {
			if _, err = registry.CancelChange(p.ctx, change.ID, p.state); err != nil {
				return err
			}
			change = nil
		} else {
			if current.Enabled || current.PendingStop {
				raw := store.CommittedDocument().Values[entry.id]
				var schemaVersion int
				if migration != nil {
					raw, schemaVersion, err = migrationSettingsForPackage(migration, snapshot.Records[entry.id])
					if err != nil {
						return err
					}
				}
				probe := p.inspectEntry(registry, entry.id, &modulepackage.ChangePermit{ID: change.ID, RegistryRevision: change.RegistryRevision, RecoveryOnly: true}, false, raw)
				if probe.failure != nil {
					return probe.failure
				}
				entry.runtime = probe.runtime
				entry.metadata = probe.metadata
				if migration != nil && probe.metadata.Module.SettingsSchemaVersion != schemaVersion {
					return errors.New("资源恢复配置版本与备份不匹配")
				}
				if len(raw) == 0 {
					return errors.New("缺少模块已提交配置，无法验证资源恢复")
				}
				if err = probe.runtime.ProjectSettings(raw, store.CommittedDocument().Revision); err != nil {
					return err
				}
				if !current.PendingStop {
					if _, err = p.state.SetIntent(p.state.Snapshot().Revision, entry.id, false); err != nil {
						return err
					}
				}
				if err = probe.runtime.ProbeRecovery(); err != nil {
					return err
				}
				if _, err = p.state.FinishStop(entry.id); err != nil {
					return err
				}
				entry.runtime = nil
			}
			approval, approvalErr := p.recoveryApproval(change.Before)
			if approvalErr != nil {
				return approvalErr
			}
			// Validate the rollback configuration before changing registration.
			stage, verifyErr := modulepackage.VerifyStage(p.ctx, change.Before.StageID, approval)
			if verifyErr != nil {
				return verifyErr
			}
			definition, verifyErr := stage.LoadSettings(p.ctx)
			if verifyErr != nil {
				return verifyErr
			}
			raw := store.CommittedDocument().Values[entry.id]
			if migration != nil {
				if stage.Manifest().Metadata().Module.SettingsSchemaVersion != migration.BeforeSchema {
					return errors.New("旧版配置定义与迁移备份不匹配")
				}
				raw = migration.Before
			}
			if len(raw) > 0 {
				if err = definition.Validate(raw); err != nil {
					return err
				}
			}
			if _, err = registry.RestoreChange(p.ctx, change.ID, p.state, approval); err != nil {
				return err
			}
			if migration != nil {
				if err = store.ApplyPackageMigration(*migration, false, definition.Validate); err != nil {
					return err
				}
				hints := store.PackageHints()
				p.mu.Lock()
				p.hints = hints
				p.mu.Unlock()
			}
		}
	}
	var permit *modulepackage.ChangePermit
	if change != nil {
		permit = &modulepackage.ChangePermit{ID: change.ID, RegistryRevision: change.RegistryRevision}
	}
	candidate := p.inspectEntry(registry, entry.id, permit, false)
	if candidate.failure != nil {
		return candidate.failure
	}
	candidate.pendingID = entry.pendingID
	entry = candidate
	if err = store.RestorePackageAdapter(packageAdapter(entry), store.CommittedDocument().Revision); err != nil {
		return err
	}
	adopted = true
	// Recovery can be interrupted after the worker starts. Persist admission
	// closure first so the next Host cannot treat an unconfirmed stop as idle.
	if !p.state.Snapshot().Modules[entry.id].PendingStop {
		if _, err = p.state.SetIntent(p.state.Snapshot().Revision, entry.id, false); err != nil {
			return err
		}
	}
	if err = entry.runtime.ProbeRecovery(); err != nil {
		return err
	}
	// This marker is cleared only after a real worker restored and exited.
	if p.state.Snapshot().Modules[entry.id].PendingStop {
		if _, err = p.state.FinishStop(entry.id); err != nil {
			return err
		}
	}
	if err = store.Recover(); err != nil {
		return err
	}
	if _, exists := store.CommittedDocument().Values[entry.id]; !exists {
		if err = store.RegisterPackageModule(packageAdapter(entry), store.CommittedDocument().Revision); err != nil {
			return err
		}
	}
	if err = entry.runtime.LifecycleGate().SetEnabled(p.state.Snapshot().Revision, store.CommittedDocument().ModuleEnabled[entry.id]); err != nil {
		return err
	}
	if change != nil {
		approval, approvalErr := p.recoveryApproval(change.Before)
		if approvalErr != nil {
			return approvalErr
		}
		if _, err = registry.FinishChange(p.ctx, change.ID, p.state, &approval); err != nil {
			return err
		}
	}
	if err = entry.runtime.Bind(control, service); err != nil {
		return err
	}
	entry.pendingID = ""
	p.replaceEntry(entry)
	return nil
}

func (p *installedPackages) recoveryApproval(record modulepackage.RegisteredPackage) (modulepackage.StageOptions, error) {
	namespace, _, _ := strings.Cut(record.ModuleID, ".")
	publisher, err := p.trust(namespace)
	if err != nil {
		return modulepackage.StageOptions{}, err
	}
	_, source, err := p.dependencyStores(nil)
	return modulepackage.StageOptions{Directory: filepath.Join(p.data, "packages"), Publisher: publisher, Environment: p.environment, ExpectedManifestSHA256: record.ManifestSHA256, Dependencies: source}, err
}

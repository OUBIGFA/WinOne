//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
)

type packageCleanupStatus struct {
	Pending bool   `json:"pending"`
	Error   string `json:"error,omitempty"`
}

// Status reads the small control files and bounded directory names, not payload.
// A missing post-commit ledger entry still offers cleanup so it can be repaired.
func (p *installedPackages) packageCleanupStatus() packageCleanupStatus {
	failed := func(err error) packageCleanupStatus { return packageCleanupStatus{Pending: true, Error: err.Error()} }
	directory := filepath.Join(p.data, "packages")
	if _, err := os.Stat(directory); errors.Is(err, os.ErrNotExist) {
		operations, err := configuration.PackageMigrationOperations(p.ctx, p.data)
		if err != nil {
			return failed(err)
		}
		return packageCleanupStatus{Pending: len(operations) > 0}
	}
	registry, err := modulepackage.OpenRegistry(directory)
	if err != nil {
		return failed(err)
	}
	snapshot, err := registry.Snapshot()
	if err != nil {
		return failed(err)
	}
	ledger, err := registry.Reclamation()
	if err != nil {
		return failed(err)
	}
	pending := len(ledger.Items) > 0
	if snapshot.LastChange != nil && snapshot.LastChange.Revision > ledger.ObservedChangeRevision {
		pending = true
	}
	if !pending {
		stages, err := registry.UntrackedStages(p.ctx)
		if err != nil {
			return failed(err)
		}
		pending = len(stages) > 0
	}
	if !pending {
		operations, err := p.untrackedPackageMigrations(snapshot, ledger)
		if err != nil {
			return failed(err)
		}
		pending = len(operations) > 0
	}
	runtimes, _, err := p.dependencyStores(registry)
	if err != nil {
		return failed(err)
	}
	sharedPending, err := runtimes.CleanupPending(p.ctx)
	if err != nil {
		return failed(err)
	}
	return packageCleanupStatus{Pending: pending || sharedPending}
}

func (p *installedPackages) cleanupPackages(request host.Request, store *configuration.HostStore) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "package_cleanup", Message: err.Error()}}
	}
	if request.Method == "cleanupStatus" {
		raw, _ := json.Marshal(p.packageCleanupStatus())
		return host.Response{ID: request.ID, Result: raw}
	}
	directory := filepath.Join(p.data, "packages")
	if _, err := os.Stat(directory); errors.Is(err, os.ErrNotExist) {
		operations, err := configuration.PackageMigrationOperations(p.ctx, p.data)
		if err != nil {
			return fail(err)
		}
		if len(operations) > 0 {
			return fail(errors.New("存在无法核对包状态的迁移备份，文件已保留，请先恢复模块"))
		}
		return host.Response{ID: request.ID, Result: json.RawMessage(`{"state":"completed"}`)}
	}
	registry, err := modulepackage.OpenRegistry(directory)
	if err != nil {
		return fail(err)
	}
	ledger, err := registry.SyncReclamation(p.ctx)
	if err != nil {
		return fail(err)
	}
	recovery, err := registry.RecoverPreparations(p.ctx, p.trust)
	if err != nil {
		return fail(err)
	}
	migrations := p.recoverLegacyMigrationReferences(registry, store)
	ledger, err = registry.Reclamation()
	if err != nil {
		return fail(err)
	}
	keys := make([]string, 0, len(ledger.Items))
	for key := range ledger.Items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var failures []error
	for _, issue := range recovery.Issues {
		failures = append(failures, fmt.Errorf("%s：%s", issue.StageID, issue.Reason))
	}
	failures = append(failures, migrations.failures...)
	for _, key := range keys {
		item := ledger.Items[key]
		if migrations.blockedStages[item.Package.StageID] {
			continue
		}
		if item.MigrationSHA256 != "" {
			if err = store.RecyclePackageMigration(p.ctx, item.ChangeID, item.MigrationSHA256); err != nil {
				failures = append(failures, err)
				continue
			}
		}
		approval, err := p.recoveryApproval(item.Package)
		if err == nil {
			_, err = registry.Reclaim(p.ctx, key, approval.Publisher)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s：%w", item.Package.ModuleID, err))
		}
	}
	runtimes, source, sharedErr := p.dependencyStores(registry)
	if sharedErr == nil {
		sharedErr = runtimes.Cleanup(p.ctx, source)
	}
	if sharedErr != nil {
		failures = append(failures, sharedErr)
	}
	if len(failures) > 0 {
		return fail(fmt.Errorf("部分安装文件仍待清理。请检查文件占用或完整性后重试：%w", errors.Join(failures...)))
	}
	return host.Response{ID: request.ID, Result: json.RawMessage(`{"state":"completed"}`)}
}

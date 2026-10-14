//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"winone/internal/configuration"
	"winone/internal/modulepackage"
)

type legacyMigrationRecovery struct {
	failures      []error
	blockedStages map[string]bool
}

func knownPackageMigrations(snapshot modulepackage.RegistrySnapshot, ledger modulepackage.ReclamationSnapshot) map[string]bool {
	known := map[string]bool{}
	if change := snapshot.Pending; change != nil && change.MigrationSHA256 != "" {
		known[change.ID] = true
	}
	if receipt := snapshot.LastChange; receipt != nil && receipt.Change.MigrationSHA256 != "" {
		known[receipt.Change.ID] = true
	}
	for _, item := range ledger.Items {
		if item.MigrationSHA256 != "" {
			known[item.ChangeID] = true
		}
	}
	return known
}

func (p *installedPackages) untrackedPackageMigrations(snapshot modulepackage.RegistrySnapshot, ledger modulepackage.ReclamationSnapshot) ([]string, error) {
	operations, err := configuration.PackageMigrationOperations(p.ctx, p.data)
	if err != nil {
		return nil, err
	}
	known := knownPackageMigrations(snapshot, ledger)
	var result []string
	for _, operation := range operations {
		if !known[operation] {
			result = append(result, operation)
		}
	}
	return result, nil
}

func sameMigrationJSON(left, right json.RawMessage) bool {
	var a, b bytes.Buffer
	return json.Compact(&a, left) == nil && json.Compact(&b, right) == nil && bytes.Equal(a.Bytes(), b.Bytes())
}

// A legacy backup becomes actionable only when the current old package and
// settings, the signed candidate migration, and the backup bytes all agree.
// Unresolved backup relationships keep matching stages as recovery evidence.
func (p *installedPackages) recoverLegacyMigrationReferences(registry *modulepackage.Registry, store *configuration.HostStore) legacyMigrationRecovery {
	result := legacyMigrationRecovery{blockedStages: map[string]bool{}}
	snapshot, err := registry.Snapshot()
	if err != nil {
		result.failures = append(result.failures, err)
		return result
	}
	ledger, err := registry.Reclamation()
	if err != nil {
		result.failures = append(result.failures, err)
		return result
	}
	block := func(module string) {
		for _, item := range ledger.Items {
			if item.Kind == "preparation" && (module == "" || item.Package.ModuleID == module) {
				result.blockedStages[item.Package.StageID] = true
			}
		}
	}
	operations, err := p.untrackedPackageMigrations(snapshot, ledger)
	if err != nil {
		block("")
		result.failures = append(result.failures, err)
		return result
	}
	if len(operations) == 0 {
		return result
	}
	if err = store.RecoveryError(); err != nil || snapshot.Pending != nil {
		block("")
		result.failures = append(result.failures, errors.New("存在未完成的配置或包变更，迁移备份已保留"))
		return result
	}
	document := store.CommittedDocument()
	keys := make([]string, 0, len(ledger.Items))
	for key := range ledger.Items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, operation := range operations {
		if err = p.ctx.Err(); err != nil {
			block("")
			result.failures = append(result.failures, err)
			break
		}
		backup, digest, readErr := store.InspectPackageMigration(operation)
		if readErr != nil {
			block("")
			result.failures = append(result.failures, fmt.Errorf("%s：迁移备份无法验证，已保留", operation))
			continue
		}
		fail := func(reason error) {
			block(backup.ModuleID)
			result.failures = append(result.failures, fmt.Errorf("%s：%w", operation, reason))
		}
		current, exists := snapshot.Records[backup.ModuleID]
		if !exists || current.ManifestSHA256 != backup.BeforePackageSHA256 || !sameMigrationJSON(document.Values[backup.ModuleID], backup.Before) {
			fail(errors.New("备份与当前旧包或配置不一致，已保留"))
			continue
		}
		namespace, _, _ := strings.Cut(backup.ModuleID, ".")
		publisher, trustErr := p.trust(namespace)
		if trustErr != nil {
			fail(trustErr)
			continue
		}
		old, checkErr := registry.Inspect(p.ctx, backup.ModuleID, publisher)
		if checkErr != nil || old.Manifest().Metadata().Module.SettingsSchemaVersion != backup.BeforeSchema {
			fail(errors.New("旧包配置定义无法验证，备份已保留"))
			continue
		}
		matched := ""
		for _, key := range keys {
			item := ledger.Items[key]
			if item.Kind == "preparation" && item.Phase == "queued" && item.ChangeID == "legacy-"+item.Package.StageID && item.MigrationSHA256 == "" && item.Package.ModuleID == backup.ModuleID && item.Package.ManifestSHA256 == backup.AfterPackageSHA256 {
				matched = key
				break
			}
		}
		if matched == "" {
			fail(errors.New("未找到对应的完整暂存包，备份已保留"))
			continue
		}
		item := ledger.Items[matched]
		candidate, checkErr := registry.InspectStoredPackage(p.ctx, item.Package, publisher)
		if checkErr != nil || candidate.Settings() == nil || candidate.Manifest().Metadata().Module.SettingsSchemaVersion != backup.AfterSchema {
			fail(errors.New("暂存包配置定义无法验证，备份已保留"))
			continue
		}
		converted, checkErr := candidate.Settings().Migrate(backup.BeforeSchema, backup.Before)
		if checkErr != nil || !sameMigrationJSON(converted, backup.After) {
			fail(errors.New("备份与已签名的迁移规则不一致，已保留"))
			continue
		}
		if _, checkErr = store.LoadPackageMigration(operation, digest); checkErr != nil || store.CommittedDocument().Revision != document.Revision {
			fail(errors.New("检查期间配置或备份已变化，请重新检查"))
			continue
		}
		if err = registry.BindRecoveredPreparationMigration(p.ctx, item.Package.StageID, operation, digest, backup.BeforePackageSHA256); err != nil {
			fail(err)
			continue
		}
		item.ChangeID = operation
		item.MigrationSHA256 = digest
		ledger.Items[matched] = item
	}
	return result
}

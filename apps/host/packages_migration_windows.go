//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"winone/internal/configuration"
	"winone/internal/modulepackage"
)

func packageMigrationForChange(store *configuration.HostStore, change *modulepackage.PackageChange) (*configuration.PackageMigrationSnapshot, error) {
	if change == nil || change.MigrationSHA256 == "" {
		return nil, nil
	}
	snapshot, err := store.LoadPackageMigration(change.ID, change.MigrationSHA256)
	if err != nil {
		return nil, err
	}
	if change.After == nil || snapshot.ModuleID != change.Before.ModuleID || snapshot.BeforePackageSHA256 != change.Before.ManifestSHA256 || snapshot.AfterPackageSHA256 != change.After.ManifestSHA256 {
		return nil, errors.New("配置迁移备份与包变更不匹配")
	}
	return &snapshot, nil
}

func migrationSettingsForPackage(snapshot *configuration.PackageMigrationSnapshot, record modulepackage.RegisteredPackage) (json.RawMessage, int, error) {
	if record.ManifestSHA256 == snapshot.BeforePackageSHA256 {
		return snapshot.Before, snapshot.BeforeSchema, nil
	}
	if record.ManifestSHA256 == snapshot.AfterPackageSHA256 {
		return snapshot.After, snapshot.AfterSchema, nil
	}
	return nil, 0, errors.New("包状态与配置迁移备份不匹配")
}

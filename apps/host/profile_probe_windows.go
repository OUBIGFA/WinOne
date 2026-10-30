//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/modulepackage"
	"winone/internal/modulestate"
	"winone/internal/startupprobe"
)

// This follows normal discovery's quarantine policy without constructing a
// runtime, loading a projection, running migrations, or changing module intent.
func checkProfileUpgrade(ctx context.Context, directory string, input startupprobe.ProfileSnapshot, environment modulepackage.Environment, trust func(string) (modulepackage.Publisher, error)) ([]startupprobe.ProfileIssue, error) {
	before, err := startupprobe.ReadProfileSnapshot(directory)
	if err != nil {
		return nil, err
	}
	if before.Digest() != input.Digest() {
		return nil, errors.New("升级检查期间配置已变化")
	}
	hints, err := configuration.PackageHintsFromData(input.Configuration)
	if err != nil {
		return nil, err
	}
	var state modulestate.Snapshot
	if input.ModuleState != nil {
		state, err = modulestate.DecodeSnapshot(input.ModuleState)
		if err != nil {
			return nil, err
		}
		for id, record := range state.Modules {
			if record.PendingStop {
				return nil, fmt.Errorf("模块 %s 尚未完成资源恢复，请先在 WinOne 中恢复后再升级", id)
			}
		}
	}
	var registry *modulepackage.Registry
	var runtimes *modulepackage.RuntimeStore
	var dependencies *modulepackage.DependencySource
	var registered modulepackage.RegistrySnapshot
	// A first installation can leave preparation bookkeeping before inventory
	// exists. Validate that ledger too instead of accepting unknown formats.
	if input.Registry != nil || input.Reclamation != nil || input.RuntimeRegistry != nil || input.RuntimeReclamation != nil {
		registry, err = modulepackage.OpenRegistry(filepath.Join(directory, "packages"))
		if err != nil {
			return nil, err
		}
		registered, err = registry.Snapshot()
		if err != nil {
			return nil, err
		}
		if registered.Pending != nil {
			return nil, fmt.Errorf("模块 %s 有未完成的包变更，请先在 WinOne 中恢复后再升级", registered.Pending.Before.ModuleID)
		}
		if input.Reclamation != nil {
			if _, err = registry.Reclamation(); err != nil {
				return nil, err
			}
		}
		runtimes, err = modulepackage.OpenRuntimeStore(filepath.Join(directory, "packages"))
		if err != nil {
			return nil, err
		}
		if _, err = runtimes.CleanupPending(ctx); err != nil {
			return nil, err
		}
		dependencies, err = modulepackage.NewDependencySource(registry, runtimes, environment, trust)
		if err != nil {
			return nil, err
		}
	}
	known := map[string]bool{}
	for id := range hints {
		known[id] = true
	}
	for id := range registered.Records {
		known[id] = true
	}
	if len(known) > 4096 {
		return nil, errors.New("扩展模块数量超过检查限制")
	}
	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	adapters := []configuration.ModuleAdapter{aotConfiguration("", nil), captureConfiguration("", nil), wintrayConfiguration("", nil, nil), selectionConfiguration("", nil), quickAssistantConfiguration("", nil), strokeConfiguration("", nil), timedPowerConfiguration()}
	issues := []startupprobe.ProfileIssue{}
	if runtimes != nil {
		shared, err := runtimes.Snapshot()
		if err != nil {
			return nil, err
		}
		for _, record := range shared.Records {
			reference := modulepackage.Dependency{Kind: "runtime", ID: record.ModuleID, Version: record.Version, ManifestSHA256: record.ManifestSHA256}
			namespace, _, _ := strings.Cut(record.ModuleID, ".")
			publisher, inspectErr := trust(namespace)
			var manifest *modulepackage.VerifiedManifest
			if inspectErr == nil {
				manifest, inspectErr = runtimes.Inspect(ctx, reference, publisher)
			}
			if inspectErr == nil {
				report, err := manifest.Metadata().CheckCompatibility(environment)
				if err != nil {
					return nil, err
				}
				if !report.Compatible {
					return nil, fmt.Errorf("共享依赖 %s %s 与目标 WinOne 版本不兼容，请先处理依赖它的模块", record.ModuleID, record.Version)
				}
				_, inspectErr = runtimes.Lookup(ctx, reference, publisher, environment)
				if inspectErr == nil {
					_, inspectErr = dependencies.Resolve(ctx, manifest)
				}
			}
			if inspectErr != nil {
				issues = append(issues, startupprobe.ProfileIssue{ModuleID: record.ModuleID, Reason: "共享依赖未通过校验；相关模块将保持隔离，文件与配置保留。"})
			}
		}
	}
	quarantine := func(id, reason string) {
		issues = append(issues, startupprobe.ProfileIssue{ModuleID: id, Reason: reason})
		adapters = append(adapters, configuration.ModuleAdapter{ID: id, Unavailable: errors.New(reason), Validate: func(raw json.RawMessage) error { return configjson.ValidateObject(raw) }})
	}
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if !catalog.ValidOptionalID(id) {
			return nil, errors.New("扩展模块标识无效")
		}
		record, exists := registered.Records[id]
		if !exists {
			quarantine(id, "模块包未登记；配置将保留并隔离。")
			continue
		}
		namespace, _, _ := strings.Cut(id, ".")
		publisher, trustErr := trust(namespace)
		if trustErr != nil {
			quarantine(id, "发布者当前不受信任；配置将保留并隔离。")
			continue
		}
		inspected, inspectErr := registry.Inspect(ctx, id, publisher)
		if inspectErr != nil {
			quarantine(id, "现有模块文件或配置定义未通过校验；配置将保留并隔离。")
			continue
		}
		metadata := inspected.Manifest().Metadata()
		if metadata.PackageKind() != "module" {
			return nil, fmt.Errorf("模块 %s 的包类型与登记不一致，不能覆盖升级", id)
		}
		compatibility, checkErr := metadata.CheckCompatibility(environment)
		if checkErr != nil {
			return nil, checkErr
		}
		if !compatibility.Compatible {
			return nil, fmt.Errorf("模块 %s 与目标 WinOne 版本不兼容，请先更新或卸载该模块", id)
		}
		if _, err = dependencies.Resolve(ctx, inspected.Manifest()); err != nil {
			quarantine(id, "所需依赖缺失或未通过校验；配置将保留并隔离。")
			continue
		}
		if inspected.Settings() == nil || metadata.Module.Activation != "on-demand" || len(metadata.Module.ShortcutActions) > 0 {
			quarantine(id, "模块需要尚未支持的入口；配置将保留并隔离。")
			continue
		}
		intent, exists := state.Modules[id]
		if !exists || intent.PackageSHA256 != record.ManifestSHA256 {
			quarantine(id, "模块包状态绑定未完成；配置将保留并隔离。")
			continue
		}
		definition := inspected.Settings()
		if raw := hints[id].Values; len(raw) > 0 {
			if err = definition.Validate(raw); err != nil {
				quarantine(id, "现有模块配置需要恢复；原数据将保留并隔离。")
				continue
			}
		}
		adapters = append(adapters, configuration.ModuleAdapter{ID: id, PackageSchemaVersion: metadata.Module.SettingsSchemaVersion, Validate: definition.Validate})
	}
	if err = configuration.CheckUpgrade(input.Configuration, adapters...); err != nil {
		return nil, err
	}
	after, err := startupprobe.ReadProfileSnapshot(directory)
	if err != nil {
		return nil, err
	}
	if after.Digest() != input.Digest() {
		return nil, errors.New("升级检查期间配置或模块登记已变化")
	}
	return issues, nil
}

//go:build windows

package main

import (
	"aead.dev/minisign"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
	"winone/internal/modulestate"
)

// Base64 JSON set by the build. Archives, profile imports and RPC callers may
// not grant trust to their own publisher. Empty builds authorize no publishers.
var modulePublishers string

func moduleTrust(encoded string) (func(string) (modulepackage.Publisher, error), error) {
	known := map[string]modulepackage.Publisher{}
	if encoded != "" {
		if len(encoded) > 128<<10 {
			return nil, errors.New("module publisher configuration exceeds size limit")
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, err
		}
		var value struct {
			Publishers []modulepackage.Publisher `json:"publishers"`
		}
		if err = configjson.DecodeObject(raw, &value); err != nil {
			return nil, err
		}
		for _, publisher := range value.Publishers {
			var key minisign.PublicKey
			if !catalog.ValidID(publisher.Namespace) || strings.Contains(publisher.Namespace, ".") || len(publisher.PublicKey) > 8192 || key.UnmarshalText([]byte(publisher.PublicKey)) != nil {
				return nil, errors.New("module publisher configuration is invalid")
			}
			if _, exists := known[publisher.Namespace]; exists {
				return nil, errors.New("duplicate module publisher")
			}
			known[publisher.Namespace] = publisher
		}
	}
	return func(namespace string) (modulepackage.Publisher, error) {
		publisher, ok := known[namespace]
		if !ok {
			return modulepackage.Publisher{}, modulepackage.ErrUntrustedPublisher
		}
		return publisher, nil
	}, nil
}

type installedPackage struct {
	id         string
	metadata   modulepackage.Metadata
	definition *modulepackage.SettingsDefinition
	runtime    *modulepackage.RuntimeModule
	failure    error
	pendingID  string
}
type installedPackages struct {
	entries        []*installedPackage
	byID           map[string]*installedPackage
	mu             sync.RWMutex
	resumeError    error
	inventoryError error
	ctx            context.Context
	data, pipe     string
	state          *modulestate.Store
	environment    modulepackage.Environment
	trust          func(string) (modulepackage.Publisher, error)
	guard          func() (func(), bool)
	hints          map[string]configuration.PackageHint
	previewMu      sync.Mutex
	previews       map[string]*packagePreview
}

func openInstalledPackages(ctx context.Context, data, pipe string, state *modulestate.Store, guard func() (func(), bool)) (*installedPackages, error) {
	trust, err := moduleTrust(modulePublishers)
	if err != nil {
		return nil, err
	}
	environment := packageEnvironment()
	return discoverInstalledPackages(ctx, data, pipe, state, environment, trust, guard)
}

func packageEnvironment() modulepackage.Environment {
	platform := "windows-" + runtime.GOARCH
	if runtime.GOARCH == "amd64" {
		platform = "windows-x64"
	}
	return modulepackage.Environment{HostVersion: updateVersion, ProtocolVersion: 1, DesignSystemVersion: 1, Platform: platform, WindowsBuild: windows.RtlGetVersion().BuildNumber}
}

func discoverInstalledPackages(ctx context.Context, data, pipe string, state *modulestate.Store, environment modulepackage.Environment, trust func(string) (modulepackage.Publisher, error), guard func() (func(), bool)) (result *installedPackages, err error) {
	result = &installedPackages{byID: map[string]*installedPackage{}, ctx: ctx, data: data, pipe: pipe, state: state, environment: environment, trust: trust, guard: guard}
	result.hints, err = configuration.ReadPackageHints(data)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for id := range result.hints {
		known[id] = true
	}
	for id, record := range state.Snapshot().Modules {
		if catalog.ValidOptionalID(id) && (record.PackageSHA256 != "" || record.Enabled || record.PendingStop) {
			known[id] = true
		}
	}
	directory := filepath.Join(data, "packages")
	var registry *modulepackage.Registry
	var snapshot modulepackage.RegistrySnapshot
	if _, statErr := os.Stat(directory); statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) || len(known) > 0 {
			result.inventoryError = statErr
		}
	} else {
		registry, err = modulepackage.OpenRegistry(directory)
		if err == nil {
			snapshot, err = registry.Snapshot()
		}
		result.inventoryError = err
	}
	for id := range snapshot.Records {
		known[id] = true
	}
	if snapshot.Pending != nil {
		known[snapshot.Pending.Before.ModuleID] = true
	}
	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		entry := &installedPackage{id: id}
		switch {
		case result.inventoryError != nil:
			entry.failure = result.inventoryError
		case snapshot.Pending != nil && snapshot.Pending.Before.ModuleID == id:
			entry.failure = modulepackage.ErrRecoveryRequired
			entry.pendingID = snapshot.Pending.ID
		case snapshot.Records[id].ModuleID == "":
			entry.failure = errors.New("模块登记缺失，请修复或重新安装模块")
		case state.Snapshot().Modules[id].PendingStop:
			entry.failure = errors.New("模块资源恢复尚未完成，请重试加载")
		default:
			entry = result.inspectEntry(registry, id, nil, true)
		}
		result.entries = append(result.entries, entry)
		result.byID[id] = entry
	}
	return result, ctx.Err()
}

func (p *installedPackages) inspectEntry(registry *modulepackage.Registry, id string, permit *modulepackage.ChangePermit, checkPending bool, candidateSettings ...json.RawMessage) *installedPackage {
	entry := &installedPackage{id: id}
	namespace, _, _ := strings.Cut(id, ".")
	publisher, err := p.trust(namespace)
	if err != nil {
		entry.failure = err
		return entry
	}
	var stage *modulepackage.StagedPackage
	if permit == nil {
		stage, err = registry.Resolve(p.ctx, id, publisher, p.environment)
	} else {
		snapshot, e := registry.Snapshot()
		err = e
		if err == nil {
			record := snapshot.Records[id]
			stage, err = modulepackage.VerifyStage(p.ctx, record.StageID, modulepackage.StageOptions{Directory: filepath.Join(p.data, "packages"), Publisher: publisher, Environment: p.environment, ExpectedManifestSHA256: record.ManifestSHA256})
		}
	}
	if err != nil {
		entry.failure = err
		return entry
	}
	entry.metadata = stage.Manifest().Metadata()
	entry.definition, err = stage.LoadSettings(p.ctx)
	if err != nil {
		entry.failure = err
		return entry
	}
	hint := p.hints[id]
	// Only the coordinator supplies a verified migration candidate. Normal
	// discovery continues to use committed hints and treats pending work as blocked.
	if len(candidateSettings) > 0 {
		if len(candidateSettings) != 1 || len(candidateSettings[0]) == 0 {
			entry.failure = errors.New("迁移候选配置缺失")
			return entry
		}
		hint.Values = candidateSettings[0]
		hint.PendingValues = nil
	}
	if len(hint.Values) > 0 {
		err = entry.definition.Validate(hint.Values)
	}
	if err == nil && checkPending && len(hint.PendingValues) > 0 {
		err = entry.definition.Validate(hint.PendingValues)
	}
	if err != nil {
		entry.failure = err
		return entry
	}
	digest := sha256.Sum256([]byte(p.pipe + "/" + id))
	_, source, err := p.dependencyStores(registry)
	if err != nil {
		entry.failure = err
		return entry
	}
	entry.runtime, entry.failure = registry.PrepareRuntime(p.ctx, id, modulepackage.RuntimeOptions{State: p.state, Trust: p.trust, Environment: p.environment, Pipe: `\\.\pipe\WinOne-package-` + hex.EncodeToString(digest[:16]), DataDirectory: filepath.Join(p.data, "modules", id), ActionGuard: p.guard, Change: permit, Dependencies: source, HostPipe: p.pipe})
	return entry
}

func (p *installedPackages) entriesSnapshot() []*installedPackage {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]*installedPackage(nil), p.entries...)
}
func (p *installedPackages) isolated() map[string]bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := map[string]bool{}
	state := p.state.Snapshot()
	for _, entry := range p.entries {
		if entry.failure != nil {
			intent := state.Modules[entry.id].Enabled
			if hint, ok := p.hints[entry.id]; ok {
				intent = hint.Enabled
			}
			result[entry.id] = intent
		}
	}
	return result
}

func (p *installedPackages) addGates(gates map[string]*modules.Gate) {
	for _, entry := range p.entriesSnapshot() {
		if entry.runtime != nil {
			gates[entry.id] = entry.runtime.LifecycleGate()
		}
	}
}
func (p *installedPackages) adapters() []configuration.ModuleAdapter {
	result := []configuration.ModuleAdapter{}
	for _, entry := range p.entriesSnapshot() {
		if entry.failure != nil {
			result = append(result, configuration.ModuleAdapter{ID: entry.id, Unavailable: entry.failure})
			continue
		}
		result = append(result, configuration.ModuleAdapter{ID: entry.metadata.Module.ID, PackageSchemaVersion: entry.metadata.Module.SettingsSchemaVersion, Load: func() (json.RawMessage, error) { return entry.definition.Defaults(), nil }, Validate: entry.definition.Validate, Apply: entry.runtime.ProjectSettings})
	}
	return result
}

func (p *installedPackages) bind(service *host.Service, control *modules.Control, store *configuration.HostStore) error {
	for _, entry := range p.entriesSnapshot() {
		if entry.failure != nil {
			if err := control.SetUnavailable(entry.id, entry.metadata.Module.Name, entry.failure.Error(), entry.pendingID); err != nil {
				return err
			}
			continue
		}
		if err := entry.runtime.Bind(control, service); err != nil {
			return err
		}
	}
	service.RegisterModule("packages", func() host.Module {
		return host.Module{ID: "packages", State: "ready", Detail: p.Warning(applicationLanguage(store.Snapshot().Language))}
	}, p.handler(store))
	return nil
}

func (p *installedPackages) handler(store *configuration.HostStore) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_package", Message: err.Error()}}
		}
		var result any
		if request.Method == "snapshot" {
			entries := []any{}
			for _, entry := range p.entriesSnapshot() {
				if entry.failure != nil {
					entries = append(entries, map[string]any{"id": entry.id, "blocked": true, "error": entry.failure.Error(), "pendingId": entry.pendingID, "running": entry.runtime != nil && entry.runtime.Running()})
					continue
				}
				entries = append(entries, map[string]any{"id": entry.id, "manifest": entry.metadata.Module, "version": entry.metadata.Version, "runtime": entry.runtime.LifecycleGate().Status(), "running": entry.runtime.Running()})
			}
			result = map[string]any{"packages": entries, "error": p.Warning(applicationLanguage(store.Snapshot().Language))}
		} else {
			var args struct {
				ID       string          `json:"id"`
				Revision uint64          `json:"revision"`
				Settings json.RawMessage `json:"settings"`
			}
			if err := configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			p.mu.RLock()
			entry := p.byID[args.ID]
			p.mu.RUnlock()
			if entry == nil {
				return fail(errors.New("模块未安装"))
			}
			if entry.failure != nil {
				return fail(entry.failure)
			}
			switch request.Method {
			case "settings.get":
				raw, revision, err := store.Module(args.ID)
				if err != nil {
					return fail(err)
				}
				result = map[string]any{"revision": revision, "settings": raw, "schema": entry.definition.Schema()}
			case "settings.save":
				raw, revision, err := store.SaveModule(args.ID, args.Revision, args.Settings)
				if err != nil {
					return fail(err)
				}
				result = map[string]any{"revision": revision, "settings": raw}
			default:
				return fail(errors.New("不支持的模块包操作"))
			}
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: raw}
	}
}

func (p *installedPackages) Running() bool {
	for _, entry := range p.entriesSnapshot() {
		if entry.runtime != nil && entry.runtime.Running() {
			return true
		}
	}
	return false
}
func (p *installedPackages) Close() error {
	p.cancelPreviews()
	entries := p.entriesSnapshot()
	for index, entry := range entries {
		if entry.runtime == nil {
			continue
		}
		if err := entry.runtime.Close(); err != nil {
			for previous := index - 1; previous >= 0; previous-- {
				if entries[previous].runtime != nil {
					err = errors.Join(err, entries[previous].runtime.Resume())
				}
			}
			return err
		}
	}
	return nil
}
func (p *installedPackages) Resume() {
	var err error
	for _, entry := range p.entriesSnapshot() {
		if entry.runtime != nil {
			err = errors.Join(err, entry.runtime.Resume())
		}
	}
	p.mu.Lock()
	p.resumeError = err
	p.mu.Unlock()
}
func (p *installedPackages) Warning(locale ...string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var messages []string
	english := len(locale) > 0 && locale[0] == "en-US"
	if p.resumeError != nil {
		prefix := "模块恢复未完成："
		if english {
			prefix = "Module recovery is incomplete: "
		}
		messages = append(messages, prefix+p.resumeError.Error())
	}
	if p.inventoryError != nil {
		prefix := "模块登记需要修复："
		if english {
			prefix = "Module inventory needs repair: "
		}
		messages = append(messages, prefix+p.inventoryError.Error())
	}
	for _, entry := range p.entries {
		if entry.failure != nil {
			format := "模块 %s 未加载：%s"
			if english {
				format = "Module %s could not load: %s"
			}
			messages = append(messages, fmt.Sprintf(format, entry.id, entry.failure))
		}
	}
	return strings.Join(messages, "\n")
}

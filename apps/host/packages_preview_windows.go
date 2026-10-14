//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/distribution"
	"winone/internal/host"
	"winone/internal/modulepackage"
)

type packagePreviewView struct {
	ID               string                             `json:"id"`
	Operation        string                             `json:"operation,omitempty"`
	PreviousVersion  string                             `json:"previousVersion,omitempty"`
	WasEnabled       bool                               `json:"wasEnabled,omitempty"`
	MigrationFrom    int                                `json:"migrationFrom,omitempty"`
	MigrationTo      int                                `json:"migrationTo,omitempty"`
	MigrationPaths   []string                           `json:"migrationPaths,omitempty"`
	State            string                             `json:"state"`
	FileName         string                             `json:"fileName"`
	Metadata         *modulepackage.Metadata            `json:"metadata,omitempty"`
	Publisher        string                             `json:"publisher,omitempty"`
	KeyFingerprint   string                             `json:"keyFingerprint,omitempty"`
	ManifestSHA256   string                             `json:"manifestSha256,omitempty"`
	PayloadBytes     int64                              `json:"payloadBytes,omitempty"`
	Compatibility    *modulepackage.CompatibilityReport `json:"compatibility,omitempty"`
	CanInstall       bool                               `json:"canInstall"`
	RestoresSettings bool                               `json:"restoresSettings,omitempty"`
	Error            string                             `json:"error,omitempty"`
	DependencyLock   *modulepackage.DependencyLock      `json:"dependencyLock,omitempty"`
}
type packagePreview struct {
	view                                                    packagePreviewView
	owner                                                   uint32
	path                                                    string
	expires                                                 time.Time
	cancel                                                  context.CancelFunc
	publisher                                               modulepackage.Publisher
	configurationRevision, moduleRevision, registryRevision uint64
	consumed                                                bool
	operation                                               string
	previous                                                *modulepackage.RegisteredPackage
	runtimeRevision                                         uint64
}

func (p *installedPackages) cancelPreviews() {
	p.previewMu.Lock()
	defer p.previewMu.Unlock()
	for _, preview := range p.previews {
		if !preview.consumed {
			preview.cancel()
			preview.view.State = "canceled"
			preview.view.CanInstall = false
		}
	}
}

func (p *installedPackages) handlePreview(request host.Request, store *configuration.HostStore) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_preview", Message: err.Error()}}
	}
	var args struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	if err := configjson.DecodeObject(request.Params, &args); err != nil {
		return fail(err)
	}
	if request.Method == "packagePreview.start" {
		if !filepath.IsAbs(args.Path) || strings.ContainsRune(args.Path, '\x00') {
			return fail(errors.New("请选择本地模块包"))
		}
		p.previewMu.Lock()
		if p.previews == nil {
			p.previews = map[string]*packagePreview{}
		}
		for id, previous := range p.previews {
			if time.Now().After(previous.expires) || previous.owner == request.PeerPID && !previous.consumed {
				previous.cancel()
				delete(p.previews, id)
			}
		}
		if len(p.previews) >= 8 {
			p.previewMu.Unlock()
			return fail(errors.New("安装预览过多，请关闭其他预览后重试"))
		}
		id := rand.Text()
		ctx, cancel := context.WithTimeout(p.ctx, 90*time.Second)
		preview := &packagePreview{view: packagePreviewView{ID: id, State: "checking", FileName: filepath.Base(args.Path)}, operation: "install", owner: request.PeerPID, path: args.Path, expires: time.Now().Add(10 * time.Minute), cancel: cancel}
		p.previews[id] = preview
		raw, _ := json.Marshal(preview.view)
		p.previewMu.Unlock()
		go p.checkPreview(ctx, preview, store)
		return host.Response{ID: request.ID, Result: raw}
	}
	p.previewMu.Lock()
	defer p.previewMu.Unlock()
	preview := p.previews[args.ID]
	if preview == nil && request.Method == "packagePreview.cancel" {
		raw, _ := json.Marshal(packagePreviewView{ID: args.ID, State: "canceled"})
		return host.Response{ID: request.ID, Result: raw}
	}
	if preview == nil || preview.owner != request.PeerPID || time.Now().After(preview.expires) {
		return fail(errors.New("安装预览不存在或已过期，请重新选择模块包"))
	}
	switch request.Method {
	case "packagePreview.get":
	case "packagePreview.cancel":
		if !preview.consumed {
			preview.cancel()
			preview.view.State = "canceled"
			preview.view.CanInstall = false
		}
	default:
		return fail(errors.New("不支持的安装预览操作"))
	}
	raw, _ := json.Marshal(preview.view)
	return host.Response{ID: request.ID, Result: raw}
}

func (p *installedPackages) checkPreview(ctx context.Context, preview *packagePreview, store *configuration.HostStore) {
	defer preview.cancel()
	view := packagePreviewView{ID: preview.view.ID, State: "failed", Operation: "install", FileName: preview.view.FileName}
	var publisher modulepackage.Publisher
	var previous *modulepackage.RegisteredPackage
	var previousMetadata modulepackage.Metadata
	checked, err := modulepackage.PreviewArchive(ctx, preview.path, func(namespace string) (modulepackage.Publisher, error) {
		var err error
		publisher, err = p.trust(namespace)
		return publisher, err
	})
	var configRevision, moduleRevision, registryRevision, runtimeRevision uint64
	if err == nil {
		metadata := checked.Manifest().Metadata()
		isRuntime := metadata.PackageKind() == "runtime"
		view.Metadata = &metadata
		view.Publisher = publisher.Namespace
		sum := sha256.Sum256([]byte(publisher.PublicKey))
		view.KeyFingerprint = hex.EncodeToString(sum[:])
		view.ManifestSHA256 = checked.Manifest().Digest()
		for _, file := range checked.Manifest().Files() {
			view.PayloadBytes += file.Size
		}
		report, checkErr := metadata.CheckCompatibility(p.environment)
		err = checkErr
		view.Compatibility = &report
		if err == nil && !report.Compatible {
			err = errors.New("模块与当前 WinOne 版本、系统或平台不兼容")
		}
		if err == nil && !isRuntime && (checked.Settings() == nil || metadata.Module.Activation != "on-demand" || len(metadata.Module.ShortcutActions) > 0) {
			err = errors.New("当前 WinOne 尚不支持此模块的配置或启动契约")
		}
		if err == nil {
			directory := filepath.Join(p.data, "packages")
			var snapshot modulepackage.RegistrySnapshot
			var registry *modulepackage.Registry
			if _, statErr := os.Stat(directory); statErr == nil {
				opened, openErr := modulepackage.OpenRegistry(directory)
				registry = opened
				err = openErr
				if err == nil {
					snapshot, err = registry.Snapshot()
				}
			} else if !errors.Is(statErr, os.ErrNotExist) {
				err = statErr
			}
			registryRevision = snapshot.Revision
			if err == nil && snapshot.Pending != nil {
				err = errors.New("请先恢复未完成的模块变更")
			}
			var source *modulepackage.DependencySource
			var runtimeSnapshot modulepackage.RuntimeSnapshot
			if err == nil && registry != nil {
				var runtimes *modulepackage.RuntimeStore
				runtimes, source, err = p.dependencyStores(registry)
				if err == nil {
					runtimeSnapshot, err = runtimes.Snapshot()
					runtimeRevision = runtimeSnapshot.Revision
				}
			}
			if err == nil {
				var lookup func(context.Context, modulepackage.Dependency) (*modulepackage.VerifiedManifest, error)
				if source != nil {
					lookup = source.Lookup
				}
				var lock modulepackage.DependencyLock
				lock, err = modulepackage.ResolveDependencies(ctx, checked.Manifest(), p.environment, lookup)
				if err == nil {
					view.DependencyLock = &lock
				}
			}
			for _, record := range runtimeSnapshot.Records {
				if record.ModuleID == metadata.PackageID() && (!isRuntime || record.Version == metadata.Version) {
					err = errors.New("此依赖版本或相同标识的共享包已安装，请选择其他版本")
				}
			}
			for id, record := range snapshot.Records {
				if isRuntime {
					if id == metadata.PackageID() {
						err = errors.New("相同标识已用于模块，无法安装为共享依赖")
					}
					continue
				}
				if id == metadata.Module.ID {
					view.Operation = "update"
					view.PreviousVersion = record.Version
					order, versionErr := distribution.CompareVersions(metadata.Version, record.Version)
					if err == nil && (versionErr != nil || order <= 0) {
						err = errors.New("请选择比已安装版本更新的模块包")
					}
					if err == nil {
						old, verifyErr := registry.Resolve(ctx, id, publisher, p.environment)
						err = verifyErr
						if err == nil {
							previous = &record
							previousMetadata = old.Manifest().Metadata()
							err = requireUnusedPackage(ctx, source, record, "module")
						}
					}
					continue
				}
				if strings.HasPrefix(id, metadata.Module.ID+".") || strings.HasPrefix(metadata.Module.ID, id+".") {
					err = errors.New("同一命名空间已安装其他模块，请先处理冲突")
					break
				}
			}
		}
		if err == nil && isRuntime {
			configRevision = store.CommittedDocument().Revision
			moduleRevision = p.state.Snapshot().Revision
		}
		if err == nil && !isRuntime {
			moduleRevision = p.state.Snapshot().Revision
			definition := checked.Settings()
			if previous == nil {
				_, configRevision, err = store.PreviewPackageSettings(configuration.ModuleAdapter{ID: metadata.Module.ID, PackageSchemaVersion: metadata.Module.SettingsSchemaVersion, Load: func() (json.RawMessage, error) { return definition.Defaults(), nil }, Validate: definition.Validate, Migrate: definition.Migrate})
				if from, versionErr := store.RetainedPackageVersion(metadata.Module.ID); versionErr != nil {
					err = versionErr
				} else if from > 0 && from != metadata.Module.SettingsSchemaVersion {
					view.MigrationFrom = from
					view.MigrationTo = metadata.Module.SettingsSchemaVersion
				}
			} else {
				document := store.CommittedDocument()
				configRevision = document.Revision
				view.WasEnabled = document.ModuleEnabled[metadata.Module.ID]
				state := p.state.Snapshot().Modules[metadata.Module.ID]
				p.mu.RLock()
				entry := p.byID[metadata.Module.ID]
				p.mu.RUnlock()
				switch {
				case store.RecoveryError() != nil:
					err = store.RecoveryError()
				case entry == nil || entry.failure != nil || state.PendingStop:
					err = errors.New("请先恢复现有模块，再更新")
				case state.PackageSHA256 != previous.ManifestSHA256 || state.Enabled != view.WasEnabled:
					err = configuration.ErrConflict
				case len(document.Values[metadata.Module.ID]) == 0:
					err = errors.New("现有模块配置缺失，请先恢复")
				default:
					_, err = definition.Migrate(previousMetadata.Module.SettingsSchemaVersion, document.Values[metadata.Module.ID])
					if previousMetadata.Module.SettingsSchemaVersion != metadata.Module.SettingsSchemaVersion {
						view.MigrationFrom = previousMetadata.Module.SettingsSchemaVersion
						view.MigrationTo = metadata.Module.SettingsSchemaVersion
					}
				}
			}
			if err == nil && view.MigrationFrom > 0 {
				paths, pathErr := definition.MigrationPaths(view.MigrationFrom)
				err = pathErr
				if len(paths) > 32 {
					paths = paths[:32]
				}
				view.MigrationPaths = paths
			}
			if p.state.Snapshot().Revision != moduleRevision {
				err = configuration.ErrConflict
			}
			_, view.RestoresSettings = store.CommittedDocument().RetainedModules[metadata.Module.ID]
		}
	}
	if err == nil {
		view.State = "ready"
		view.CanInstall = true
	} else {
		view.Error = err.Error()
	}
	p.previewMu.Lock()
	defer p.previewMu.Unlock()
	if p.previews[view.ID] != preview || preview.view.State == "canceled" || preview.consumed {
		return
	}
	if ctx.Err() != nil {
		view.State = "failed"
		view.CanInstall = false
		view.Error = "模块包检查已取消或超时，请重试"
	}
	preview.view = view
	preview.operation = view.Operation
	preview.previous = previous
	preview.publisher = publisher
	preview.configurationRevision = configRevision
	preview.moduleRevision = moduleRevision
	preview.registryRevision = registryRevision
	preview.runtimeRevision = runtimeRevision
}

//go:build windows

package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
)

func (p *installedPackages) moduleHandler(service *host.Service, control *modules.Control, store *configuration.HostStore) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		switch request.Method {
		case "packageInventory":
			raw, _ := json.Marshal(p.packageInventory())
			return host.Response{ID: request.ID, Result: raw}
		case "packagePreview.start", "packagePreview.get", "packagePreview.cancel":
			return p.handlePreview(request, store)
		case "installPackage":
			return p.installPreview(request, store, service, control)
		case "updatePackage":
			return p.updatePreview(request, store, service, control)
		case "cleanupStatus", "cleanupPackages":
			return p.cleanupPackages(request, store)
		case "packageRemoval.preview":
			return p.removalPreview(request, store)
		case "removePackage":
			return p.removePreview(request, store, service, control)
		default:
			return p.recoveryHandler(service, control, store)(request)
		}
	}
}

// Confirmation carries only an owned preview ID. Paths, publishers, hashes and
// revisions come from that verified preview, not fresh caller-controlled fields.
func (p *installedPackages) installPreview(request host.Request, store *configuration.HostStore, service *host.Service, control *modules.Control) (reply host.Response) {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_install", Message: err.Error()}}
	}
	var args struct {
		ID string `json:"id"`
	}
	if err := configjson.DecodeObject(request.Params, &args); err != nil {
		return fail(err)
	}
	p.previewMu.Lock()
	preview := p.previews[args.ID]
	if preview == nil || preview.owner != request.PeerPID || preview.operation != "install" || preview.consumed || preview.view.State != "ready" || !preview.view.CanInstall || time.Now().After(preview.expires) {
		p.previewMu.Unlock()
		return fail(errors.New("安装预览无效或已使用，请重新选择模块包"))
	}
	copy := *preview
	preview.consumed = true
	p.previewMu.Unlock()
	defer func() { preview.cancel(); p.previewMu.Lock(); delete(p.previews, args.ID); p.previewMu.Unlock() }()
	if store.CommittedDocument().Revision != copy.configurationRevision || p.state.Snapshot().Revision != copy.moduleRevision {
		return fail(configuration.ErrConflict)
	}
	if err := store.RecoveryError(); err != nil {
		return fail(err)
	}
	publisher, err := p.trust(copy.publisher.Namespace)
	if err != nil {
		return fail(err)
	}
	if publisher != copy.publisher {
		return fail(modulepackage.ErrUntrustedPublisher)
	}
	directory := filepath.Join(p.data, "packages")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return fail(err)
	}
	registry, err := modulepackage.OpenRegistry(directory)
	if err != nil {
		return fail(err)
	}
	snapshot, err := registry.Snapshot()
	if err != nil {
		return fail(err)
	}
	if snapshot.Revision != copy.registryRevision {
		return fail(configuration.ErrConflict)
	}
	if snapshot.Pending != nil {
		return fail(modulepackage.ErrRecoveryRequired)
	}
	if copy.view.Metadata.PackageKind() == "runtime" {
		reply = p.installSharedRuntime(copy, registry, publisher)
		reply.ID = request.ID
		return reply
	}
	id := copy.view.Metadata.Module.ID
	for existing := range snapshot.Records {
		if existing == id || strings.HasPrefix(existing, id+".") || strings.HasPrefix(id, existing+".") {
			return fail(errors.New("模块命名空间已安装，请刷新"))
		}
	}
	_, source, err := p.dependencyStores(registry)
	if err != nil {
		return fail(err)
	}
	options := modulepackage.StageOptions{Directory: directory, Publisher: publisher, Environment: p.environment, ExpectedManifestSHA256: copy.view.ManifestSHA256, Dependencies: source}
	stage, err := registry.StageArchive(p.ctx, copy.path, options, "install-"+strings.ToLower(rand.Text()))
	defer p.finishPackagePreparation(registry, stage, &reply)
	if err != nil {
		if stage != nil {
			err = errors.Join(err, recordHostEvent(p.data, "package-stage-incomplete", fmt.Errorf("%s: %w", stage.Directory(), err)))
		}
		return fail(err)
	}
	definition, err := stage.LoadSettings(p.ctx)
	if err != nil {
		return fail(err)
	}
	prepared, err := registry.PrepareModule(p.ctx, filepath.Base(stage.Directory()), options, snapshot.Revision, p.state)
	if err != nil {
		if prepared.Package.ModuleID != "" {
			err = p.exposeIncompleteInstallation(prepared.Package.ModuleID, stage.Manifest().Metadata(), err, nil, store, control)
		}
		return fail(err)
	}
	hints := store.PackageHints()
	p.mu.Lock()
	p.hints = hints
	p.mu.Unlock()
	entry := p.inspectEntry(registry, id, nil, false)
	if entry.failure != nil {
		return fail(p.exposeIncompleteInstallation(id, stage.Manifest().Metadata(), entry.failure, entry.runtime, store, control))
	}
	entry.definition = definition
	if err = store.RegisterPackageModule(packageAdapter(entry), copy.configurationRevision); err != nil {
		return fail(p.exposeIncompleteInstallation(id, entry.metadata, err, entry.runtime, store, control))
	}
	if err = entry.runtime.Bind(control, service); err != nil {
		return fail(p.exposeIncompleteInstallation(id, entry.metadata, err, entry.runtime, store, control))
	}
	p.replaceEntry(entry)
	p.mu.Lock()
	p.inventoryError = nil
	p.mu.Unlock()
	raw, _ := json.Marshal(map[string]any{"state": "completed", "moduleId": id, "enabled": false})
	return host.Response{ID: request.ID, Result: raw}
}

func (p *installedPackages) exposeIncompleteInstallation(id string, metadata modulepackage.Metadata, failure error, runtime *modulepackage.RuntimeModule, store *configuration.HostStore, control *modules.Control, pendingIDs ...string) error {
	if runtime != nil {
		closeErr := runtime.CloseForRecovery()
		failure = errors.Join(failure, closeErr)
		if closeErr == nil {
			runtime = nil
		}
	}
	failure = errors.Join(failure, store.QuarantinePackage(id, failure))
	entry := &installedPackage{id: id, metadata: metadata, failure: failure, runtime: runtime}
	if len(pendingIDs) > 0 {
		entry.pendingID = pendingIDs[0]
	}
	p.replaceEntry(entry)
	return errors.Join(failure, control.SetUnavailable(id, metadata.Module.Name, failure.Error(), entry.pendingID))
}

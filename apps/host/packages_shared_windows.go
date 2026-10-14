//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"winone/internal/catalog"
	"winone/internal/configuration"
	"winone/internal/distribution"
	"winone/internal/host"
	"winone/internal/modulepackage"
)

// Both module and runtime operations resolve through these fixed Host stores.
// Opening them is read-only; the caller creates storage only after confirmation.
func (p *installedPackages) dependencyStores(registry *modulepackage.Registry) (*modulepackage.RuntimeStore, *modulepackage.DependencySource, error) {
	directory := filepath.Join(p.data, "packages")
	var err error
	if registry == nil {
		registry, err = modulepackage.OpenRegistry(directory)
		if err != nil {
			return nil, nil, err
		}
	}
	runtimes, err := modulepackage.OpenRuntimeStore(directory)
	if err != nil {
		return nil, nil, err
	}
	source, err := modulepackage.NewDependencySource(registry, runtimes, p.environment, p.trust)
	return runtimes, source, err
}

type sharedRuntimeView struct {
	modulepackage.Dependency
	Name       catalog.LocalizedName      `json:"name"`
	RequiredBy []modulepackage.Dependency `json:"requiredBy"`
	CanRemove  bool                       `json:"canRemove"`
	Error      string                     `json:"error,omitempty"`
}
type packageInventoryView struct {
	Runtimes     []sharedRuntimeView                   `json:"runtimes"`
	Dependencies map[string][]modulepackage.Dependency `json:"dependencies"`
	Error        string                                `json:"error,omitempty"`
}

func (p *installedPackages) packageInventory() packageInventoryView {
	view := packageInventoryView{Runtimes: []sharedRuntimeView{}, Dependencies: map[string][]modulepackage.Dependency{}}
	for _, entry := range p.entriesSnapshot() {
		if len(entry.metadata.Dependencies) > 0 {
			view.Dependencies[entry.id] = entry.metadata.Dependencies
		}
	}
	if _, err := os.Stat(filepath.Join(p.data, "packages")); errors.Is(err, os.ErrNotExist) {
		return view
	}
	runtimes, source, err := p.dependencyStores(nil)
	if err != nil {
		view.Error = err.Error()
		return view
	}
	current, err := runtimes.Snapshot()
	if err != nil {
		view.Error = err.Error()
		return view
	}
	targets := make([]modulepackage.Dependency, 0, len(current.Records))
	for _, record := range current.Records {
		targets = append(targets, modulepackage.Dependency{Kind: "runtime", ID: record.ModuleID, Version: record.Version, ManifestSHA256: record.ManifestSHA256})
	}
	references, referenceErr := source.ReferencesFor(p.ctx, targets)
	for _, record := range current.Records {
		row := sharedRuntimeView{Dependency: modulepackage.Dependency{Kind: "runtime", ID: record.ModuleID, Version: record.Version, ManifestSHA256: record.ManifestSHA256}, Name: catalog.LocalizedName{Chinese: record.ModuleID, English: record.ModuleID}, RequiredBy: []modulepackage.Dependency{}}
		namespace, _, _ := strings.Cut(record.ModuleID, ".")
		publisher, err := p.trust(namespace)
		if err == nil {
			var manifest *modulepackage.VerifiedManifest
			manifest, err = runtimes.Inspect(p.ctx, row.Dependency, publisher)
			if err == nil {
				row.Name = manifest.Metadata().Runtime.Name
				row.RequiredBy, err = references[row.Dependency], referenceErr
			}
		}
		if err != nil {
			row.Error = err.Error()
		} else {
			row.CanRemove = len(row.RequiredBy) == 0
		}
		view.Runtimes = append(view.Runtimes, row)
	}
	sort.Slice(view.Runtimes, func(i, j int) bool {
		if view.Runtimes[i].ID == view.Runtimes[j].ID {
			// Inventory loading has already validated both versions. Compare
			// version components so 10.0.0 follows 2.0.0 instead of preceding it.
			order, _ := distribution.CompareVersions(view.Runtimes[i].Version, view.Runtimes[j].Version)
			return order < 0
		}
		return view.Runtimes[i].ID < view.Runtimes[j].ID
	})
	return view
}

func (p *installedPackages) runtimeRemovalPreview(request host.Request, store *configuration.HostStore, id, digest string) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "runtime_remove", Message: err.Error()}}
	}
	registry, err := modulepackage.OpenRegistry(filepath.Join(p.data, "packages"))
	if err != nil {
		return fail(err)
	}
	registered, err := registry.Snapshot()
	if err != nil {
		return fail(err)
	}
	if registered.Pending != nil {
		return fail(modulepackage.ErrRecoveryRequired)
	}
	runtimes, source, err := p.dependencyStores(registry)
	if err != nil {
		return fail(err)
	}
	current, err := runtimes.Snapshot()
	if err != nil {
		return fail(err)
	}
	record, exists := current.Records[digest]
	if !exists || record.ModuleID != id {
		return fail(errors.New("共享依赖版本已变化，请刷新"))
	}
	if err = requireUnusedPackage(p.ctx, source, record, "runtime"); err != nil {
		return fail(err)
	}
	namespace, _, _ := strings.Cut(id, ".")
	publisher, err := p.trust(namespace)
	if err != nil {
		return fail(err)
	}
	manifest, err := runtimes.Inspect(p.ctx, modulepackage.Dependency{Kind: "runtime", ID: id, Version: record.Version, ManifestSHA256: digest}, publisher)
	if err != nil {
		return fail(err)
	}
	metadata := manifest.Metadata()
	preview := &packagePreview{view: packagePreviewView{ID: rand.Text(), State: "ready", Operation: "remove", Metadata: &metadata, ManifestSHA256: digest, Publisher: publisher.Namespace}, operation: "remove-runtime", owner: request.PeerPID, publisher: publisher, cancel: func() {}, expires: time.Now().Add(10 * time.Minute), configurationRevision: store.CommittedDocument().Revision, moduleRevision: p.state.Snapshot().Revision, registryRevision: registered.Revision, runtimeRevision: current.Revision}
	return p.cacheRemovalPreview(request, preview)
}

func (p *installedPackages) removeSharedRuntime(approved packagePreview) host.Response {
	fail := func(err error) host.Response {
		return host.Response{Error: &host.RPCError{Code: "runtime_remove", Message: err.Error()}}
	}
	runtimes, source, err := p.dependencyStores(nil)
	if err != nil {
		return fail(err)
	}
	current, err := runtimes.Snapshot()
	if err != nil {
		return fail(err)
	}
	if current.Revision != approved.runtimeRevision {
		return fail(configuration.ErrConflict)
	}
	publisher, err := p.trust(approved.publisher.Namespace)
	if err != nil {
		return fail(err)
	}
	if publisher != approved.publisher {
		return fail(modulepackage.ErrUntrustedPublisher)
	}
	metadata := approved.view.Metadata
	reference := modulepackage.Dependency{Kind: "runtime", ID: metadata.PackageID(), Version: metadata.Version, ManifestSHA256: approved.view.ManifestSHA256}
	if removed, err := runtimes.Remove(p.ctx, reference, source, publisher); err != nil {
		return fail(err)
	} else if !removed {
		return fail(configuration.ErrConflict)
	}
	warning := ""
	if err = runtimes.Reclaim(p.ctx, reference, source, publisher); err != nil {
		warning = "共享依赖已卸载，文件等待清理：" + err.Error()
	}
	raw, _ := json.Marshal(map[string]any{"state": "completed", "kind": "runtime", "moduleId": reference.ID, "warning": warning})
	return host.Response{Result: raw}
}

func requireUnusedPackage(ctx context.Context, source *modulepackage.DependencySource, record modulepackage.RegisteredPackage, kind string) error {
	references, err := source.References(ctx, modulepackage.Dependency{Kind: kind, ID: record.ModuleID, Version: record.Version, ManifestSHA256: record.ManifestSHA256})
	if err != nil {
		return err
	}
	if len(references) > 0 {
		return fmt.Errorf("%s %s 仍被 %s %s 使用，请先处理依赖它的模块", record.ModuleID, record.Version, references[0].ID, references[0].Version)
	}
	return nil
}

func (p *installedPackages) installSharedRuntime(approved packagePreview, registry *modulepackage.Registry, publisher modulepackage.Publisher) host.Response {
	fail := func(err error) host.Response {
		return host.Response{Error: &host.RPCError{Code: "runtime_install", Message: err.Error()}}
	}
	runtimes, source, err := p.dependencyStores(registry)
	if err != nil {
		return fail(err)
	}
	current, err := runtimes.Snapshot()
	if err != nil {
		return fail(err)
	}
	if current.Revision != approved.runtimeRevision {
		return fail(configuration.ErrConflict)
	}
	record, err := runtimes.Install(p.ctx, approved.path, modulepackage.RuntimeInstallOptions{Publisher: publisher, Environment: p.environment, ExpectedManifestSHA256: approved.view.ManifestSHA256, Lookup: source.Lookup})
	if err != nil {
		return fail(err)
	}
	if record.ModuleID != approved.view.Metadata.PackageID() {
		return fail(errors.New("共享依赖登记与安装预览不匹配"))
	}
	raw, _ := json.Marshal(map[string]any{"state": "completed", "kind": "runtime", "moduleId": record.ModuleID, "version": record.Version})
	return host.Response{Result: raw}
}

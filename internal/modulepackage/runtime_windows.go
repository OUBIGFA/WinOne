//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
)

// RuntimeOptions are supplied by the Host, never by a package. Trust is queried
// again before use so removing a publisher does not leave an old key active.
type RuntimeOptions struct {
	State         *modulestate.Store
	Control       *modules.Control
	Service       *host.Service
	Environment   Environment
	Trust         func(namespace string) (Publisher, error)
	Pipe          string
	DataDirectory string
	ActionGuard   func() (func(), bool)
	Change        *ChangePermit
	Dependencies  *DependencySource
	HostPipe      string
}

// RuntimeModule retains only metadata until an explicit command needs a worker.
// Close is also used during Host shutdown; it preserves the user's enable intent.
type RuntimeModule struct {
	mu                    sync.Mutex
	ctx                   context.Context
	registry              *Registry
	options               RuntimeOptions
	id, digest, directory string
	publisher             Publisher
	manifest              *VerifiedManifest
	files                 *runtimeFiles
	dependencies          *DependencyLease
	worker                *modules.Lazy
	gate                  *modules.Gate
	closing               bool
	unloaded              bool
	bound                 bool
}

// LoadModule attaches an already prepared package to an existing Host. Signature,
// files, compatibility and the live intent binding must all match before a route
// is published. Configured activation and shortcut registration need their own
// Host adapters; unsupported declarations are rejected rather than ignored.
func (r *Registry) LoadModule(ctx context.Context, id string, options RuntimeOptions) (*RuntimeModule, error) {
	if options.Control == nil || options.Service == nil {
		return nil, errors.New("module runtime options are incomplete")
	}
	runtime, err := r.PrepareRuntime(ctx, id, options)
	if err != nil {
		return nil, err
	}
	if err = runtime.Bind(options.Control, options.Service); err != nil {
		return nil, err
	}
	return runtime, nil
}

// PrepareRuntime verifies and creates a dormant lifecycle gate before the Host
// configuration opens. Bind publishes its route only after configuration exists,
// avoiding startup recovery against a catalog that lacks installed modules.
func (r *Registry) PrepareRuntime(ctx context.Context, id string, options RuntimeOptions) (*RuntimeModule, error) {
	if ctx == nil || options.State == nil || options.Trust == nil || options.ActionGuard == nil {
		return nil, errors.New("module runtime options are incomplete")
	}
	if !catalog.ValidOptionalID(id) || !filepath.IsAbs(options.DataDirectory) || strings.ContainsRune(options.DataDirectory, '\x00') {
		return nil, errors.New("module runtime identity or data directory is invalid")
	}
	// Use the same full pipe-name limit as the desktop/Host launch contract.
	if !strings.HasPrefix(options.Pipe, `\\.\pipe\WinOne-`) || strings.ContainsRune(options.Pipe, '\x00') || len(options.Pipe) > 256 {
		return nil, errors.New("module runtime pipe is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	namespace, _, _ := strings.Cut(id, ".")
	publisher, err := options.Trust(namespace)
	if err != nil {
		return nil, err
	}
	if options.Change != nil {
		permit := *options.Change
		options.Change = &permit
	}
	stage, err := r.resolve(ctx, id, publisher, options.Environment, options.Change)
	if err != nil {
		return nil, err
	}
	metadata := stage.Manifest().Metadata()
	if metadata.PackageKind() != "module" {
		return nil, errors.New("only module packages have an executable runtime")
	}
	var launchArgs []string
	if len(metadata.Dependencies) > 0 {
		if options.Dependencies == nil || !strings.HasPrefix(options.HostPipe, `\\.\pipe\WinOne-`) || strings.ContainsRune(options.HostPipe, '\x00') || len(options.HostPipe) > 256 {
			return nil, errors.New("module dependency resolution requires Host-owned stores and a Host pipe")
		}
		if err = options.Dependencies.checkAdmission(ctx, stage.Manifest(), r.directory); err != nil {
			return nil, err
		}
		// Only this worker receives its fixed per-generation context. Legacy
		// workers keep their original command line and launch contract.
		launchArgs = []string{"--dependency-context", filepath.Join(options.DataDirectory, "dependencies.json")}
	}
	// Worker logs and settings must remain outside the signed, read-only tree.
	// Different volumes are valid; only this package and its descendants fail.
	if relative, err := filepath.Rel(stage.Directory(), options.DataDirectory); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("module data directory must be outside the package")
	}
	manifest := stage.Manifest().Metadata().Module
	if manifest.Activation != "on-demand" || len(manifest.ShortcutActions) != 0 {
		return nil, errors.New("module activation requires an unregistered Host adapter")
	}
	runtime := &RuntimeModule{ctx: ctx, registry: r, options: options, id: id, digest: stage.Manifest().Digest(), directory: stage.Directory(), publisher: publisher, manifest: stage.Manifest()}
	runtime.worker = modules.NewLazy(ctx, id, manifest.Name.Chinese, filepath.Join(stage.Directory(), filepath.FromSlash(manifest.Worker)), options.Pipe, options.DataDirectory, launchArgs...)
	gate, err := modules.NewPackageGate(manifest, runtime.digest, options.State, modules.LifecycleHooks{
		Start: runtime.Resume, Stop: runtime.Close, IntentChanged: func() error { return nil }, Status: runtime.worker.Status, Handle: runtime.handle,
	})
	if err != nil {
		return nil, err
	}
	gate.SetActionGuard(options.ActionGuard)
	runtime.gate = gate
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return runtime, nil
}

func (r *RuntimeModule) LifecycleGate() *modules.Gate { return r.gate }

func (r *RuntimeModule) Bind(control *modules.Control, service *host.Service) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bound || r.unloaded || control == nil || service == nil || r.options.Change != nil && r.options.Change.RecoveryOnly {
		return errors.New("module runtime binding is invalid")
	}
	if err := control.RegisterPackage(service, r.manifest.Metadata().Module, r.gate); err != nil {
		return err
	}
	r.options.Control = control
	r.options.Service = service
	r.bound = true
	return nil
}

func (r *RuntimeModule) currentTrust() error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	publisher, err := r.options.Trust(r.publisher.Namespace)
	if err != nil {
		return err
	}
	if publisher != r.publisher {
		return ErrUntrustedPublisher
	}
	return nil
}

// Recheck files before every new worker lifetime, including a restart after a
// crash. An earlier inventory entry is not permanent permission to run a path.
func (r *RuntimeModule) verify() error {
	if err := r.currentTrust(); err != nil {
		return err
	}
	stage, err := r.registry.resolve(r.ctx, r.id, r.publisher, r.options.Environment, r.options.Change)
	if err != nil {
		return err
	}
	if stage.Manifest().Digest() != r.digest || stage.Directory() != r.directory {
		return errors.New("loaded module package changed")
	}
	if r.options.State.Snapshot().Modules[r.id].PackageSHA256 != r.digest {
		return errors.New("module intent binding changed")
	}
	if len(r.manifest.metadata.Dependencies) > 0 {
		return r.options.Dependencies.checkAdmission(r.ctx, r.manifest, r.registry.directory)
	}
	return nil
}

func (r *RuntimeModule) handle(request host.Request) host.Response {
	r.mu.Lock()
	defer r.mu.Unlock()
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_package", Message: err.Error()}}
	}
	if r.closing {
		return fail(errors.New("module runtime is closing"))
	}
	if err := r.currentTrust(); err != nil {
		return fail(err)
	}
	if !r.worker.Running() {
		if err := r.prepareFiles(); err != nil {
			return fail(err)
		}
	} else {
		// A durable preparation also closes admission for an already running
		// worker, even if an interrupted caller has released its in-memory lock.
		record, err := r.registry.runtimeRecord(r.ctx, r.id, r.options.Change)
		if err != nil {
			return fail(err)
		}
		if record.ManifestSHA256 != r.digest || record.StageID != filepath.Base(r.directory) || record.Version != r.manifest.metadata.Version {
			return fail(errors.New("loaded module package changed"))
		}
		if r.dependencies != nil {
			if err := r.dependencies.Recheck(r.ctx); err != nil {
				return fail(err)
			}
		}
	}
	reply := r.worker.Handle(request)
	if !r.worker.Running() {
		if err := r.releaseFiles(); err != nil {
			if reply.Error != nil {
				err = errors.Join(errors.New(reply.Error.Message), err)
			}
			return fail(err)
		}
	}
	return reply
}

// Hold paths and bytes before the final verification, and keep the handles
// through the worker lifetime so an upgrade cannot replace checked code.
func (r *RuntimeModule) prepareFiles() error {
	if err := r.releaseFiles(); err != nil {
		return err
	}
	files, err := holdRuntimeFiles(r.ctx, r.directory, r.manifest)
	r.files = files
	if err == nil {
		err = r.verify()
	}
	if err == nil && len(r.manifest.metadata.Dependencies) > 0 {
		r.dependencies, err = r.options.Dependencies.Hold(r.ctx, r.manifest)
		if err == nil {
			_, err = r.dependencies.WriteContext(r.options.DataDirectory, r.options.HostPipe)
		}
	}
	if err != nil {
		return errors.Join(err, r.releaseFiles())
	}
	return nil
}

func (r *RuntimeModule) releaseFiles() error {
	var result error
	if r.dependencies != nil {
		if err := r.dependencies.Close(); err != nil {
			result = errors.Join(result, err)
		} else {
			r.dependencies = nil
		}
	}
	if r.files != nil {
		if err := r.files.Close(); err != nil {
			result = errors.Join(result, err)
		} else {
			r.files = nil
		}
	}
	return result
}

// Resume is used by enable and by an aborted Host shutdown. It never starts an
// on-demand worker; the next explicit request still performs launch validation.
func (r *RuntimeModule) Resume() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.unloaded {
		return errors.New("module runtime was unloaded")
	}
	if err := r.verify(); err != nil {
		return err
	}
	r.worker.Resume()
	r.closing = false
	return nil
}

func (r *RuntimeModule) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closing = true
	if err := r.worker.Close(); err != nil {
		r.closing = false
		return err
	}
	return r.releaseFiles()
}

func (r *RuntimeModule) Running() bool { return r.worker.Running() }

// CloseForRecovery also retires an old public gate after its stop succeeds.
// Failed stops retain the gate and worker so a later recovery can retry them.
func (r *RuntimeModule) CloseForRecovery() error {
	r.mu.Lock()
	bound, unloaded := r.bound, r.unloaded
	r.mu.Unlock()
	if bound && !unloaded {
		if err := r.gate.SetEnabled(r.options.State.Snapshot().Revision, false); err != nil {
			return err
		}
		return r.Unload()
	}
	return r.Close()
}

// Unload retires only a disabled runtime. Stop/restore must finish first; files
// and saved preferences are retained for uninstall, upgrade or later loading.
func (r *RuntimeModule) Unload() error {
	r.mu.Lock()
	if r.unloaded {
		r.mu.Unlock()
		return nil
	}
	if !r.bound {
		r.mu.Unlock()
		return errors.New("module runtime is not bound")
	}
	running := r.worker.Running()
	r.mu.Unlock()
	if running {
		return errors.New("module worker is still running")
	}
	// Do not hold the runtime mutex while the gate drains admitted operations.
	if err := r.options.Control.UnregisterPackage(r.id, r.gate); err != nil {
		return err
	}
	r.mu.Lock()
	r.unloaded = true
	r.closing = true
	r.mu.Unlock()
	return nil
}

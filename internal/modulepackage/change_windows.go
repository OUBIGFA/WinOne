//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"winone/internal/maintenance"
	"winone/internal/modulestate"
	"winone/internal/settings"
)

// These methods use the Host's live state store. The product coordinator must
// serialize configuration admission, stop/unload runtimes, and validate the
// candidate before FinishChange. They do not delete files or run worker code.
func (r *Registry) withRegistryLease(ctx context.Context, apply func(RegistrySnapshot) (RegistrySnapshot, error)) (snapshot RegistrySnapshot, err error) {
	return r.lockedRegistry(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
		if err := r.finishPreparations(ctx, current, ""); err != nil {
			return current, errors.Join(ErrReclamationPending, err)
		}
		if err := r.syncReclamation(ctx, current.LastChange); err != nil {
			return current, errors.Join(ErrReclamationPending, err)
		}
		next, err := apply(current)
		if err != nil {
			return next, err
		}
		if err = r.syncReclamation(ctx, next.LastChange); err != nil {
			return next, errors.Join(ErrReclamationPending, err)
		}
		if err = r.finishPreparations(ctx, next, ""); err != nil {
			return next, errors.Join(ErrReclamationPending, err)
		}
		return next, nil
	})
}

// Reclaim uses this lower-level lease so a full queue can be drained even when
// a completed package operation could not append its new cleanup record.
func (r *Registry) lockedRegistry(ctx context.Context, apply func(RegistrySnapshot) (RegistrySnapshot, error)) (snapshot RegistrySnapshot, err error) {
	if err = ctx.Err(); err != nil {
		return snapshot, err
	}
	coordination := r.coordination
	if coordination == "" {
		coordination = r.directory
	}
	lease, err := maintenance.AcquireMaintenance(coordination)
	if err != nil {
		return snapshot, err
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	snapshot, err = r.read(ctx)
	if err != nil {
		return snapshot, err
	}
	return apply(snapshot)
}

func (r *Registry) approvedRecord(ctx context.Context, id string, options StageOptions) (RegisteredPackage, error) {
	if !strings.EqualFold(filepath.Clean(options.Directory), r.directory) {
		return RegisteredPackage{}, errors.New("stage belongs to a different store")
	}
	stage, err := VerifyStage(ctx, id, options)
	if err != nil {
		return RegisteredPackage{}, err
	}
	metadata := stage.Manifest().Metadata()
	if metadata.PackageKind() != "module" {
		return RegisteredPackage{}, errors.New("replacement requires a dependency-aware installation coordinator")
	}
	if err = checkModuleDependencies(ctx, stage.Manifest(), options); err != nil {
		return RegisteredPackage{}, err
	}
	return RegisteredPackage{ModuleID: metadata.Module.ID, Version: metadata.Version, StageID: id, ManifestSHA256: stage.Manifest().Digest()}, nil
}

func (r *Registry) approveTarget(ctx context.Context, target *RegisteredPackage, options *StageOptions) error {
	if target == nil {
		return nil
	}
	if options == nil {
		return errors.New("current package trust and compatibility approval are required")
	}
	actual, err := r.approvedRecord(ctx, target.StageID, *options)
	if err != nil {
		return err
	}
	if actual != *target {
		return ErrApprovalChanged
	}
	return nil
}

func (r *Registry) matchesRequest(change PackageChange, request ChangeRequest) bool {
	if change.ID != request.ID || change.RegistryRevision != request.RegistryRevision || change.StateRevision != request.StateRevision || change.Before != request.Current || change.MigrationSHA256 != request.MigrationSHA256 {
		return false
	}
	if request.Replacement == nil {
		return change.After == nil
	}
	return change.After != nil && change.After.StageID == request.Replacement.ID && change.After.ManifestSHA256 == strings.ToLower(request.Replacement.Options.ExpectedManifestSHA256) && strings.EqualFold(filepath.Clean(request.Replacement.Options.Directory), r.directory)
}

// BeginChange durably blocks the affected module before any lifecycle changes.
// An exact retry returns its pending state or receipt, never a new operation.
func (r *Registry) BeginChange(ctx context.Context, request ChangeRequest, state *modulestate.Store) (RegistrySnapshot, error) {
	if state == nil {
		return RegistrySnapshot{}, errors.New("module state store is required")
	}
	return r.withRegistryLease(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
		if current.Pending != nil && current.Pending.ID == request.ID {
			if r.matchesRequest(*current.Pending, request) {
				return current, nil
			}
			return current, settings.ErrConflict
		}
		if current.LastChange != nil && current.LastChange.Change.ID == request.ID {
			if r.matchesRequest(current.LastChange.Change, request) {
				return current, nil
			}
			return current, settings.ErrConflict
		}
		if current.Pending != nil {
			return current, ErrRecoveryRequired
		}
		intent := state.Snapshot()
		if current.Revision != request.RegistryRevision || intent.Revision != request.StateRevision || current.Records[request.Current.ModuleID] != request.Current {
			return current, settings.ErrConflict
		}
		// Reserve room for prepare, apply, restore and finish without wrapping.
		if current.Revision > ^uint64(0)-4 || intent.Revision > ^uint64(0)-8 {
			return current, errors.New("package change revisions are exhausted")
		}
		if err := r.checkChangeReferences(ctx, current, request); err != nil {
			return current, err
		}
		change := PackageChange{ID: request.ID, Phase: "prepared", RegistryRevision: current.Revision, StateRevision: intent.Revision, Before: request.Current, PreviousState: intent.Modules[request.Current.ModuleID]}
		change.MigrationSHA256 = request.MigrationSHA256
		if request.Replacement != nil {
			after, err := r.approvedRecord(ctx, request.Replacement.ID, request.Replacement.Options)
			if err != nil {
				return current, err
			}
			change.After = &after
		}
		if err := validateChange(change); err != nil {
			return current, err
		}
		next, err := nextRegistry(current)
		if err != nil {
			return current, err
		}
		next.Pending = &change
		if change.MigrationSHA256 != "" {
			next.SchemaVersion = 3
		}
		if err = r.writeRegistry(ctx, next); err != nil {
			return current, err
		}
		return next, nil
	})
}

func pendingChange(snapshot RegistrySnapshot, id string) (*PackageChange, error) {
	if snapshot.Pending != nil && snapshot.Pending.ID == id {
		return snapshot.Pending, nil
	}
	if snapshot.LastChange != nil && snapshot.LastChange.Change.ID == id {
		return nil, nil
	}
	return nil, settings.ErrConflict
}

func changeTarget(change *PackageChange, phase string) (*RegisteredPackage, string, bool) {
	if phase == "restoring" {
		return &change.Before, change.PreviousState.PackageSHA256, change.PreviousState.Enabled
	}
	if change.After == nil {
		return nil, "", false
	}
	return change.After, change.After.ManifestSHA256, change.PreviousState.Enabled
}

// ApplyChange switches only stopped bindings. The applied journal remains until
// candidate validation and restoration of the previous enable choice finish.
func (r *Registry) ApplyChange(ctx context.Context, id string, state *modulestate.Store, approval *StageOptions) (RegistrySnapshot, error) {
	return r.transitionChange(ctx, id, state, approval, "applied")
}

// RestoreChange keeps the old files intact and verifies them again. Revoked or
// damaged rollback code is not silently enabled to make recovery look successful.
func (r *Registry) RestoreChange(ctx context.Context, id string, state *modulestate.Store, approval StageOptions) (RegistrySnapshot, error) {
	return r.transitionChange(ctx, id, state, &approval, "restoring")
}

func (r *Registry) transitionChange(ctx context.Context, id string, state *modulestate.Store, approval *StageOptions, phase string) (RegistrySnapshot, error) {
	if state == nil {
		return RegistrySnapshot{}, errors.New("module state store is required")
	}
	return r.withRegistryLease(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
		change, err := pendingChange(current, id)
		if err != nil || change == nil {
			return current, err
		}
		if phase == "applied" && change.Phase != "prepared" && change.Phase != "applied" {
			return current, ErrRecoveryRequired
		}
		target, digest, _ := changeTarget(change, phase)
		if err = r.approveTarget(ctx, target, approval); err != nil {
			return current, err
		}
		record := state.Snapshot().Modules[change.Before.ModuleID]
		if change.Phase == phase && !record.PendingStop && record.PackageSHA256 == digest {
			return current, nil
		}
		if record.Enabled || record.PendingStop {
			return current, errors.New("module must be stopped and restored before changing its package")
		}
		allowed := record.PackageSHA256 == change.PreviousState.PackageSHA256 || record.PackageSHA256 == change.Before.ManifestSHA256 || record.PackageSHA256 == digest || (change.After != nil && record.PackageSHA256 == change.After.ManifestSHA256)
		// Removal's applied binding is empty. It is a legitimate rollback
		// source only after the durable registry has entered apply/restore.
		allowed = allowed || (change.After == nil && change.Phase != "prepared" && record.PackageSHA256 == "")
		if !allowed {
			return current, settings.ErrConflict
		}
		next := current
		if change.Phase != phase {
			next, err = nextRegistry(current)
			if err != nil {
				return current, err
			}
			if target == nil {
				delete(next.Records, change.Before.ModuleID)
			} else {
				next.Records[change.Before.ModuleID] = *target
			}
			next.Pending.Phase = phase
			// Persist the direction before touching the separate intent file.
			// A crash must never lose the fact that rollback was requested.
			if err = r.writeRegistry(ctx, next); err != nil {
				return current, err
			}
		}
		if err = ctx.Err(); err != nil {
			return next, err
		}
		if _, err = state.ReplacePackageBinding(change.Before.ModuleID, record.PackageSHA256, digest); err != nil {
			return next, err
		}
		return next, nil
	})
}

// FinishChange commits a checked candidate or a checked restoration. It never
// invents worker-health evidence; that belongs to the product coordinator.
func (r *Registry) FinishChange(ctx context.Context, id string, state *modulestate.Store, approval *StageOptions) (RegistrySnapshot, error) {
	if state == nil {
		return RegistrySnapshot{}, errors.New("module state store is required")
	}
	return r.withRegistryLease(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
		change, err := pendingChange(current, id)
		if err != nil || change == nil {
			return current, err
		}
		if change.Phase == "prepared" {
			return current, ErrRecoveryRequired
		}
		target, digest, enabled := changeTarget(change, change.Phase)
		if err = r.approveTarget(ctx, target, approval); err != nil {
			return current, err
		}
		record := state.Snapshot().Modules[change.Before.ModuleID]
		if record.PendingStop || record.PackageSHA256 != digest || record.Enabled != enabled {
			return current, errors.New("module enable preference has not been restored")
		}
		outcome := "committed"
		if change.Phase == "restoring" {
			outcome = "restored"
		}
		return r.finishRegistryChange(ctx, current, *change, outcome)
	})
}

// CancelChange is for a preparation that has not altered module intent. After a
// stop or binding write, callers must restore instead of discarding the journal.
func (r *Registry) CancelChange(ctx context.Context, id string, state *modulestate.Store) (RegistrySnapshot, error) {
	if state == nil {
		return RegistrySnapshot{}, errors.New("module state store is required")
	}
	return r.withRegistryLease(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
		change, err := pendingChange(current, id)
		if err != nil || change == nil {
			return current, err
		}
		if change.Phase != "prepared" || state.Snapshot().Modules[change.Before.ModuleID] != change.PreviousState {
			return current, ErrRecoveryRequired
		}
		return r.finishRegistryChange(ctx, current, *change, "cancelled")
	})
}

func (r *Registry) finishRegistryChange(ctx context.Context, current RegistrySnapshot, change PackageChange, outcome string) (RegistrySnapshot, error) {
	next, err := nextRegistry(current)
	if err != nil {
		return current, err
	}
	next.Pending = nil
	next.LastChange = &ChangeReceipt{Change: change, Outcome: outcome, Revision: next.Revision}
	if err = r.writeRegistry(ctx, next); err != nil {
		return current, err
	}
	return next, nil
}

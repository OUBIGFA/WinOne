//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"winone/internal/catalog"
)

// StageArchive retains preparation ownership before writing executable bytes.
// A failed or interrupted stage is visible to cleanup even when registration
// never began. The ordinary signed-directory checks still govern recycling.
func (r *Registry) StageArchive(ctx context.Context, filename string, options StageOptions, operation string) (*StagedPackage, error) {
	if !catalog.ValidID(operation) || len(operation) > 128 || !strings.EqualFold(filepath.Clean(options.Directory), r.directory) {
		return nil, errors.New("invalid package preparation identity")
	}
	options.onCreated = func(receipt StageReceipt) error {
		_, err := r.lockedRegistry(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
			ledger, err := r.Reclamation()
			if err != nil {
				return current, err
			}
			item := ReclamationItem{Kind: "preparation", Package: RegisteredPackage{ModuleID: receipt.ModuleID, Version: receipt.Version, StageID: receipt.StageID, ManifestSHA256: receipt.ManifestSHA256}, ChangeID: operation, Phase: "queued"}
			key := reclaimKey(item)
			if stageReferenced(current, receipt.StageID) {
				return current, errors.New("preparation stage is already referenced")
			}
			if _, exists := ledger.Items[key]; exists {
				return current, errors.New("preparation stage was already recorded")
			}
			ledger, err = nextReclamation(ledger)
			if err != nil {
				return current, err
			}
			ledger.SchemaVersion = 3
			ledger.Items[key] = item
			return current, r.writeReclamation(ctx, ledger)
		})
		return err
	}
	return StageArchive(ctx, filename, options)
}

// BindPreparationMigration runs before creating the backup. Its predicted
// digest remains useful if saving the backup or starting the transaction fails.
func (r *Registry) BindPreparationMigration(ctx context.Context, stageID, operation, digest string) error {
	if !stageIDPattern.MatchString(stageID) || !validMigrationDigest(digest) {
		return errors.New("invalid preparation migration reference")
	}
	_, err := r.lockedRegistry(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
		ledger, err := r.Reclamation()
		if err != nil {
			return current, err
		}
		key := reclaimKey(ReclamationItem{Package: RegisteredPackage{StageID: stageID}})
		item, exists := ledger.Items[key]
		if !exists || item.Kind != "preparation" || item.ChangeID != operation || item.Phase != "queued" || stageReferenced(current, stageID) {
			return current, errors.New("package preparation has changed")
		}
		if item.MigrationSHA256 == digest {
			return current, nil
		}
		if item.MigrationSHA256 != "" {
			return current, errors.New("preparation migration digest changed")
		}
		ledger, err = nextReclamation(ledger)
		if err != nil {
			return current, err
		}
		item.MigrationSHA256 = digest
		ledger.Items[key] = item
		return current, r.writeReclamation(ctx, ledger)
	})
	return err
}

// BindRecoveredPreparationMigration links a previously untracked backup only
// after the Host checked the signed migration and current old configuration.
// Only a legacy-adopted stage may change its synthetic operation identity.
func (r *Registry) BindRecoveredPreparationMigration(ctx context.Context, stageID, operation, digest, beforePackage string) error {
	if !stageIDPattern.MatchString(stageID) || !catalog.ValidID(operation) || len(operation) > 128 || !validMigrationDigest(digest) || !validMigrationDigest(beforePackage) {
		return errors.New("invalid recovered migration identity")
	}
	_, err := r.lockedRegistry(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
		if current.Pending != nil {
			return current, ErrRecoveryRequired
		}
		ledger, err := r.Reclamation()
		if err != nil {
			return current, err
		}
		key := reclaimKey(ReclamationItem{Package: RegisteredPackage{StageID: stageID}})
		item, exists := ledger.Items[key]
		if !exists || item.Kind != "preparation" || item.Phase != "queued" || stageReferenced(current, stageID) || current.Records[item.Package.ModuleID].ManifestSHA256 != beforePackage {
			return current, errors.New("recovered migration package state changed")
		}
		if item.ChangeID == operation && item.MigrationSHA256 == digest {
			return current, nil
		}
		if item.ChangeID != "legacy-"+stageID || item.MigrationSHA256 != "" {
			return current, errors.New("preparation already belongs to another operation")
		}
		for _, other := range ledger.Items {
			if other.ChangeID == operation && other.MigrationSHA256 != "" {
				return current, errors.New("migration backup already has a cleanup owner")
			}
		}
		ledger, err = nextReclamation(ledger)
		if err != nil {
			return current, err
		}
		item.ChangeID = operation
		item.MigrationSHA256 = digest
		ledger.Items[key] = item
		return current, r.writeReclamation(ctx, ledger)
	})
	return err
}

// FinishPreparation discards only bookkeeping now owned by durable inventory
// or a package transaction. An unregistered stage stays queued for explicit
// cleanup; this method never removes payload or recovery files.
func (r *Registry) FinishPreparation(ctx context.Context, stageID string) error {
	if !stageIDPattern.MatchString(stageID) {
		return errors.New("invalid preparation stage")
	}
	_, err := r.lockedRegistry(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
		return current, r.finishPreparations(ctx, current, stageID)
	})
	return err
}

func (r *Registry) finishPreparations(ctx context.Context, current RegistrySnapshot, only string) error {
	ledger, err := r.Reclamation()
	if err != nil {
		return err
	}
	var keys []string
	for key, item := range ledger.Items {
		if item.Kind == "preparation" && item.Phase == "queued" && (only == "" || item.Package.StageID == only) && stageReferenced(current, item.Package.StageID) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	ledger, err = nextReclamation(ledger)
	if err != nil {
		return err
	}
	for _, key := range keys {
		delete(ledger.Items, key)
	}
	return r.writeReclamation(ctx, ledger)
}

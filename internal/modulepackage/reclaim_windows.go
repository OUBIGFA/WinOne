//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"path/filepath"
	"winone/internal/recycle"
)

func stageReferenced(snapshot RegistrySnapshot, id string) bool {
	for _, record := range snapshot.Records {
		if record.StageID == id {
			return true
		}
	}
	if pending := snapshot.Pending; pending != nil {
		if pending.Before.StageID == id || (pending.After != nil && pending.After.StageID == id) {
			return true
		}
	}
	return false
}

// Reclaim processes one ended operation's package. Publisher is a known key for
// validating retired content, not permission to run it. Missing keys or changed
// files leave the directory intact. No permanent-delete fallback is provided.
func (r *Registry) Reclaim(ctx context.Context, key string, publisher Publisher) (result ReclaimResult, err error) {
	return r.reclaim(ctx, key, publisher, nil)
}

// Shared runtime cleanup adds a reference check under the same package lease
// that excludes a concurrent module registration.
func (r *Registry) reclaim(ctx context.Context, key string, publisher Publisher, references func(RegisteredPackage) error) (result ReclaimResult, err error) {
	_, err = r.lockedRegistry(ctx, func(current RegistrySnapshot) (snapshot RegistrySnapshot, operationErr error) {
		ledger, err := r.Reclamation()
		if err != nil {
			return current, err
		}
		item, exists := ledger.Items[key]
		if !exists {
			if ledger.LastResult != nil && ledger.LastResult.ID == key {
				result = *ledger.LastResult
				return current, nil
			}
			return current, errors.New("reclaim work is not pending")
		}
		if references != nil {
			if err = references(item.Package); err != nil {
				return current, err
			}
		}
		parents, err := holdRuntimeParents(r.directory)
		if parents != nil {
			defer func() { operationErr = errors.Join(operationErr, parents.Close()) }()
		}
		if err != nil {
			return current, err
		}
		source := filepath.Join(r.directory, item.Package.StageID)
		quarantine := filepath.Join(r.directory, quarantineName(key))
		result = ReclaimResult{ID: key, Path: source}
		quarantined, err := ordinaryDirectoryExists(quarantine)
		if err != nil {
			return current, err
		}
		if item.Phase == "queued" && !quarantined {
			if stageReferenced(current, item.Package.StageID) {
				result.State = "retained-reference"
				return current, r.completeReclamation(ctx, ledger, key, result)
			}
			exists, err := ordinaryDirectoryExists(source)
			if err != nil {
				return current, err
			}
			if !exists {
				return current, errors.New("reclaim source is missing without a recycle receipt")
			}
			if err = moveReclaimDirectory(source, quarantine, func() error { return verifyReclaimDirectory(ctx, source, item.Package, publisher) }); err != nil {
				return current, err
			}
			quarantined = true
		}
		result.Path = quarantine
		var receipt recycle.Receipt
		var recycleErr error
		if quarantined {
			if err = verifyReclaimDirectory(ctx, quarantine, item.Package, publisher); err != nil {
				return current, err
			}
			if item.Phase != "recycling" {
				ledger, err = nextReclamation(ledger)
				if err != nil {
					return current, err
				}
				item.Phase = "recycling"
				item.Origin = quarantine
				ledger.Items[key] = item
				if err = r.writeReclamation(ctx, ledger); err != nil {
					return current, err
				}
			}
			if item.Origin != quarantine {
				return current, errors.New("reclaim location changed; inspect the preserved directory")
			}
			receipt, recycleErr = recycle.CheckedDirectory(ctx, quarantine, func() error { return verifyReclaimDirectory(ctx, quarantine, item.Package, publisher) })
		} else {
			if item.Phase != "recycling" {
				return current, errors.New("quarantine is missing before recycling was recorded")
			}
			receipt, recycleErr = recycle.FindDirectory(ctx, item.Origin)
		}
		if !receipt.Recycled {
			return current, errors.Join(errors.New("package recycling could not be confirmed"), recycleErr)
		}
		if err = verifyReclaimDirectory(ctx, receipt.Path, item.Package, publisher); err != nil {
			return current, errors.Join(recycleErr, err)
		}
		result = ReclaimResult{ID: key, State: "recycled", Path: receipt.Path}
		return current, errors.Join(recycleErr, r.completeReclamation(ctx, ledger, key, result))
	})
	return result, err
}

func (r *Registry) completeReclamation(ctx context.Context, ledger ReclamationSnapshot, key string, result ReclaimResult) error {
	next, err := nextReclamation(ledger)
	if err != nil {
		return err
	}
	delete(next.Items, key)
	next.LastResult = &result
	return r.writeReclamation(ctx, next)
}

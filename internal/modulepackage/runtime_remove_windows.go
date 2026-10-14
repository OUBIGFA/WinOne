//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"winone/internal/maintenance"
)

func (s *RuntimeStore) checkSource(source *DependencySource) error {
	if source == nil || source.runtimes != s || source.modules.directory != s.packagesDirectory {
		return errors.New("runtime removal requires the owning Host reference inventory")
	}
	return nil
}

func requireUnreferenced(ctx context.Context, source *DependencySource, target Dependency) error {
	references, err := source.References(ctx, target)
	if err != nil {
		return err
	}
	if len(references) > 0 {
		return fmt.Errorf("%s@%s is still required by %s@%s", target.ID, target.Version, references[0].ID, references[0].Version)
	}
	return nil
}

// Remove durably unregisters an unused immutable version and records its files
// for checked recycling. A failed recycle never makes the index claim that the
// runtime is still installed, and a failed index write preserves installation.
func (s *RuntimeStore) Remove(ctx context.Context, target Dependency, source *DependencySource, publisher Publisher) (removed bool, err error) {
	if err = target.Validate(); err != nil {
		return false, err
	}
	if target.Kind != "runtime" {
		return false, errors.New("runtime removal requires a runtime identity")
	}
	if err = s.checkSource(source); err != nil {
		return false, err
	}
	lease, err := maintenance.AcquireMaintenance(s.packagesDirectory)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	current, err := s.read(ctx)
	if err != nil {
		return false, err
	}
	record, exists := current.Records[target.ManifestSHA256]
	if !exists {
		return false, nil
	}
	if record.ModuleID != target.ID || record.Version != target.Version {
		return false, errors.New("runtime removal identity changed")
	}
	if err = requireUnreferenced(ctx, source, target); err != nil {
		return false, err
	}
	if _, err = s.Lookup(ctx, target, publisher, source.environment); err != nil {
		return false, err
	}
	ledger, err := s.journal.Reclamation()
	if err != nil {
		return false, err
	}
	item := ReclamationItem{Kind: "preparation", Package: record, ChangeID: "runtime-remove-" + record.ManifestSHA256[:16], Phase: "queued"}
	key := reclaimKey(item)
	if previous, exists := ledger.Items[key]; exists && (previous.Package != record || previous.Phase != "queued" || previous.MigrationSHA256 != "") {
		return false, errors.New("runtime cleanup ownership changed")
	}
	ledger, err = nextReclamation(ledger)
	if err != nil {
		return false, err
	}
	ledger.SchemaVersion = 3
	ledger.Items[key] = item
	if err = s.journal.writeReclamation(ctx, ledger); err != nil {
		return false, err
	}
	if current.Revision == ^uint64(0) {
		return false, errors.New("runtime registry revision exhausted")
	}
	next := current
	next.Revision++
	next.Records = maps.Clone(current.Records)
	delete(next.Records, target.ManifestSHA256)
	if err = s.write(ctx, next); err != nil {
		return false, err
	}
	return true, nil
}

// Reclaim rechecks both installed references and running file leases before
// using the shared no-permanent-deletion backend. It never removes a module.
func (s *RuntimeStore) Reclaim(ctx context.Context, target Dependency, source *DependencySource, publisher Publisher) error {
	if err := s.checkSource(source); err != nil {
		return err
	}
	if err := target.Validate(); err != nil {
		return err
	}
	if target.Kind != "runtime" {
		return errors.New("runtime cleanup requires a runtime identity")
	}
	ledger, err := s.journal.Reclamation()
	if err != nil {
		return err
	}
	for key, item := range ledger.Items {
		if item.Package.ManifestSHA256 != target.ManifestSHA256 {
			continue
		}
		if item.Package.ModuleID != target.ID || item.Package.Version != target.Version {
			return errors.New("runtime cleanup identity changed")
		}
		_, err = s.journal.reclaim(ctx, key, publisher, func(record RegisteredPackage) error { return requireUnreferenced(ctx, source, target) })
		return err
	}
	return nil
}

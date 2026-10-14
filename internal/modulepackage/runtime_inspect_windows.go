//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Inspect authenticates the small inventory for display. Launch and installation
// still use Lookup, which also verifies every payload file.
func (s *RuntimeStore) Inspect(ctx context.Context, reference Dependency, publisher Publisher) (*VerifiedManifest, error) {
	if err := reference.Validate(); err != nil {
		return nil, err
	}
	current, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	record, exists := current.Records[reference.ManifestSHA256]
	if reference.Kind != "runtime" || !exists || record.ModuleID != reference.ID || record.Version != reference.Version {
		return nil, errors.New("runtime inventory identity changed")
	}
	manifest, err := inspectStoredReferenceManifest(ctx, s.directory, record, publisher)
	if err == nil && manifest.metadata.PackageKind() != "runtime" {
		err = errors.New("runtime inventory contains a module package")
	}
	return manifest, err
}

// CleanupPending includes interrupted preparation without creating storage.
func (s *RuntimeStore) CleanupPending(ctx context.Context) (bool, error) {
	exists, err := ordinaryDirectoryExists(s.directory)
	if err != nil || !exists {
		return false, err
	}
	ledger, err := s.journal.Reclamation()
	if err != nil {
		return false, err
	}
	if len(ledger.Items) > 0 {
		return true, nil
	}
	stages, err := s.journal.UntrackedStages(ctx)
	return len(stages) > 0, err
}

// Cleanup shares the signed preparation recovery and recycle-only backend. An
// interrupted index commit first drops bookkeeping for still-installed stages.
func (s *RuntimeStore) Cleanup(ctx context.Context, source *DependencySource) error {
	if err := s.checkSource(source); err != nil {
		return err
	}
	exists, err := ordinaryDirectoryExists(s.directory)
	if err != nil || !exists {
		return err
	}
	if _, err = s.journal.lockedRegistry(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
		return current, s.journal.finishPreparations(ctx, current, "")
	}); err != nil {
		return err
	}
	recovery, err := s.journal.RecoverPreparations(ctx, source.trust)
	if err != nil {
		return err
	}
	ledger, err := s.journal.Reclamation()
	if err != nil {
		return err
	}
	var failures []error
	for _, issue := range recovery.Issues {
		failures = append(failures, fmt.Errorf("%s: %s", issue.StageID, issue.Reason))
	}
	keys := make([]string, 0, len(ledger.Items))
	for key := range ledger.Items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		record := ledger.Items[key].Package
		namespace, _, _ := strings.Cut(record.ModuleID, ".")
		publisher, err := source.trust(namespace)
		if err == nil {
			err = s.Reclaim(ctx, Dependency{Kind: "runtime", ID: record.ModuleID, Version: record.Version, ManifestSHA256: record.ManifestSHA256}, source, publisher)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", record.ModuleID, err))
		}
	}
	return errors.Join(failures...)
}

//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DependencySource uses the Host's fixed stores and trust policy. Package data
// cannot supply a resolver, filesystem root or alternate publisher key.
type DependencySource struct {
	modules     *Registry
	runtimes    *RuntimeStore
	environment Environment
	trust       func(string) (Publisher, error)
}

func NewDependencySource(modules *Registry, runtimes *RuntimeStore, environment Environment, trust func(string) (Publisher, error)) (*DependencySource, error) {
	if modules == nil || runtimes == nil || trust == nil || modules.inventory != nil || !strings.EqualFold(modules.directory, runtimes.packagesDirectory) {
		return nil, errors.New("dependency source requires matching Host-owned stores")
	}
	return &DependencySource{modules: modules, runtimes: runtimes, environment: environment, trust: trust}, nil
}

func (s *DependencySource) Resolve(ctx context.Context, root *VerifiedManifest) (DependencyLock, error) {
	return ResolveDependencies(ctx, root, s.environment, s.lookup)
}

// Lookup exposes only authenticated metadata to the installation graph; paths
// for execution remain available through the per-worker lease.
func (s *DependencySource) Lookup(ctx context.Context, reference Dependency) (*VerifiedManifest, error) {
	return s.lookup(ctx, reference)
}

func (s *DependencySource) lookup(ctx context.Context, reference Dependency) (*VerifiedManifest, error) {
	stage, err := s.stage(ctx, reference)
	if err != nil {
		return nil, err
	}
	return stage.Manifest(), nil
}

func (s *DependencySource) stage(ctx context.Context, reference Dependency) (*StagedPackage, error) {
	namespace, _, _ := strings.Cut(reference.ID, ".")
	publisher, err := s.trust(namespace)
	if err != nil {
		return nil, err
	}
	if reference.Kind == "runtime" {
		stage, err := s.runtimes.Lookup(ctx, reference, publisher, s.environment)
		if err != nil {
			return nil, err
		}
		return stage, nil
	}
	if reference.Kind != "module" {
		return nil, errors.New("dependency kind is unsupported")
	}
	stage, err := s.modules.Resolve(ctx, reference.ID, publisher, s.environment)
	if err != nil {
		return nil, err
	}
	metadata := stage.Manifest().Metadata()
	if metadata.PackageKind() != "module" || metadata.Version != reference.Version || stage.Manifest().Digest() != reference.ManifestSHA256 {
		return nil, errors.New("module dependency differs from its pinned version")
	}
	return stage, nil
}

func checkModuleDependencies(ctx context.Context, manifest *VerifiedManifest, options StageOptions) error {
	if len(manifest.metadata.Dependencies) == 0 {
		return nil
	}
	if options.Dependencies == nil {
		return errors.New("module dependencies have not been resolved by the Host")
	}
	return options.Dependencies.checkAdmission(ctx, manifest, options.Directory)
}

func (s *DependencySource) checkAdmission(ctx context.Context, manifest *VerifiedManifest, directory string) error {
	if s == nil || !strings.EqualFold(s.modules.directory, filepath.Clean(directory)) {
		return errors.New("dependency source belongs to a different package store")
	}
	_, err := s.Resolve(ctx, manifest)
	return err
}

// References authenticates declarations in active modules, pending old/new
// versions, and installed runtimes. Damaged ownership metadata blocks removal;
// it is never treated as an empty reference list. Retired module files alone do
// not keep installation references after their transaction has completed.
func (s *DependencySource) References(ctx context.Context, target Dependency) ([]Dependency, error) {
	result, err := s.ReferencesFor(ctx, []Dependency{target})
	return result[target], err
}

// ReferencesFor authenticates each installed declaration once per snapshot.
// Results are never cached across operations: removal must see current trust,
// pending transactions and newly installed consumers on every check.
func (s *DependencySource) ReferencesFor(ctx context.Context, targets []Dependency) (map[Dependency][]Dependency, error) {
	if len(targets) > maxRegisteredModules {
		return nil, errors.New("too many dependency reference targets")
	}
	result := make(map[Dependency][]Dependency, len(targets))
	for _, target := range targets {
		if err := target.Validate(); err != nil {
			return nil, err
		}
		if _, exists := result[target]; exists {
			return nil, errors.New("duplicate dependency reference target")
		}
		result[target] = []Dependency{}
	}
	if len(targets) == 0 {
		return result, ctx.Err()
	}
	modules, err := s.modules.read(ctx)
	if err != nil {
		return nil, err
	}
	runtimes, err := s.runtimes.read(ctx)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		directory string
		record    RegisteredPackage
		kind      string
	}
	var candidates []candidate
	for _, record := range modules.Records {
		candidates = append(candidates, candidate{s.modules.directory, record, "module"})
	}
	if pending := modules.Pending; pending != nil {
		candidates = append(candidates, candidate{s.modules.directory, pending.Before, "module"})
		if pending.After != nil {
			candidates = append(candidates, candidate{s.modules.directory, *pending.After, "module"})
		}
	}
	for _, record := range runtimes.Records {
		candidates = append(candidates, candidate{s.runtimes.directory, record, "runtime"})
	}
	seen := map[string]bool{}
	for _, entry := range candidates {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		identity := Dependency{Kind: entry.kind, ID: entry.record.ModuleID, Version: entry.record.Version, ManifestSHA256: entry.record.ManifestSHA256}
		key := entry.directory + "/" + entry.record.StageID
		if seen[key] || len(targets) == 1 && identity == targets[0] {
			continue
		}
		seen[key] = true
		namespace, _, _ := strings.Cut(identity.ID, ".")
		publisher, err := s.trust(namespace)
		if err != nil {
			return nil, fmt.Errorf("cannot inspect references from %s: %w", identity.ID, err)
		}
		manifest, err := inspectStoredReferenceManifest(ctx, entry.directory, entry.record, publisher)
		if err != nil {
			return nil, fmt.Errorf("cannot inspect references from %s: %w", identity.ID, err)
		}
		if manifest.metadata.PackageKind() != entry.kind {
			return nil, errors.New("reference inventory package kind changed")
		}
		for _, reference := range manifest.metadata.Dependencies {
			if _, wanted := result[reference]; wanted {
				result[reference] = append(result[reference], identity)
			}
		}
	}
	for _, references := range result {
		sort.Slice(references, func(i, j int) bool {
			if references[i].ID == references[j].ID {
				return references[i].ManifestSHA256 < references[j].ManifestSHA256
			}
			return references[i].ID < references[j].ID
		})
	}
	return result, nil
}

// Reference inspection needs the authenticated declarations, not module
// configuration or executable contents. The signed inventory remains bound to
// the registered digest even when an installed worker itself needs repair.
func inspectStoredReferenceManifest(ctx context.Context, directory string, record RegisteredPackage, publisher Publisher) (*VerifiedManifest, error) {
	if err := validateRegistration(record); err != nil {
		return nil, err
	}
	base, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	info, err := base.Lstat(record.StageID)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("reference stage is not an ordinary directory")
	}
	root, err := base.OpenRoot(record.StageID)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	raw, err := readStageFile(ctx, root, "module.json", MaxManifestBytes)
	if err != nil {
		return nil, err
	}
	signature, err := readStageFile(ctx, root, "module.json.minisig", MaxSignatureBytes)
	if err != nil {
		return nil, err
	}
	manifest, err := VerifyManifest(raw, signature, publisher)
	if err != nil {
		return nil, err
	}
	if manifest.Digest() != record.ManifestSHA256 || manifest.metadata.PackageID() != record.ModuleID || manifest.metadata.Version != record.Version {
		return nil, errors.New("reference manifest differs from inventory")
	}
	return manifest, nil
}

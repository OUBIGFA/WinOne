//go:build windows

package modulepackage

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/configjson"
	"winone/internal/maintenance"
	"winone/internal/settings"
)

const runtimeIndexName = ".winone-runtime-registry.json"

// RuntimeSnapshot indexes immutable versions by signed-manifest digest. A
// runtime has no module toggle, settings namespace or independently run Worker.
type RuntimeSnapshot struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Revision      uint64                       `json:"revision"`
	Records       map[string]RegisteredPackage `json:"records"`
}

type RuntimeStore struct {
	packagesDirectory string
	directory         string
	journal           *Registry
}

type RuntimeInstallOptions struct {
	Publisher              Publisher
	Environment            Environment
	ExpectedManifestSHA256 string
	Lookup                 func(context.Context, Dependency) (*VerifiedManifest, error)
}

func OpenRuntimeStore(packagesDirectory string) (*RuntimeStore, error) {
	if !filepath.IsAbs(packagesDirectory) {
		return nil, errors.New("runtime store requires an absolute package directory")
	}
	packagesDirectory = filepath.Clean(packagesDirectory)
	if _, err := ordinaryDirectoryExists(packagesDirectory); err != nil {
		return nil, err
	}
	store := &RuntimeStore{packagesDirectory: packagesDirectory, directory: filepath.Join(packagesDirectory, "shared-runtimes")}
	store.journal = &Registry{directory: store.directory, coordination: packagesDirectory, inventory: store.inventory}
	if _, err := store.Snapshot(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *RuntimeStore) Snapshot() (RuntimeSnapshot, error) { return s.read(context.Background()) }

func (s *RuntimeStore) read(ctx context.Context) (RuntimeSnapshot, error) {
	empty := RuntimeSnapshot{SchemaVersion: 1, Records: map[string]RegisteredPackage{}}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	exists, err := ordinaryDirectoryExists(s.directory)
	if err != nil || !exists {
		return empty, err
	}
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		return empty, err
	}
	defer root.Close()
	raw, err := readStageFile(ctx, root, runtimeIndexName, maxRegistryBytes)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	var value RuntimeSnapshot
	if err = configjson.DecodeObject(raw, &value); err != nil {
		return empty, err
	}
	return value, validateRuntimeSnapshot(value)
}

func validateRuntimeSnapshot(value RuntimeSnapshot) error {
	if value.SchemaVersion != 1 || value.Records == nil || len(value.Records) > maxRegisteredModules {
		return errors.New("runtime registry header or size is invalid")
	}
	versions, stages := map[string]bool{}, map[string]bool{}
	for digest, record := range value.Records {
		if err := validateRegistration(record); err != nil {
			return err
		}
		version := record.ModuleID + "@" + record.Version
		if digest != record.ManifestSHA256 || versions[version] || stages[record.StageID] {
			return errors.New("runtime registry contains duplicate or conflicting identities")
		}
		versions[version] = true
		stages[record.StageID] = true
	}
	return nil
}

func (s *RuntimeStore) inventory(ctx context.Context) (RegistrySnapshot, error) {
	value, err := s.read(ctx)
	return RegistrySnapshot{SchemaVersion: 2, Revision: value.Revision, Records: value.Records}, err
}

func (s *RuntimeStore) write(ctx context.Context, value RuntimeSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateRuntimeSnapshot(value); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) > maxRegistryBytes {
		return errors.New("runtime registry exceeds size limit")
	}
	return settings.WriteAtomic(filepath.Join(s.directory, runtimeIndexName), raw)
}

// Lookup revalidates the exact installed identity and all files. Displaying a
// snapshot is not permission to resolve a different version or execute code.
func (s *RuntimeStore) Lookup(ctx context.Context, reference Dependency, publisher Publisher, environment Environment) (*StagedPackage, error) {
	if err := reference.Validate(); err != nil {
		return nil, err
	}
	if reference.Kind != "runtime" {
		return nil, errors.New("runtime lookup requires a runtime identity")
	}
	value, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	record, exists := value.Records[reference.ManifestSHA256]
	if !exists || record.ModuleID != reference.ID || record.Version != reference.Version {
		return nil, errors.New("pinned runtime is not installed")
	}
	stage, err := VerifyStage(ctx, record.StageID, StageOptions{Directory: s.directory, Publisher: publisher, Environment: environment, ExpectedManifestSHA256: reference.ManifestSHA256})
	if err != nil {
		return nil, err
	}
	metadata := stage.Manifest().Metadata()
	if metadata.PackageKind() != "runtime" || metadata.PackageID() != reference.ID || metadata.Version != reference.Version {
		return nil, errors.New("runtime index differs from signed content")
	}
	return stage, nil
}

// Install keeps versions side by side and never starts runtime code. The
// package-directory lease is shared with module registration; dependencies are
// resolved again under that lease immediately before the index commit.
func (s *RuntimeStore) Install(ctx context.Context, file string, options RuntimeInstallOptions) (record RegisteredPackage, err error) {
	verified, err := VerifyArchive(ctx, file, options.Publisher)
	if err != nil {
		return record, err
	}
	metadata := verified.Manifest().Metadata()
	if metadata.PackageKind() != "runtime" {
		return record, errors.New("shared runtime installation requires a runtime package")
	}
	approval := StageOptions{Directory: s.directory, Publisher: options.Publisher, Environment: options.Environment, ExpectedManifestSHA256: options.ExpectedManifestSHA256}
	if err = validateStageOptions(approval); err != nil {
		return record, err
	}
	if err = verifyStageApproval(verified.Manifest(), approval); err != nil {
		return record, err
	}
	if _, err = ResolveDependencies(ctx, verified.Manifest(), options.Environment, options.Lookup); err != nil {
		return record, err
	}
	current, err := s.read(ctx)
	if err != nil {
		return record, err
	}
	if old, exists := current.Records[verified.Manifest().Digest()]; exists {
		reference := Dependency{Kind: "runtime", ID: metadata.PackageID(), Version: metadata.Version, ManifestSHA256: verified.Manifest().Digest()}
		if _, err = s.Lookup(ctx, reference, options.Publisher, options.Environment); err != nil {
			return record, err
		}
		return old, nil
	}
	for _, old := range current.Records {
		if old.ModuleID == metadata.PackageID() && old.Version == metadata.Version {
			return record, errors.New("this runtime version is already installed with different content")
		}
	}
	if err = os.MkdirAll(s.directory, 0700); err != nil {
		return record, err
	}
	stage, err := s.journal.StageArchive(ctx, file, approval, "runtime-install-"+strings.ToLower(rand.Text()))
	if stage != nil {
		defer func() {
			if cleanupErr := s.journal.FinishPreparation(ctx, filepath.Base(stage.Directory())); cleanupErr != nil {
				err = errors.Join(err, ErrReclamationPending, cleanupErr)
			}
		}()
	}
	if err != nil {
		return record, err
	}
	return s.register(ctx, filepath.Base(stage.Directory()), approval, options.Lookup)
}

func (s *RuntimeStore) register(ctx context.Context, stageID string, approval StageOptions, lookup func(context.Context, Dependency) (*VerifiedManifest, error)) (record RegisteredPackage, err error) {
	lease, err := maintenance.AcquireMaintenance(s.packagesDirectory)
	if err != nil {
		return record, err
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	current, err := s.read(ctx)
	if err != nil {
		return record, err
	}
	stage, err := VerifyStage(ctx, stageID, approval)
	if err != nil {
		return record, err
	}
	metadata := stage.Manifest().Metadata()
	if metadata.PackageKind() != "runtime" {
		return record, errors.New("runtime stage kind changed")
	}
	if _, err = ResolveDependencies(ctx, stage.Manifest(), approval.Environment, lookup); err != nil {
		return record, err
	}
	if old, exists := current.Records[stage.Manifest().Digest()]; exists {
		if _, err = s.Lookup(ctx, Dependency{Kind: "runtime", ID: metadata.PackageID(), Version: metadata.Version, ManifestSHA256: stage.Manifest().Digest()}, approval.Publisher, approval.Environment); err != nil {
			return record, err
		}
		return old, nil
	}
	for _, old := range current.Records {
		if old.ModuleID == metadata.PackageID() && old.Version == metadata.Version {
			return record, errors.New("runtime version content changed during installation")
		}
	}
	if current.Revision == ^uint64(0) {
		return record, errors.New("runtime registry revision exhausted")
	}
	next := current
	next.Revision++
	next.Records = maps.Clone(current.Records)
	record = RegisteredPackage{ModuleID: metadata.PackageID(), Version: metadata.Version, StageID: stageID, ManifestSHA256: stage.Manifest().Digest()}
	next.Records[record.ManifestSHA256] = record
	if err = s.write(ctx, next); err != nil {
		return RegisteredPackage{}, err
	}
	return record, nil
}

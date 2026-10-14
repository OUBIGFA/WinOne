//go:build windows

package modulepackage

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/distribution"
	"winone/internal/maintenance"
	"winone/internal/settings"
)

const registryFileName = ".winone-module-registry.json"
const maxRegistryBytes = 1 << 20
const maxRegisteredModules = 4096

// Registration identifies verified files only. Enable intent and runtime state
// remain in the configuration/lifecycle coordinator, not this inventory.
type RegisteredPackage struct {
	ModuleID       string `json:"moduleId"`
	Version        string `json:"version"`
	StageID        string `json:"stageId"`
	ManifestSHA256 string `json:"manifestSha256"`
}
type RegistrySnapshot struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Revision      uint64                       `json:"revision"`
	Records       map[string]RegisteredPackage `json:"records"`
	Pending       *PackageChange               `json:"pending,omitempty"`
	LastChange    *ChangeReceipt               `json:"lastChange,omitempty"`
}
type Registry struct {
	directory string
	// Shared runtime storage reuses preparation/reclamation with its own
	// immutable version index. Module transaction writes never use this view.
	coordination string
	inventory    func(context.Context) (RegistrySnapshot, error)
}

func OpenRegistry(directory string) (*Registry, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("module registry requires an absolute store directory")
	}
	result := &Registry{directory: filepath.Clean(directory)}
	if _, err := result.Snapshot(); err != nil {
		return nil, err
	}
	return result, nil
}
func (r *Registry) Snapshot() (RegistrySnapshot, error) { return r.read(context.Background()) }
func (r *Registry) read(ctx context.Context) (RegistrySnapshot, error) {
	if r.inventory != nil {
		return r.inventory(ctx)
	}
	root, err := os.OpenRoot(r.directory)
	if err != nil {
		return RegistrySnapshot{}, err
	}
	defer root.Close()
	data, err := readStageFile(ctx, root, registryFileName, maxRegistryBytes)
	if errors.Is(err, os.ErrNotExist) {
		return RegistrySnapshot{SchemaVersion: 2, Records: map[string]RegisteredPackage{}}, nil
	}
	if err != nil {
		return RegistrySnapshot{}, err
	}
	var result RegistrySnapshot
	if err = configjson.DecodeObject(data, &result); err != nil {
		return RegistrySnapshot{}, err
	}
	if err = validateRegistry(result); err != nil {
		return RegistrySnapshot{}, err
	}
	// Read legacy inventory without rewriting it; the next successful change
	// writes v2 so an older Host cannot overlook unfinished package work.
	if result.SchemaVersion < 2 {
		result.SchemaVersion = 2
	}
	return result, nil
}
func validateRegistry(snapshot RegistrySnapshot) error {
	if (snapshot.SchemaVersion != 1 && snapshot.SchemaVersion != 2 && snapshot.SchemaVersion != 3) || snapshot.Records == nil || len(snapshot.Records) > maxRegisteredModules {
		return errors.New("invalid module registry")
	}
	ids := make([]string, 0, len(snapshot.Records))
	stages := map[string]bool{}
	for id, record := range snapshot.Records {
		if record.ModuleID != id || stages[record.StageID] {
			return errors.New("invalid module registration")
		}
		if err := validateRegistration(record); err != nil {
			return err
		}
		ids = append(ids, id)
		stages[record.StageID] = true
	}
	sort.Strings(ids)
	for _, id := range ids {
		prefix := id + "."
		index := sort.SearchStrings(ids, prefix)
		if index < len(ids) && strings.HasPrefix(ids[index], prefix) {
			return errors.New("registered module namespaces overlap")
		}
	}
	return validateRegistryChange(snapshot)
}

func validateRegistration(record RegisteredPackage) error {
	digest, err := hex.DecodeString(record.ManifestSHA256)
	if !catalog.ValidOptionalID(record.ModuleID) || !stageIDPattern.MatchString(record.StageID) || err != nil || len(digest) != 32 || record.ManifestSHA256 != strings.ToLower(record.ManifestSHA256) {
		return errors.New("invalid module registration")
	}
	_, err = distribution.CompareVersions(record.Version, record.Version)
	return err
}

func nextRegistry(snapshot RegistrySnapshot) (RegistrySnapshot, error) {
	if snapshot.Revision == ^uint64(0) {
		return snapshot, errors.New("module registry revision exhausted")
	}
	if snapshot.SchemaVersion < 2 {
		snapshot.SchemaVersion = 2
	}
	snapshot.Revision++
	snapshot.Records = maps.Clone(snapshot.Records)
	if snapshot.Pending != nil {
		change := *snapshot.Pending
		snapshot.Pending = &change
	}
	return snapshot, nil
}

func (r *Registry) writeRegistry(ctx context.Context, next RegistrySnapshot) error {
	if r.inventory != nil {
		return errors.New("alternate inventories cannot write module transactions")
	}
	if err := validateRegistry(next); err != nil {
		return err
	}
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if len(data) > maxRegistryBytes {
		return errors.New("module registry exceeds size limit")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return settings.WriteAtomic(filepath.Join(r.directory, registryFileName), data)
}

// RegisterStage serializes cross-process writes with the existing maintenance
// lease. Retrying the same verified stage is idempotent, even after a lost reply.
// Replacements/upgrades are intentionally separate lifecycle transactions.
func (r *Registry) RegisterStage(ctx context.Context, id string, options StageOptions, expectedRevision uint64) (snapshot RegistrySnapshot, err error) {
	if err = validateStageOptions(options); err != nil {
		return snapshot, err
	}
	if !stageIDPattern.MatchString(id) || !strings.EqualFold(filepath.Clean(options.Directory), r.directory) {
		return snapshot, errors.New("stage belongs to a different store")
	}
	if err = ctx.Err(); err != nil {
		return snapshot, err
	}
	lease, err := maintenance.AcquireMaintenance(r.directory)
	if err != nil {
		return snapshot, err
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	snapshot, err = r.read(ctx)
	if err != nil {
		return snapshot, err
	}
	if snapshot.Pending != nil {
		return snapshot, ErrRecoveryRequired
	}
	if snapshot.Revision != expectedRevision {
		possibleRetry := false
		for _, record := range snapshot.Records {
			if record.StageID == id && record.ManifestSHA256 == strings.ToLower(options.ExpectedManifestSHA256) {
				possibleRetry = true
				break
			}
		}
		if !possibleRetry {
			return snapshot, settings.ErrConflict
		}
	}
	stage, err := VerifyStage(ctx, id, options)
	if err != nil {
		return snapshot, err
	}
	metadata := stage.manifest.metadata
	if metadata.PackageKind() != "module" {
		return snapshot, errors.New("package requires a dependency-aware installation coordinator")
	}
	if err = checkModuleDependencies(ctx, stage.Manifest(), options); err != nil {
		return snapshot, err
	}
	record := RegisteredPackage{ModuleID: metadata.Module.ID, Version: metadata.Version, StageID: id, ManifestSHA256: stage.manifest.Digest()}
	if existing, ok := snapshot.Records[record.ModuleID]; ok && existing == record {
		return snapshot, nil
	}
	if snapshot.Revision != expectedRevision {
		return snapshot, settings.ErrConflict
	}
	for existing := range snapshot.Records {
		if existing == record.ModuleID || strings.HasPrefix(existing, record.ModuleID+".") || strings.HasPrefix(record.ModuleID, existing+".") {
			return snapshot, errors.New("module is already registered or its namespace overlaps")
		}
	}
	next, err := nextRegistry(snapshot)
	if err != nil {
		return snapshot, err
	}
	next.Records[record.ModuleID] = record
	if err = r.writeRegistry(ctx, next); err != nil {
		return snapshot, err
	}
	return next, nil
}

// Resolve never treats an inventory entry as executable authorization. Current
// trust policy, compatibility and every file are checked again before use.
func (r *Registry) Resolve(ctx context.Context, id string, publisher Publisher, environment Environment) (*StagedPackage, error) {
	return r.resolve(ctx, id, publisher, environment, nil)
}

func (r *Registry) runtimeRecord(ctx context.Context, id string, permit *ChangePermit) (RegisteredPackage, error) {
	snapshot, err := r.read(ctx)
	if err != nil {
		return RegisteredPackage{}, err
	}
	if change := snapshot.Pending; change != nil && change.Before.ModuleID == id {
		if permit == nil || permit.ID != change.ID || permit.RegistryRevision != change.RegistryRevision || (change.Phase != "applied" && change.Phase != "restoring" && !(change.Phase == "prepared" && permit.RecoveryOnly)) {
			return RegisteredPackage{}, ErrRecoveryRequired
		}
	}
	record, ok := snapshot.Records[id]
	if !ok {
		return RegisteredPackage{}, errors.New("module is not registered")
	}
	return record, nil
}

func (r *Registry) resolve(ctx context.Context, id string, publisher Publisher, environment Environment, permit *ChangePermit) (*StagedPackage, error) {
	record, err := r.runtimeRecord(ctx, id, permit)
	if err != nil {
		return nil, err
	}
	stage, err := VerifyStage(ctx, record.StageID, StageOptions{Directory: r.directory, Publisher: publisher, Environment: environment, ExpectedManifestSHA256: record.ManifestSHA256})
	if err != nil {
		return nil, err
	}
	if stage.manifest.metadata.Module.ID != record.ModuleID || stage.manifest.metadata.Version != record.Version {
		return nil, errors.New("registered metadata differs from signed module")
	}
	return stage, nil
}

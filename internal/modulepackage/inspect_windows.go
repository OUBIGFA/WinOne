//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// Inspect authenticates an installed package without making it executable or
// applying target-Host compatibility. Maintenance callers inspect compatibility
// separately so damaged existing packages can remain quarantined.
func (r *Registry) Inspect(ctx context.Context, id string, publisher Publisher) (*VerifiedArchive, error) {
	record, err := r.runtimeRecord(ctx, id, nil)
	if err != nil {
		return nil, err
	}
	return r.InspectStoredPackage(ctx, record, publisher)
}

// InspectStoredPackage validates a recorded stage for recovery, without
// registration, activation or compatibility-based execution permission.
func (r *Registry) InspectStoredPackage(ctx context.Context, record RegisteredPackage, publisher Publisher) (*VerifiedArchive, error) {
	if err := validateRegistration(record); err != nil {
		return nil, err
	}
	base, err := os.OpenRoot(r.directory)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	info, err := base.Lstat(record.StageID)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("installed stage is not an ordinary directory")
	}
	root, err := base.OpenRoot(record.StageID)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	manifest, err := verifyStageContents(ctx, root, record.StageID, StageOptions{Publisher: publisher, ExpectedManifestSHA256: record.ManifestSHA256}, false)
	if err != nil {
		return nil, err
	}
	if manifest.metadata.PackageID() != record.ModuleID || manifest.metadata.Version != record.Version {
		return nil, ErrApprovalChanged
	}
	result := &VerifiedArchive{manifest: manifest}
	if manifest.metadata.Settings != nil {
		stage := &StagedPackage{directory: filepath.Join(r.directory, record.StageID), manifest: manifest, ready: true}
		result.definition, err = stage.LoadSettings(ctx)
	}
	return result, err
}

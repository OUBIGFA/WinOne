//go:build windows

package modulepackage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/configjson"
)

// Called inside the common package maintenance lock, before blocking or
// unloading a module. New dependents cannot be registered during this check.
func (r *Registry) checkChangeReferences(ctx context.Context, current RegistrySnapshot, request ChangeRequest) error {
	if source := request.Dependencies; source != nil {
		if !strings.EqualFold(source.modules.directory, r.directory) {
			return errors.New("module change dependency source belongs to another store")
		}
		record := request.Current
		return requireUnreferenced(ctx, source, Dependency{Kind: "module", ID: record.ModuleID, Version: record.Version, ManifestSHA256: record.ManifestSHA256})
	}
	// Legacy callers can still operate dependency-free stores. A digest-bound
	// declaration is sufficient to require the current coordinator, never to
	// grant execution or to treat an unauthenticated reference as removable.
	for _, record := range current.Records {
		if err := requireDependencyFreeRecord(ctx, r.directory, record); err != nil {
			return err
		}
	}
	runtimes, err := OpenRuntimeStore(r.directory)
	if err != nil {
		return err
	}
	snapshot, err := runtimes.read(ctx)
	if err != nil {
		return err
	}
	for _, record := range snapshot.Records {
		if err := requireDependencyFreeRecord(ctx, runtimes.directory, record); err != nil {
			return err
		}
	}
	return nil
}

func requireDependencyFreeRecord(ctx context.Context, directory string, record RegisteredPackage) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	raw, err := readStageFile(ctx, root, filepath.ToSlash(filepath.Join(record.StageID, "module.json")), MaxManifestBytes)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != record.ManifestSHA256 {
		return errors.New("dependency ownership metadata differs from the registered content")
	}
	var manifest Manifest
	if err = configjson.DecodeObject(raw, &manifest); err != nil {
		return err
	}
	if err = manifest.Metadata.Validate(); err != nil {
		return err
	}
	if len(manifest.Metadata.Dependencies) > 0 {
		return errors.New("package changes with dependencies require the Host dependency coordinator")
	}
	return nil
}

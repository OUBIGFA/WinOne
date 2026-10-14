package modulepackage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// DependencyLock is the complete content-pinned closure for one root package.
// Paths and process identities are intentionally absent from persistent data.
type DependencyLock struct {
	SchemaVersion int          `json:"schemaVersion"`
	Platform      string       `json:"platform"`
	Root          Dependency   `json:"root"`
	Packages      []Dependency `json:"packages"`
}

func (lock DependencyLock) Validate() error {
	if lock.SchemaVersion != 1 || (lock.Platform != "windows-x64" && lock.Platform != "windows-arm64") || len(lock.Packages) > MaxDependencyPackages {
		return errors.New("dependency lock header or size is invalid")
	}
	if err := lock.Root.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{lock.Root.ID: true}
	for _, item := range lock.Packages {
		if err := item.Validate(); err != nil {
			return err
		}
		if seen[item.ID] {
			return errors.New("dependency lock contains repeated identities")
		}
		seen[item.ID] = true
	}
	return nil
}

// Digest normalizes enumeration order while binding every exact package hash.
func (lock DependencyLock) Digest() (string, error) {
	if err := lock.Validate(); err != nil {
		return "", err
	}
	lock.Packages = append([]Dependency{}, lock.Packages...)
	sort.Slice(lock.Packages, func(i, j int) bool { return lock.Packages[i].ID < lock.Packages[j].ID })
	raw, err := json.Marshal(lock)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ResolveDependencies validates a signed-metadata graph; it does not install or
// execute anything. The Host lookup must authenticate current stored content.
// Runtime file leases and durable installation references remain separate steps.
func ResolveDependencies(ctx context.Context, root *VerifiedManifest, environment Environment, lookup func(context.Context, Dependency) (*VerifiedManifest, error)) (DependencyLock, error) {
	if root == nil {
		return DependencyLock{}, errors.New("dependency root is missing")
	}
	if err := ctx.Err(); err != nil {
		return DependencyLock{}, err
	}
	metadata := root.Metadata()
	identity := Dependency{Kind: metadata.PackageKind(), ID: metadata.PackageID(), Version: metadata.Version, ManifestSHA256: root.Digest()}
	if err := identity.Validate(); err != nil {
		return DependencyLock{}, err
	}
	report, err := metadata.CheckCompatibility(environment)
	if err != nil {
		return DependencyLock{}, err
	}
	if !report.Compatible {
		return DependencyLock{}, fmt.Errorf("dependency root %s is incompatible with this Host", identity.ID)
	}
	lock := DependencyLock{SchemaVersion: 1, Platform: environment.Platform, Root: identity, Packages: []Dependency{}}
	identities := map[string]Dependency{identity.ID: identity}
	states := map[string]uint8{identity.ID: 1}
	var visit func(*VerifiedManifest, int) error
	visit = func(manifest *VerifiedManifest, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > MaxDependencyDepth {
			return errors.New("dependency graph exceeds depth limit")
		}
		for _, wanted := range manifest.Metadata().Dependencies {
			if previous, exists := identities[wanted.ID]; exists {
				if previous != wanted {
					return fmt.Errorf("dependency %s has conflicting versions, kinds or content", wanted.ID)
				}
				if states[wanted.ID] == 1 {
					return fmt.Errorf("dependency graph contains a cycle at %s", wanted.ID)
				}
				continue
			}
			if len(identities) > MaxDependencyPackages {
				return errors.New("dependency graph contains too many packages")
			}
			if lookup == nil {
				return fmt.Errorf("dependency %s@%s is not available", wanted.ID, wanted.Version)
			}
			loaded, err := lookup(ctx, wanted)
			if err != nil {
				return fmt.Errorf("dependency %s@%s: %w", wanted.ID, wanted.Version, err)
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			if loaded == nil {
				return fmt.Errorf("dependency %s@%s is missing", wanted.ID, wanted.Version)
			}
			value := loaded.Metadata()
			actual := Dependency{Kind: value.PackageKind(), ID: value.PackageID(), Version: value.Version, ManifestSHA256: loaded.Digest()}
			if actual != wanted {
				return fmt.Errorf("dependency %s differs from its pinned identity", wanted.ID)
			}
			report, err := value.CheckCompatibility(environment)
			if err != nil {
				return err
			}
			if !report.Compatible {
				return fmt.Errorf("dependency %s is incompatible with this Host", wanted.ID)
			}
			identities[wanted.ID] = wanted
			states[wanted.ID] = 1
			if err = visit(loaded, depth+1); err != nil {
				return err
			}
			states[wanted.ID] = 2
			lock.Packages = append(lock.Packages, wanted)
		}
		return nil
	}
	if err = visit(root, 0); err != nil {
		return DependencyLock{}, err
	}
	sort.Slice(lock.Packages, func(i, j int) bool { return lock.Packages[i].ID < lock.Packages[j].ID })
	return lock, lock.Validate()
}

package modulepackage

import (
	"encoding/hex"
	"errors"
	"strings"
	"winone/internal/catalog"
	"winone/internal/distribution"
)

const MaxDirectDependencies = 64
const MaxDependencyPackages = 256
const MaxDependencyDepth = 32

// Dependency pins content, not just a display version. It carries no path or
// download URL; trusted Host storage resolves the signed package identity.
type Dependency struct {
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	Version        string `json:"version"`
	ManifestSHA256 string `json:"manifestSha256"`
}

type RuntimeManifest struct {
	ID      string                `json:"id"`
	Name    catalog.LocalizedName `json:"name"`
	Exports []RuntimeExport       `json:"exports"`
}

// Exports describe signed files/directories, never shell commands or global
// environment mutations. A worker later receives only resolved private paths.
type RuntimeExport struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"`
}

func (r RuntimeManifest) Validate() error {
	if !catalog.ValidOptionalID(r.ID) || len(r.ID) > 128 || strings.TrimSpace(r.Name.Chinese) == "" || strings.TrimSpace(r.Name.English) == "" {
		return errors.New("runtime identity is invalid")
	}
	if len(r.Exports) == 0 || len(r.Exports) > 32 {
		return errors.New("runtime exports are missing or exceed limits")
	}
	seen := map[string]bool{}
	for _, export := range r.Exports {
		if !catalog.ValidID(export.Name) || len(export.Name) > 64 || seen[export.Name] {
			return errors.New("runtime export name is invalid or duplicated")
		}
		seen[export.Name] = true
		if export.Kind != "file" && export.Kind != "directory" {
			return errors.New("runtime export kind is invalid")
		}
		if export.Path == "." && export.Kind == "directory" {
			continue
		}
		if !distribution.ValidPayloadPath(export.Path) {
			return errors.New("runtime export requires a canonical package-relative path")
		}
	}
	return nil
}

func (d Dependency) Validate() error {
	if (d.Kind != "module" && d.Kind != "runtime") || !catalog.ValidOptionalID(d.ID) || len(d.ID) > 128 {
		return errors.New("dependency identity is invalid")
	}
	if _, err := distribution.CompareVersions(d.Version, d.Version); err != nil {
		return err
	}
	digest, err := hex.DecodeString(d.ManifestSHA256)
	if err != nil || len(digest) != 32 || d.ManifestSHA256 != strings.ToLower(d.ManifestSHA256) {
		return errors.New("dependency manifest digest is invalid")
	}
	return nil
}

func validateDependencies(id string, dependencies []Dependency) error {
	if len(dependencies) > MaxDirectDependencies {
		return errors.New("too many direct dependencies")
	}
	seen := map[string]bool{}
	for _, dependency := range dependencies {
		if err := dependency.Validate(); err != nil {
			return err
		}
		if dependency.ID == id || seen[dependency.ID] {
			return errors.New("dependency is self-referencing or duplicated")
		}
		seen[dependency.ID] = true
	}
	return nil
}

func validateRuntimeInventory(metadata Metadata, files map[string]distribution.File) error {
	for _, export := range metadata.Runtime.Exports {
		path := strings.ToLower(export.Path)
		if export.Kind == "file" {
			if _, exists := files[path]; !exists {
				return errors.New("runtime export is absent from the signed inventory")
			}
			continue
		}
		if path == "." {
			continue
		}
		if _, exists := files[path]; exists {
			return errors.New("runtime directory export refers to a file")
		}
		found := false
		for name := range files {
			if strings.HasPrefix(name, path+"/") {
				found = true
				break
			}
		}
		if !found {
			return errors.New("runtime directory export is absent from the signed inventory")
		}
	}
	return nil
}

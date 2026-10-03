// Package modulepackage validates optional-module package data before installation.
// Metadata compatibility alone never establishes publisher trust or file integrity.
package modulepackage

import (
	"errors"
	"strconv"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/distribution"
)

const MaxMetadataBytes = 64 << 10

// Metadata wraps the same runtime manifest used by builtin features. Package
// versions and host requirements belong to installation, not runtime activation.
type Metadata struct {
	SchemaVersion int              `json:"schemaVersion"`
	Module        catalog.Manifest `json:"module"`
	Version       string           `json:"version"`
	Compatibility Compatibility    `json:"compatibility"`
}
type Compatibility struct {
	MinimumHostVersion          string `json:"minimumHostVersion"`
	MaximumHostVersionExclusive string `json:"maximumHostVersionExclusive"`
	ProtocolVersion             int    `json:"protocolVersion"`
	DesignSystemVersion         int    `json:"designSystemVersion"`
	Platform                    string `json:"platform"`
	MinimumWindowsBuild         uint32 `json:"minimumWindowsBuild"`
}

// Environment must be populated from the installed Host, never from the package.
type Environment struct {
	HostVersion         string
	ProtocolVersion     int
	DesignSystemVersion int
	Platform            string
	WindowsBuild        uint32
}
type Issue struct {
	Code     string `json:"code"`
	Required string `json:"required"`
	Actual   string `json:"actual"`
}
type CompatibilityReport struct {
	Compatible bool    `json:"compatible"`
	Issues     []Issue `json:"issues"`
}

func DecodeMetadata(data []byte) (Metadata, error) {
	if len(data) > MaxMetadataBytes {
		return Metadata{}, errors.New("module metadata exceeds size limit")
	}
	var metadata Metadata
	if err := configjson.DecodeObject(data, &metadata); err != nil {
		return Metadata{}, err
	}
	if err := metadata.Validate(); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}
func (m Metadata) Validate() error {
	if m.SchemaVersion != 1 {
		return errors.New("unsupported module package metadata")
	}
	if m.Module.Distribution != "optional" {
		return errors.New("external packages cannot declare builtin modules")
	}
	if err := m.Module.Validate(); err != nil {
		return err
	}
	if _, err := distribution.CompareVersions(m.Version, m.Version); err != nil {
		return err
	}
	c := m.Compatibility
	order, err := distribution.CompareVersions(c.MinimumHostVersion, c.MaximumHostVersionExclusive)
	if err != nil {
		return err
	}
	if order >= 0 {
		return errors.New("module host version range is empty")
	}
	if c.ProtocolVersion < 1 || c.DesignSystemVersion < 1 || c.MinimumWindowsBuild == 0 {
		return errors.New("module compatibility requirements are incomplete")
	}
	if c.Platform != "windows-x64" && c.Platform != "windows-arm64" {
		return errors.New("unsupported module platform")
	}
	return nil
}

// CheckCompatibility reports every mismatch so fixing one prerequisite does not
// hide another. Exact protocol/design-system versions are required in schema v1.
func (m Metadata) CheckCompatibility(environment Environment) (CompatibilityReport, error) {
	if err := m.Validate(); err != nil {
		return CompatibilityReport{}, err
	}
	if _, err := distribution.CompareVersions(environment.HostVersion, environment.HostVersion); err != nil {
		return CompatibilityReport{}, err
	}
	if environment.ProtocolVersion < 1 || environment.DesignSystemVersion < 1 || environment.Platform == "" || environment.WindowsBuild == 0 {
		return CompatibilityReport{}, errors.New("host compatibility information is incomplete")
	}
	result := CompatibilityReport{Issues: []Issue{}}
	c := m.Compatibility
	add := func(code, required, actual string) {
		result.Issues = append(result.Issues, Issue{code, required, actual})
	}
	if environment.Platform != c.Platform {
		add("platform", c.Platform, environment.Platform)
	}
	if environment.ProtocolVersion != c.ProtocolVersion {
		add("protocol", strconv.Itoa(c.ProtocolVersion), strconv.Itoa(environment.ProtocolVersion))
	}
	if environment.DesignSystemVersion != c.DesignSystemVersion {
		add("design_system", strconv.Itoa(c.DesignSystemVersion), strconv.Itoa(environment.DesignSystemVersion))
	}
	if environment.WindowsBuild < c.MinimumWindowsBuild {
		add("windows_build", strconv.FormatUint(uint64(c.MinimumWindowsBuild), 10), strconv.FormatUint(uint64(environment.WindowsBuild), 10))
	}
	minimum, _ := distribution.CompareVersions(environment.HostVersion, c.MinimumHostVersion)
	maximum, _ := distribution.CompareVersions(environment.HostVersion, c.MaximumHostVersionExclusive)
	if minimum < 0 {
		add("host_too_old", c.MinimumHostVersion, environment.HostVersion)
	}
	if maximum >= 0 {
		add("host_too_new", c.MaximumHostVersionExclusive, environment.HostVersion)
	}
	result.Compatible = len(result.Issues) == 0
	return result, nil
}

// Package catalog describes modules without starting their processes or UI.
package catalog

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

type LocalizedName struct {
	Chinese string `json:"zh-CN"`
	English string `json:"en-US"`
}
type Manifest struct {
	ManifestVersion       int           `json:"manifestVersion"`
	ID                    string        `json:"id"`
	Name                  LocalizedName `json:"name"`
	Distribution          string        `json:"distribution"`
	Worker                string        `json:"worker"`
	Activation            string        `json:"activation"`
	ShortcutActions       []string      `json:"shortcutActions"`
	SettingsSchemaVersion int           `json:"settingsSchemaVersion"`
}

var identifier = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

func ValidID(value string) bool { return identifier.MatchString(value) }

// ValidOptionalID shares publisher namespace rules with package registration.
func ValidOptionalID(value string) bool {
	if !ValidID(value) {
		return false
	}
	publisher, _, qualified := strings.Cut(value, ".")
	if !qualified {
		return false
	}
	switch publisher {
	case "capture", "always-on-top", "wintray", "settings", "modules", "shortcuts", "residency", "profiles", "application", "updates", "packages":
		return false
	}
	return true
}

// Validate checks the static contract. Package signatures and file-system containment
// still have to be verified by the installer before any external module can execute.
func (m Manifest) Validate() error {
	if m.ManifestVersion != 1 {
		return errors.New("unsupported module manifest version")
	}
	if !ValidID(m.ID) {
		return errors.New("invalid module ID")
	}
	if strings.TrimSpace(m.Name.Chinese) == "" || strings.TrimSpace(m.Name.English) == "" {
		return errors.New("module requires Chinese and English names")
	}
	if m.Distribution != "bundled" && m.Distribution != "optional" {
		return errors.New("invalid module distribution")
	}
	if m.Distribution == "optional" {
		if !ValidOptionalID(m.ID) {
			return errors.New("optional module ID requires an unreserved publisher namespace")
		}
	}
	if m.Activation != "on-demand" && m.Activation != "configured" {
		return errors.New("invalid module activation policy")
	}
	normalized := strings.ReplaceAll(m.Worker, `\`, "/")
	if normalized == "" || normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "/") || strings.ContainsAny(normalized, ":\x00") || path.Clean(normalized) != normalized || strings.HasPrefix(normalized, "../") {
		return errors.New("module worker must use a canonical package-relative path")
	}
	if m.SettingsSchemaVersion < 1 {
		return errors.New("invalid settings schema version")
	}
	seen := map[string]bool{}
	for _, action := range m.ShortcutActions {
		if !identifier.MatchString(action) || seen[action] {
			return errors.New("invalid or duplicate shortcut action")
		}
		seen[action] = true
	}
	return nil
}
func (m Manifest) Clone() Manifest {
	m.ShortcutActions = append([]string{}, m.ShortcutActions...)
	return m
}

//go:embed builtin.json
var source embed.FS
var builtins = readBuiltins()

func readBuiltins() []Manifest {
	data, err := source.ReadFile("builtin.json")
	if err != nil {
		panic(err)
	}
	var manifests []Manifest
	if err = json.Unmarshal(data, &manifests); err != nil {
		panic(err)
	}
	seen := map[string]bool{}
	for _, m := range manifests {
		if err = m.Validate(); err != nil {
			panic(fmt.Errorf("builtin %s: %w", m.ID, err))
		}
		if seen[m.ID] {
			panic("duplicate builtin module")
		}
		seen[m.ID] = true
	}
	return manifests
}
func Builtins() []Manifest {
	result := make([]Manifest, len(builtins))
	for i, m := range builtins {
		result[i] = m.Clone()
	}
	return result
}

func Builtin(id string) (Manifest, bool) {
	for _, manifest := range builtins {
		if manifest.ID == id {
			return manifest.Clone(), true
		}
	}
	return Manifest{}, false
}

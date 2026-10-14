package modulepackage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"sort"
	"strings"
	"winone/internal/configjson"
	"winone/internal/distribution"
)

const MaxMigrationPatchBytes = 256 << 10
const maxMigrationSteps = 32
const maxMigrationOperations = 256

type compiledMigration struct {
	to    int
	patch jsonpatch.Patch
	paths []string
}

func validateMigrations(target int, steps []SettingsMigration) error {
	if len(steps) > maxMigrationSteps {
		return errors.New("too many configuration migration steps")
	}
	byVersion := make(map[int]SettingsMigration, len(steps))
	for _, step := range steps {
		if step.From < 1 || step.To <= step.From || step.To > target || !distribution.ValidPayloadPath(step.Patch) {
			return errors.New("invalid configuration migration declaration")
		}
		if _, exists := byVersion[step.From]; exists {
			return errors.New("ambiguous configuration migration chain")
		}
		byVersion[step.From] = step
	}
	for start := range byVersion {
		for version := start; version != target; {
			step, exists := byVersion[version]
			if !exists {
				return errors.New("configuration migration chain does not reach target")
			}
			version = step.To
		}
	}
	return nil
}

// JSON Pointer paths use RFC 6901 escapes. Reject control characters so field
// paths remain safe to display; values are never included in migration errors.
func validMigrationPointer(path string) bool {
	if len(path) > 1024 || path != "" && !strings.HasPrefix(path, "/") {
		return false
	}
	for index := 0; index < len(path); index++ {
		if path[index] < 0x20 {
			return false
		}
		if path[index] == '~' {
			index++
			if index >= len(path) || (path[index] != '0' && path[index] != '1') {
				return false
			}
		}
	}
	return true
}

func decodeMigrationPatch(raw []byte) (jsonpatch.Patch, []string, error) {
	invalid := errors.New("模块配置迁移规则无效")
	if len(raw) == 0 || len(raw) > MaxMigrationPatchBytes {
		return nil, nil, invalid
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, nil, invalid
	}
	// The shared validator checks duplicate keys, UTF-8 and depth. Wrapping the
	// RFC array does not interpret any values and avoids a second JSON grammar.
	wrapped := append(append([]byte(`{"patch":`), raw...), '}')
	if configjson.ValidateObject(wrapped) != nil {
		return nil, nil, invalid
	}
	var operations []json.RawMessage
	if err := json.Unmarshal(raw, &operations); err != nil || len(operations) > maxMigrationOperations {
		return nil, nil, invalid
	}
	fields := map[string]bool{}
	for _, rawOperation := range operations {
		var operation struct {
			Op    string
			Path  *string
			From  *string
			Value json.RawMessage
		}
		if json.Unmarshal(rawOperation, &operation) != nil || operation.Path == nil || !validMigrationPointer(*operation.Path) {
			return nil, nil, invalid
		}
		switch operation.Op {
		case "add", "replace", "test":
			if len(operation.Value) == 0 {
				return nil, nil, invalid
			}
		case "copy", "move":
			if operation.From == nil || !validMigrationPointer(*operation.From) {
				return nil, nil, invalid
			}
		case "remove":
		default:
			return nil, nil, invalid
		}
		if operation.Op != "test" {
			fields[*operation.Path] = true
		}
		if operation.Op == "move" {
			fields[*operation.From] = true
		}
	}
	patch, err := jsonpatch.DecodePatch(raw)
	if err != nil {
		return nil, nil, invalid
	}
	paths := make([]string, 0, len(fields))
	for path := range fields {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return patch, paths, nil
}

func (s *SettingsDefinition) configureMigrations(target int, steps []SettingsMigration, read func(string) ([]byte, error)) error {
	if target < 1 {
		return errors.New("配置版本无效")
	}
	if err := validateMigrations(target, steps); err != nil {
		return err
	}
	compiled := make(map[int]compiledMigration, len(steps))
	for _, step := range steps {
		raw, err := read(step.Patch)
		if err != nil {
			return fmt.Errorf("无法读取已签名的配置迁移规则：%w", err)
		}
		patch, paths, err := decodeMigrationPatch(raw)
		if err != nil {
			return err
		}
		compiled[step.From] = compiledMigration{step.To, patch, paths}
	}
	s.version, s.migrations = target, compiled
	return nil
}

// Migrate creates a separate candidate. It never changes the committed input,
// permits external references, runs code, or changes package/global settings.
func (s *SettingsDefinition) Migrate(from int, raw json.RawMessage) (json.RawMessage, error) {
	if from < 1 || from > s.version || len(raw) > MaxSettingsBytes || configjson.ValidateObject(raw) != nil {
		return nil, errors.New("原配置或配置版本无效")
	}
	candidate := append(json.RawMessage(nil), raw...)
	for version := from; version != s.version; {
		step, exists := s.migrations[version]
		if !exists {
			return nil, fmt.Errorf("缺少从配置版本 %d 升级的迁移规则", version)
		}
		options := &jsonpatch.ApplyOptions{SupportNegativeIndices: false, AccumulatedCopySizeLimit: MaxSettingsBytes, AllowMissingPathOnRemove: false, EnsurePathExistsOnAdd: false, EscapeHTML: false}
		next, err := step.patch.ApplyWithOptions(candidate, options)
		if err != nil || len(next) > MaxSettingsBytes || configjson.ValidateObject(next) != nil {
			return nil, fmt.Errorf("配置从版本 %d 迁移失败，原配置已保留", version)
		}
		candidate = next
		version = step.to
	}
	if err := s.Validate(candidate); err != nil {
		return nil, err
	}
	return candidate, nil
}

func (s *SettingsDefinition) MigrationPaths(from int) ([]string, error) {
	fields := map[string]bool{}
	if from < 1 || from > s.version {
		return nil, errors.New("原配置版本无效")
	}
	for version := from; version != s.version; {
		step, exists := s.migrations[version]
		if !exists {
			return nil, errors.New("配置迁移链不完整")
		}
		for _, path := range step.paths {
			if path == "" {
				path = "/"
			}
			fields[path] = true
		}
		version = step.to
	}
	paths := make([]string, 0, len(fields))
	for path := range fields {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

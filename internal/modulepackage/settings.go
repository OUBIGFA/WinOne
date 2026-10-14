package modulepackage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"os"
	"sort"
	"strings"
	"winone/internal/configjson"
)

const MaxSettingsSchemaBytes = 256 << 10
const MaxSettingsBytes = 2 << 20
const settingsSchemaURL = "https://winone.invalid/module-settings"
const settingsDialect = "https://json-schema.org/draft/2020-12/schema"

type closedSchemaLoader struct{}

func (closedSchemaLoader) Load(string) (any, error) {
	return nil, errors.New("module schemas may only reference their own embedded definitions")
}

// SettingsDefinition contains a compiled, immutable schema and copied default
// data. Loading/validating it never launches a worker or loads executable code.
type SettingsDefinition struct {
	schema     *jsonschema.Schema
	rawSchema  json.RawMessage
	defaults   json.RawMessage
	version    int
	migrations map[int]compiledMigration
}

func (s *SettingsDefinition) Defaults() json.RawMessage {
	return append(json.RawMessage(nil), s.defaults...)
}
func (s *SettingsDefinition) Schema() json.RawMessage {
	return append(json.RawMessage(nil), s.rawSchema...)
}

func (s *SettingsDefinition) Validate(raw json.RawMessage) error {
	if len(raw) > MaxSettingsBytes {
		return errors.New("模块配置超过大小限制")
	}
	value, err := configjson.ParseObject(raw)
	if err != nil {
		return errors.New("模块配置不是有效的 JSON 对象")
	}
	if err = s.schema.Validate(value); err == nil {
		return nil
	}
	// Upstream detailed errors may quote instance values. Return field paths
	// only; credentials or private text must not appear in RPC errors/logs.
	var validation *jsonschema.ValidationError
	if !errors.As(err, &validation) {
		return errors.New("模块配置校验失败")
	}
	fields := map[string]bool{}
	var visit func(*jsonschema.ValidationError)
	visit = func(problem *jsonschema.ValidationError) {
		if len(fields) >= 16 {
			return
		}
		if len(problem.Causes) > 0 {
			for _, cause := range problem.Causes {
				visit(cause)
			}
			return
		}
		var segments []string
		for _, segment := range problem.InstanceLocation {
			if len(segment) > 128 {
				segments = nil
				break
			}
			segments = append(segments, strings.ReplaceAll(strings.ReplaceAll(segment, "~", "~0"), "/", "~1"))
		}
		fields["/"+strings.Join(segments, "/")] = true
	}
	visit(validation)
	paths := make([]string, 0, len(fields))
	for field := range fields {
		paths = append(paths, field)
	}
	sort.Strings(paths)
	return errors.New("模块配置无效，请检查字段：" + strings.Join(paths, ", "))
}

// LoadSettings checks the actual read bytes against the signed inventory again;
// verification of a stage earlier in the workflow is not permission to read a
// changed schema later. A closed loader prevents both network and file access.
func (s *StagedPackage) LoadSettings(ctx context.Context) (*SettingsDefinition, error) {
	if s == nil || !s.ready || s.manifest == nil || s.manifest.metadata.Settings == nil {
		return nil, errors.New("模块包未提供受支持的离线配置定义")
	}
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	read := func(name string, limit int64) ([]byte, error) {
		for _, record := range s.manifest.files {
			if !strings.EqualFold(name, record.Path) {
				continue
			}
			raw, err := readStageFile(ctx, root, record.Path, limit)
			if err != nil {
				return nil, err
			}
			sum := sha256.Sum256(raw)
			if int64(len(raw)) != record.Size || hex.EncodeToString(sum[:]) != record.SHA256 {
				return nil, errors.New("模块配置文件与已签名清单不符")
			}
			return raw, nil
		}
		return nil, errors.New("模块配置文件未列入签名清单")
	}
	files := s.manifest.metadata.Settings
	schemaBytes, err := read(files.Schema, MaxSettingsSchemaBytes)
	if err != nil {
		return nil, err
	}
	defaults, err := read(files.Defaults, MaxSettingsBytes)
	if err != nil {
		return nil, err
	}
	result, err := compileSettings(schemaBytes, defaults)
	if err != nil {
		return nil, err
	}
	if err = result.configureMigrations(s.manifest.metadata.Module.SettingsSchemaVersion, files.Migrations, func(name string) ([]byte, error) { return read(name, MaxMigrationPatchBytes) }); err != nil {
		return nil, err
	}
	return result, ctx.Err()
}

func compileSettings(schemaBytes, defaults []byte) (*SettingsDefinition, error) {
	value, err := configjson.ParseObject(schemaBytes)
	if err != nil {
		return nil, err
	}
	if value["$schema"] != settingsDialect {
		return nil, errors.New("模块配置定义必须使用 JSON Schema 2020-12")
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(closedSchemaLoader{})
	if err = compiler.AddResource(settingsSchemaURL, value); err != nil {
		return nil, errors.New("模块配置定义无法载入")
	}
	schema, err := compiler.Compile(settingsSchemaURL)
	if err != nil {
		return nil, errors.New("模块配置定义无效，或引用了包外资源")
	}
	result := &SettingsDefinition{schema: schema, rawSchema: append(json.RawMessage(nil), schemaBytes...), defaults: append(json.RawMessage(nil), defaults...)}
	if err = result.Validate(defaults); err != nil {
		return nil, errors.New("模块默认配置未通过其配置定义校验")
	}
	return result, nil
}

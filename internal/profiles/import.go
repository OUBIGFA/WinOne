package profiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"winone/internal/captureprefs"
	"winone/internal/catalog"
)

// ImportPreview is a patch, not a settings.save request. The coordinator must
// merge local metadata and check revisions and device capabilities before use.
type ImportPreview struct {
	Patch         json.RawMessage `json:"patch,omitempty"`
	MissingPaths  []string        `json:"missingPaths"`
	ArgumentApps  []string        `json:"argumentApps"`
	RequiredPaths []string        `json:"requiredPaths"`
}

type pathResolver func(string, string) (string, error)

func exactFields(source map[string]json.RawMessage, fields string, nested ...string) (map[string]any, error) {
	allowed := map[string]bool{}
	for _, field := range strings.Fields(fields) {
		allowed[strings.Split(field, ":")[0]] = true
	}
	for _, name := range nested {
		allowed[name] = true
	}
	if len(source) != len(allowed) {
		return nil, errors.New("配置字段缺失或不受支持")
	}
	for name := range source {
		if !allowed[name] {
			return nil, fmt.Errorf("不支持的配置字段 %s", name)
		}
	}
	return projectFields(source, fields)
}
func nestedFields(raw json.RawMessage, fields string) (map[string]any, error) {
	var object map[string]json.RawMessage
	if err := strictJSON(raw, &object); err != nil {
		return nil, err
	}
	return exactFields(object, fields)
}
func requiredArray(raw json.RawMessage, target any) error {
	if strings.TrimSpace(string(raw)) == "null" || len(raw) == 0 {
		return errors.New("配置列表缺失")
	}
	return json.Unmarshal(raw, target)
}
func numberRange(values map[string]any, key string, minimum, maximum int) error {
	value := values[key].(int)
	if value < minimum || value > maximum {
		return fmt.Errorf("配置字段 %s 超出范围", key)
	}
	return nil
}

// Syntax only: existence, file type and permissions belong to device validation.
func ordinaryWindowsPath(value string) bool {
	value = strings.ReplaceAll(value, "/", `\`)
	if strings.ContainsAny(value, "\x00\r\n<>\"|?*") {
		return false
	}
	rest := ""
	if len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && value[2] == '\\' {
		rest = value[3:]
	} else if strings.HasPrefix(value, `\\`) {
		parts := strings.Split(value[2:], `\`)
		if len(parts) < 2 || parts[0] == "" || parts[0] == "." || parts[0] == ".." || parts[1] == "" {
			return false
		}
		rest = value[2:]
	} else {
		return false
	}
	return !strings.Contains(rest, ":")
}

func importCapture(source map[string]json.RawMessage, resolve pathResolver) (map[string]any, error) {
	optional := []string{}
	if _, exists := source["toolbarTools"]; exists {
		optional = append(optional, "toolbarTools")
	}
	if _, exists := source["fileNameTemplate"]; exists {
		optional = append(optional, "fileNameTemplate")
	}
	patch, err := exactFields(source, "copyToClipboard:b askWhereToSave:b openEditor:b pinAfterCapture:b scrollDelay:n scrollFromTop:b outputDirectoryRef:s", optional...)
	if err != nil {
		return nil, err
	}
	if raw, exists := source["toolbarTools"]; exists {
		patch["toolbarTools"], err = captureprefs.DecodeToolbar(raw)
		if err != nil {
			return nil, err
		}
	}
	if err = numberRange(patch, "scrollDelay", 100, 2000); err != nil {
		return nil, err
	}
	if raw, exists := source["fileNameTemplate"]; exists {
		patch["fileNameTemplate"], err = captureprefs.DecodeFileNameTemplate(raw)
		if err != nil {
			return nil, err
		}
	}
	patch["outputDirectory"], err = resolve(patch["outputDirectoryRef"].(string), "directory")
	delete(patch, "outputDirectoryRef")
	return patch, err
}

var portableColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func importAOT(source map[string]json.RawMessage, resolve pathResolver) (map[string]any, error) {
	patch, err := exactFields(source, "frame:b frameThickness:n frameColor:s frameOpacity:n useAccent:b roundCorners:b blockInGameMode:b sound:b", "excludedApps")
	if err != nil {
		return nil, err
	}
	if err = numberRange(patch, "frameThickness", 1, 15); err != nil {
		return nil, err
	}
	if err = numberRange(patch, "frameOpacity", 10, 100); err != nil {
		return nil, err
	}
	if !portableColor.MatchString(patch["frameColor"].(string)) {
		return nil, errors.New("颜色须为 #RRGGBB 格式")
	}
	var entries []map[string]json.RawMessage
	if err = requiredArray(source["excludedApps"], &entries); err != nil {
		return nil, err
	}
	apps := make([]string, 0, len(entries))
	for _, entry := range entries {
		if len(entry) != 1 {
			return nil, errors.New("排除程序须为名称或路径引用")
		}
		var name string
		if _, exists := entry["name"]; exists {
			name, err = sourceString(entry, "name")
			if err == nil && (strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\\/:\x00\r\n")) {
				err = errors.New("程序路径必须通过路径映射导入")
			}
		} else {
			var key string
			key, err = sourceString(entry, "pathRef")
			if err == nil {
				name, err = resolve(key, "program")
			}
		}
		if err != nil {
			return nil, err
		}
		apps = append(apps, name)
	}
	patch["excludedApps"] = apps
	return patch, nil
}

// PrepareBuiltinImport never writes settings or starts applications. Missing
// mappings withhold the entire patch; path hints never act as device bindings.
func PrepareBuiltinImport(id string, module ModuleSettings, paths map[string]PathReference, bindings map[string]string) (ImportPreview, error) {
	result := ImportPreview{MissingPaths: []string{}, ArgumentApps: []string{}, RequiredPaths: []string{}}
	if module.SchemaVersion != 1 || len(module.Settings) > MaxEntryBytes {
		return result, errors.New("不支持的模块配置版本或大小")
	}
	var source map[string]json.RawMessage
	if err := strictJSON(module.Settings, &source); err != nil {
		return result, err
	}
	missing := map[string]bool{}
	referenced := map[string]bool{}
	resolve := func(key, kind string) (string, error) {
		ref, exists := paths[key]
		if !catalog.ValidID(key) || !exists || ref.Kind != kind {
			return "", errors.New("路径引用缺失或类型不匹配")
		}
		if !referenced[key] {
			result.RequiredPaths = append(result.RequiredPaths, key)
			referenced[key] = true
		}
		value := bindings[key]
		if value == "" {
			if !missing[key] {
				result.MissingPaths = append(result.MissingPaths, key)
				missing[key] = true
			}
			return "", nil
		}
		if !ordinaryWindowsPath(value) {
			return "", errors.New("请选择有效的绝对路径")
		}
		return value, nil
	}
	var patch map[string]any
	var err error
	switch id {
	case "timed-power":
		patch, err = importTimedPower(source)
	case "stroke":
		patch, err = importStroke(source, resolve)
	case "quick-assistant":
		patch, err = quickAssistantFields(source, true)
	case "selection":
		patch, err = selectionFields(source, true)
	case "capture":
		patch, err = importCapture(source, resolve)
	case "always-on-top":
		patch, err = importAOT(source, resolve)
	case "wintray":
		patch, result.ArgumentApps, err = importTray(source, resolve)
	default:
		err = errors.New("模块尚无导入适配器")
	}
	if err != nil {
		return ImportPreview{}, err
	}
	if len(result.MissingPaths) == 0 {
		result.Patch, err = json.Marshal(patch)
	}
	return result, err
}

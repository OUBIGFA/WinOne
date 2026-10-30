package profiles

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"winone/internal/captureprefs"
)

// ExportOptions requires an explicit choice per application because command-line
// arguments may contain secrets. Arbitrary files and binaries are never included;
// known Stroke action source remains a reviewable part of its configuration.
type ExportOptions struct {
	IncludeArguments map[string]bool
}

// ExportedSettings is data only: callers must obtain a committed configuration
// snapshot before invoking it. Paths carry no source-machine location.
type ExportedSettings struct {
	Module           ModuleSettings
	Paths            map[string]PathReference
	OmittedArguments []string
}

// ExportBuiltinSettings projects current worker settings into the portable v1
// schema. Unknown source fields are deliberately discarded at every level;
// future worker versions require an explicit adapter rather than a blind copy.
// This is not an import validator or a configuration consistency barrier.
func ExportBuiltinSettings(id string, raw json.RawMessage, options ExportOptions) (ExportedSettings, error) {
	if id == "timed-power" {
		return exportTimedPower(raw)
	}
	if id == "stroke" {
		return exportStroke(raw)
	}
	result := ExportedSettings{Paths: map[string]PathReference{}, OmittedArguments: []string{}}
	if len(raw) > MaxEntryBytes {
		return result, errors.New("模块配置过大")
	}
	var source map[string]json.RawMessage
	if err := strictJSON(raw, &source); err != nil {
		return result, err
	}
	expected := 1
	if id == "wintray" {
		expected = 3
	}
	var version int
	if err := json.Unmarshal(source["schemaVersion"], &version); err != nil || version != expected {
		return result, errors.New("不支持的模块配置版本")
	}
	var portable map[string]any
	var err error
	switch id {
	case "quick-assistant":
		portable, err = quickAssistantFields(source, false)
	case "selection":
		portable, err = selectionFields(source, false)
	case "capture":
		portable, err = projectFields(source, "copyToClipboard:b askWhereToSave:b openEditor:b pinAfterCapture:b scrollDelay:n scrollFromTop:b")
		if raw, exists := source["toolbarTools"]; err == nil && exists {
			portable["toolbarTools"], err = captureprefs.DecodeToolbar(raw)
		}
		if raw, exists := source["fileNameTemplate"]; err == nil && exists {
			portable["fileNameTemplate"], err = captureprefs.DecodeFileNameTemplate(raw)
		}
		if err != nil {
			break
		}
		var directory string
		directory, err = sourceString(source, "outputDirectory")
		if err != nil {
			break
		}
		if strings.TrimSpace(directory) == "" {
			err = errors.New("截图保存目录为空")
			break
		}
		portable["outputDirectoryRef"] = "capture.output"
		result.Paths["capture.output"] = PathReference{Kind: "directory", Label: "截图保存目录"}
	case "always-on-top":
		portable, err = projectFields(source, "frame:b frameThickness:n frameColor:s frameOpacity:n useAccent:b roundCorners:b blockInGameMode:b sound:b")
		if err != nil {
			break
		}
		var apps []string
		if bytes.Equal(bytes.TrimSpace(source["excludedApps"]), []byte("null")) {
			err = errors.New("排除程序列表为空")
			break
		}
		if err = json.Unmarshal(source["excludedApps"], &apps); err != nil {
			break
		}
		exclusions := make([]map[string]string, 0, len(apps))
		for index, app := range apps {
			if strings.TrimSpace(app) == "" {
				err = errors.New("排除程序名称为空")
				break
			}
			// Windows paths include drive, UNC and relative path forms. Even a relative
			// path needs a device mapping; only bare executable names are portable.
			if strings.ContainsAny(app, `\/:`) {
				key := fmt.Sprintf("always-on-top.excluded.%d", index)
				result.Paths[key] = PathReference{Kind: "program", Label: fmt.Sprintf("置顶排除程序 %d", index+1)}
				exclusions = append(exclusions, map[string]string{"pathRef": key})
			} else {
				exclusions = append(exclusions, map[string]string{"name": app})
			}
		}
		portable["excludedApps"] = exclusions
	case "wintray":
		portable, err = projectFields(source, "exitAfterManagedAppsCompleted:b closeWindowRetrySeconds:n startupIntervalSeconds:n")
		if err != nil {
			break
		}
		var entries []map[string]json.RawMessage
		if bytes.Equal(bytes.TrimSpace(source["managedApps"]), []byte("null")) {
			err = errors.New("程序列表为空")
			break
		}
		if err = json.Unmarshal(source["managedApps"], &entries); err != nil {
			break
		}
		apps := make([]map[string]any, 0, len(entries))
		seen := map[string]bool{}
		for index, entry := range entries {
			var app map[string]any
			app, err = projectFields(entry, "id:s name:s runOnStartup:b collectTrayIcon:b launchViaLogonTask:b launchHiddenInBackground:b")
			if err != nil {
				break
			}
			appID := app["id"].(string)
			if appID == "" || seen[appID] {
				err = errors.New("程序标识为空或重复")
				break
			}
			seen[appID] = true
			var path, args string
			path, err = sourceString(entry, "exePath")
			if err != nil {
				break
			}
			if strings.TrimSpace(path) == "" {
				err = errors.New("程序位置为空")
				break
			}
			args, err = sourceString(entry, "args")
			if err != nil {
				break
			}
			app["args"] = ""
			if options.IncludeArguments[appID] {
				app["args"] = args
			} else if args != "" {
				result.OmittedArguments = append(result.OmittedArguments, appID)
			}
			key := fmt.Sprintf("wintray.program.%d", index)
			app["programRef"] = key
			result.Paths[key] = PathReference{Kind: "program", Label: app["name"].(string)}
			for _, nested := range []struct{ name, fields string }{
				{"trayBehavior", "autoMinimizeAndHideOnLaunch:b closeDelaySeconds:n"},
				{"schedule", "enabled:b startDelayMinutes:n autoExitMinutes:n frequencyEnabled:b frequencyDays:n frequencyRuns:n"},
			} {
				var fields map[string]json.RawMessage
				if err = strictJSON(entry[nested.name], &fields); err != nil {
					break
				}
				app[nested.name], err = projectFields(fields, nested.fields)
				if err != nil {
					break
				}
			}
			if err != nil {
				break
			}
			apps = append(apps, app)
		}
		if err == nil {
			for key, include := range options.IncludeArguments {
				if include && !seen[key] {
					err = errors.New("选择的程序已不存在")
					break
				}
			}
		}
		portable["managedApps"] = apps
	default:
		err = errors.New("模块尚无导出适配器")
	}
	if err != nil {
		return ExportedSettings{}, err
	}
	encoded, err := json.Marshal(portable)
	if err != nil {
		return ExportedSettings{}, err
	}
	result.Module = ModuleSettings{SchemaVersion: 1, Settings: encoded}
	return result, nil
}

// A narrow allowlist also checks primitive types so malformed or newly nested
// values cannot smuggle runtime records into otherwise portable field names.
func projectFields(source map[string]json.RawMessage, fields string) (map[string]any, error) {
	result := map[string]any{}
	for _, field := range strings.Fields(fields) {
		pair := strings.Split(field, ":")
		name, kind := pair[0], pair[1]
		raw, exists := source[name]
		if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("缺少配置字段 %s", name)
		}
		var target any
		switch kind {
		case "b":
			target = new(bool)
		case "n":
			target = new(int)
		case "s":
			target = new(string)
		default:
			return nil, errors.New("导出字段类型无效")
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return nil, fmt.Errorf("配置字段 %s 类型无效", name)
		}
		switch value := target.(type) {
		case *bool:
			result[name] = *value
		case *int:
			result[name] = *value
		case *string:
			result[name] = *value
		}
	}
	return result, nil
}

func sourceString(source map[string]json.RawMessage, name string) (string, error) {
	fields, err := projectFields(source, name+":s")
	if err != nil {
		return "", err
	}
	return fields[name].(string), nil
}

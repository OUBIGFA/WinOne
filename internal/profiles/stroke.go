package profiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"winone/modules/stroke"
)

// Stroke actions are known configuration, shown in the import review before
// application. Preserve their C# source verbatim; never copy plugin binaries or
// interpret code to guess which string literals might be local file paths.
type portableStroke struct {
	stroke.Preferences
	Scopes     []portableStrokeScope    `json:"scopes"`
	Assemblies []portableStrokeAssembly `json:"assemblies"`
}
type portableStrokeScope struct {
	stroke.Scope
	ProcessPathRef string `json:"processPathRef,omitempty"`
}
type portableStrokeAssembly struct {
	Name    string `json:"name,omitempty"`
	PathRef string `json:"pathRef,omitempty"`
}

func exportStroke(raw json.RawMessage) (ExportedSettings, error) {
	result := ExportedSettings{Paths: map[string]PathReference{}, OmittedArguments: []string{}}
	var prefs stroke.Preferences
	if len(raw) > MaxEntryBytes {
		return result, errors.New("模块配置过大")
	}
	if err := strictJSON(raw, &prefs); err != nil {
		return result, err
	}
	if err := prefs.Validate(); err != nil {
		return result, err
	}
	portable := portableStroke{Preferences: prefs, Scopes: []portableStrokeScope{}, Assemblies: []portableStrokeAssembly{}}
	portable.Revision = 0
	portable.Preferences.Scopes = nil
	portable.Preferences.Assemblies = nil
	for index, scope := range prefs.Scopes {
		entry := portableStrokeScope{Scope: scope}
		if scope.ProcessPath != "" {
			key := fmt.Sprintf("stroke.scope.%d", index)
			entry.ProcessPath = ""
			entry.ProcessPathRef = key
			result.Paths[key] = PathReference{Kind: "program", Label: scope.Name}
		}
		portable.Scopes = append(portable.Scopes, entry)
	}
	for index, assembly := range prefs.Assemblies {
		entry := portableStrokeAssembly{Name: assembly}
		if strings.ContainsAny(assembly, `\/:`) {
			key := fmt.Sprintf("stroke.assembly.%d", index)
			entry = portableStrokeAssembly{PathRef: key}
			result.Paths[key] = PathReference{Kind: "file", Label: fmt.Sprintf("鼠标手势程序集 %d", index+1)}
		}
		portable.Assemblies = append(portable.Assemblies, entry)
	}
	encoded, err := json.Marshal(portable)
	result.Module = ModuleSettings{SchemaVersion: 1, Settings: encoded}
	return result, err
}

func importStroke(source map[string]json.RawMessage, resolve pathResolver) (map[string]any, error) {
	raw, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	var portable portableStroke
	if err = strictJSON(raw, &portable); err != nil {
		return nil, err
	}
	if portable.Revision != 0 || portable.Scopes == nil || portable.Assemblies == nil {
		return nil, errors.New("鼠标手势导入内容不完整")
	}
	prefs := portable.Preferences
	prefs.Scopes = []stroke.Scope{}
	prefs.Assemblies = []string{}
	for _, entry := range portable.Scopes {
		if entry.ProcessPath != "" {
			return nil, errors.New("程序位置需要通过本机路径映射导入")
		}
		scope := entry.Scope
		if entry.ProcessPathRef != "" {
			scope.ProcessPath, err = resolve(entry.ProcessPathRef, "program")
			if err != nil {
				return nil, err
			}
			// The coordinator withholds the entire patch when a mapping is
			// missing. A non-executed sentinel lets the remaining fields validate.
			if scope.ProcessPath == "" {
				scope.ProcessPath = `C:\pending\program.exe`
			}
		}
		prefs.Scopes = append(prefs.Scopes, scope)
	}
	for _, entry := range portable.Assemblies {
		if (entry.Name == "") == (entry.PathRef == "") || strings.ContainsAny(entry.Name, `\/:`) {
			return nil, errors.New("程序集需要名称或本机文件映射")
		}
		name := entry.Name
		if entry.PathRef != "" {
			name, err = resolve(entry.PathRef, "file")
			if err != nil {
				return nil, err
			}
			if name == "" {
				name = `C:\pending\assembly.dll`
			}
		}
		prefs.Assemblies = append(prefs.Assemblies, name)
	}
	if err = prefs.Validate(); err != nil {
		return nil, err
	}
	raw, err = json.Marshal(prefs)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	err = json.Unmarshal(raw, &result)
	return result, err
}

// Package profiles implements bounded, data-only portable configuration archives.
package profiles

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"winone/internal/catalog"
	"winone/internal/hotkeys"
	"winone/internal/settings"
)

const MaxArchiveBytes = 8 << 20
const MaxEntryBytes = 2 << 20
const MaxFiles = 256

type Preferences struct {
	Name           string            `json:"name"`
	Theme          string            `json:"theme"`
	Language       string            `json:"language"`
	Enabled        map[string]bool   `json:"enabled"`
	Shortcuts      map[string]string `json:"shortcuts"`
	RunAtLogon     bool              `json:"runAtLogon"`
	StartMinimized bool              `json:"startMinimized"`
	// Optional pointers distinguish older archives from explicit empty/false.
	DefaultPage   *string `json:"defaultPage,omitempty"`
	KeepUIRunning *bool   `json:"keepUIRunning,omitempty"`
	OpenShortcut  *string `json:"openShortcut,omitempty"`
}
type ModuleSettings struct {
	SchemaVersion int             `json:"schemaVersion"`
	Settings      json.RawMessage `json:"settings"`
}
type PathReference struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
}
type Archive struct {
	CreatedBy   string
	Preferences Preferences
	Modules     map[string]ModuleSettings
	Paths       map[string]PathReference
}
type manifest struct {
	Format    string            `json:"format"`
	Version   int               `json:"version"`
	CreatedBy string            `json:"createdBy"`
	Files     map[string]string `json:"files"`
}

func validate(a Archive) error {
	if a.CreatedBy == "" || len(a.CreatedBy) > 64 {
		return errors.New("配置缺少有效的创建版本")
	}
	if len(a.Preferences.Name) > 128 {
		return errors.New("配置名称过长")
	}
	switch a.Preferences.Theme {
	case "system", "light", "dark":
	default:
		return errors.New("配置主题无效")
	}
	switch a.Preferences.Language {
	case "system", "zh-CN", "en-US":
	default:
		return errors.New("配置语言无效")
	}
	if a.Preferences.Enabled == nil || a.Preferences.Shortcuts == nil || a.Modules == nil || a.Paths == nil {
		return errors.New("配置内容不完整")
	}
	if a.Preferences.DefaultPage != nil {
		if err := settings.Validate(settings.Settings{SchemaVersion: 1, Theme: "system", DefaultPage: *a.Preferences.DefaultPage}); err != nil {
			return fmt.Errorf("默认页面无效：%w", err)
		}
	}
	if a.Preferences.OpenShortcut != nil {
		if len(*a.Preferences.OpenShortcut) > 128 {
			return errors.New("打开 WinOne 的快捷键过长")
		}
		if _, err := hotkeys.Parse(*a.Preferences.OpenShortcut); err != nil {
			return fmt.Errorf("打开 WinOne：%w", err)
		}
	}
	for id := range a.Preferences.Enabled {
		if !catalog.ValidID(id) {
			return errors.New("模块标识无效")
		}
	}
	for id, key := range a.Preferences.Shortcuts {
		if !catalog.ValidID(id) || len(key) > 128 {
			return errors.New("快捷键配置无效")
		}
	}
	for id, module := range a.Modules {
		if !catalog.ValidID(id) || module.SchemaVersion < 1 {
			return errors.New("模块配置版本或标识无效")
		}
		if err := objectJSON(module.Settings); err != nil {
			return fmt.Errorf("模块 %s：%w", id, err)
		}
	}
	for id, ref := range a.Paths {
		if !catalog.ValidID(id) || len(ref.Label) > 256 || len(ref.Hint) > 4096 {
			return errors.New("路径映射无效")
		}
		switch ref.Kind {
		case "known-folder", "program", "directory", "file":
		default:
			return errors.New("不支持的路径映射类型")
		}
	}
	return nil
}

// Encode does not include runtime records, package binaries or arbitrary files.
// Module export adapters remain responsible for selecting portable, non-secret fields.
func Encode(a Archive) ([]byte, error) {
	if err := validate(a); err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	add := func(name string, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if len(data) > MaxEntryBytes {
			return errors.New("单项配置过大")
		}
		files[name] = data
		return nil
	}
	if err := add("profile.json", a.Preferences); err != nil {
		return nil, err
	}
	if err := add("paths.json", a.Paths); err != nil {
		return nil, err
	}
	for id, module := range a.Modules {
		if err := add("modules/"+id+".json", module); err != nil {
			return nil, err
		}
	}
	metadata := manifest{Format: "winone-profile", Version: 1, CreatedBy: a.CreatedBy, Files: map[string]string{}}
	for name, data := range files {
		sum := sha256.Sum256(data)
		metadata.Files[name] = hex.EncodeToString(sum[:])
	}
	if err := add("manifest.json", metadata); err != nil {
		return nil, err
	}
	if len(files) > MaxFiles {
		return nil, errors.New("配置条目过多")
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetMode(0600)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err = entry.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if output.Len() > MaxArchiveBytes {
		return nil, errors.New("配置档案超过 8 MiB")
	}
	return output.Bytes(), nil
}

// Decode reads into bounded memory and never extracts entries onto the filesystem.
// Digests detect corruption, not publisher authenticity or trustworthiness.
func Decode(data []byte) (Archive, error) {
	empty := Archive{}
	if len(data) > MaxArchiveBytes {
		return empty, errors.New("配置档案超过 8 MiB")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return empty, err
	}
	if len(reader.File) > MaxFiles {
		return empty, errors.New("配置条目过多")
	}
	files := map[string][]byte{}
	total := 0
	for _, entry := range reader.File {
		if !allowedEntry(entry.Name) || entry.Mode()&os.ModeType != 0 {
			return empty, errors.New("配置档案包含不允许的文件")
		}
		if _, exists := files[entry.Name]; exists {
			return empty, errors.New("配置档案包含重复文件")
		}
		if entry.CompressedSize64 > uint64(len(data)) || entry.UncompressedSize64 > MaxEntryBytes || entry.UncompressedSize64 > entry.CompressedSize64*128+65536 {
			return empty, errors.New("配置解压大小或压缩比超出限制")
		}
		stream, err := entry.Open()
		if err != nil {
			return empty, err
		}
		contents, readErr := io.ReadAll(io.LimitReader(stream, MaxEntryBytes+1))
		closeErr := stream.Close()
		if err = errors.Join(readErr, closeErr); err != nil {
			return empty, err
		}
		total += len(contents)
		if len(contents) > MaxEntryBytes || total > MaxArchiveBytes {
			return empty, errors.New("配置解压内容过大")
		}
		files[entry.Name] = contents
	}
	var meta manifest
	if err = strictJSON(files["manifest.json"], &meta); err != nil {
		return empty, err
	}
	if meta.Format != "winone-profile" || meta.Version != 1 || len(meta.Files) != len(files)-1 {
		return empty, errors.New("配置清单版本或文件列表无效")
	}
	for name, digest := range meta.Files {
		content, exists := files[name]
		if !exists || name == "manifest.json" {
			return empty, errors.New("配置清单与内容不一致")
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != digest {
			return empty, errors.New("配置内容校验失败")
		}
	}
	result := Archive{CreatedBy: meta.CreatedBy, Modules: map[string]ModuleSettings{}}
	if err = strictJSON(files["profile.json"], &result.Preferences); err != nil {
		return empty, err
	}
	if err = strictJSON(files["paths.json"], &result.Paths); err != nil {
		return empty, err
	}
	for name, content := range files {
		if strings.HasPrefix(name, "modules/") {
			id := strings.TrimSuffix(strings.TrimPrefix(name, "modules/"), ".json")
			var settings ModuleSettings
			if err = strictJSON(content, &settings); err != nil {
				return empty, err
			}
			result.Modules[id] = settings
		}
	}
	if err = validate(result); err != nil {
		return empty, err
	}
	return result, nil
}
func allowedEntry(name string) bool {
	if name == "manifest.json" || name == "profile.json" || name == "paths.json" {
		return true
	}
	if !strings.HasPrefix(name, "modules/") || !strings.HasSuffix(name, ".json") {
		return false
	}
	return catalog.ValidID(strings.TrimSuffix(strings.TrimPrefix(name, "modules/"), ".json"))
}

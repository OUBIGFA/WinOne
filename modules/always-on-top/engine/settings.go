// Package engine wraps the PowerToys native core and owns module preferences.
package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"winone/internal/configjson"
)

type Settings struct {
	SchemaVersion   int      `json:"schemaVersion"`
	Revision        uint64   `json:"revision"`
	Enabled         bool     `json:"enabled"` // Legacy wire field; Host owns the module switch.
	Frame           bool     `json:"frame"`
	FrameThickness  int      `json:"frameThickness"`
	FrameColor      string   `json:"frameColor"`
	FrameOpacity    int      `json:"frameOpacity"`
	UseAccent       bool     `json:"useAccent"`
	RoundCorners    bool     `json:"roundCorners"`
	BlockInGameMode bool     `json:"blockInGameMode"`
	Sound           bool     `json:"sound"`
	ExcludedApps    []string `json:"excludedApps"`
}

func Defaults() Settings {
	return Settings{SchemaVersion: 1, Enabled: true, Frame: true, FrameThickness: 3, FrameColor: "#808080", FrameOpacity: 100, UseAccent: true, RoundCorners: true, BlockInGameMode: true, ExcludedApps: []string{}}
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (s Settings) Validate() error {
	if s.SchemaVersion != 1 {
		return errors.New("不支持的置顶配置版本")
	}
	if s.FrameThickness < 1 || s.FrameThickness > 15 || s.FrameOpacity < 10 || s.FrameOpacity > 100 {
		return errors.New("边框粗细或不透明度超出范围")
	}
	if !colorPattern.MatchString(s.FrameColor) {
		return errors.New("颜色须为 #RRGGBB 格式")
	}
	for _, app := range s.ExcludedApps {
		if strings.TrimSpace(app) == "" || strings.ContainsAny(app, "\r\n") {
			return errors.New("每个排除项须为一个程序名称或路径")
		}
	}
	return nil
}

// ColorRef converts CSS RGB order to the COLORREF order required by Win32.
func (s Settings) ColorRef() uint32 {
	rgb, _ := strconv.ParseUint(strings.TrimPrefix(s.FrameColor, "#"), 16, 32)
	return uint32((rgb&255)<<16 | (rgb & 0xff00) | (rgb>>16)&255)
}
func (s Settings) Flags() uint32 {
	var flags uint32
	for i, b := range []bool{s.Frame, s.UseAccent, s.RoundCorners, s.BlockInGameMode, s.Sound} {
		if b {
			flags |= 1 << i
		}
	}
	return flags
}
func copySettings(s Settings) Settings {
	s.ExcludedApps = append([]string{}, s.ExcludedApps...)
	return s
}

type Store struct {
	mu          sync.Mutex
	path        string
	settings    Settings
	managedPath string
}

func OpenSettings(path string) (*Store, error) {
	s := &Store{path: path, settings: Defaults()}
	managedPath := filepath.Join(filepath.Dir(path), "managed-settings.json")
	_, statErr := os.Stat(managedPath)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	if statErr == nil {
		s.managedPath = managedPath
		path = managedPath
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err = configjson.DecodeObject(data, &s.settings); err != nil {
			return nil, err
		}
	}
	if err = s.settings.Validate(); err != nil {
		return nil, err
	}
	s.settings.Enabled = true
	return s, nil
}
func (s *Store) Snapshot() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copySettings(s.settings)
}

// IsManaged is fixed at startup; a managed worker never falls back to a legacy
// writable file if the Host projection disappears or becomes unreadable.
func (s *Store) IsManaged() bool { return s.managedPath != "" }

// ReloadManaged applies the Host's read-only projection. Runtime failure keeps
// the previous in-memory value and reconciles native state back to that value.
func (s *Store) ReloadManaged(apply func(Settings) error) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.managedPath == "" || apply == nil {
		return copySettings(s.settings), errors.New("未启用统一配置")
	}
	data, err := os.ReadFile(s.managedPath)
	if err != nil {
		return copySettings(s.settings), err
	}
	var next Settings
	if err = configjson.DecodeObject(data, &next); err != nil {
		return copySettings(s.settings), err
	}
	if err = next.Validate(); err != nil {
		return copySettings(s.settings), err
	}
	next.Enabled = true
	if err = apply(copySettings(next)); err != nil {
		return copySettings(s.settings), errors.Join(err, apply(copySettings(s.settings)))
	}
	s.settings = copySettings(next)
	return copySettings(s.settings), nil
}
func (s *Store) Save(next Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.managedPath != "" {
		return copySettings(s.settings), errors.New("请通过 WinOne 保存置顶设置")
	}
	if next.Revision != s.settings.Revision {
		return s.settings, errors.New("置顶设置已变更，请刷新后重试")
	}
	if err := next.Validate(); err != nil {
		return s.settings, err
	}
	next = copySettings(next)
	next.Enabled = true
	next.Revision++
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return s.settings, err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return s.settings, err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".aot-settings-*.tmp")
	if err != nil {
		return s.settings, err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return s.settings, err
	}
	if err = replaceSettings(tmp, s.path); err != nil {
		return s.settings, err
	}
	s.settings = next
	return copySettings(next), nil
}

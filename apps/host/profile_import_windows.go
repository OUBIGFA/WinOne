//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"winone/internal/configuration"
	"winone/internal/modulestate"
	"winone/internal/profiles"
	"winone/internal/settings"
)

type profilePathView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Value string `json:"value"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}
type profileFilePreview struct {
	archive               profiles.Archive
	Name                  string               `json:"name"`
	SourceHash            string               `json:"sourceHash"`
	ConfigurationRevision uint64               `json:"configurationRevision"`
	ModuleRevision        uint64               `json:"moduleRevision"`
	Preferences           profiles.Preferences `json:"preferences"`
	Plan                  profiles.ArchivePlan `json:"plan"`
	Paths                 []profilePathView    `json:"paths"`
	CurrentShortcuts      map[string]string    `json:"currentShortcuts"`
}

// Preview performs data and device checks only. Revisions and a content digest
// bind a later apply to what was reviewed; none of these checks authorizes execution.
func previewProfileFile(path string, bindings map[string]string, store *configuration.HostStore, states *modulestate.Store) (profileFilePreview, error) {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".winone") {
		return profileFilePreview{}, errors.New("请选择 .winone 配置文件")
	}
	file, err := os.Open(path)
	if err != nil {
		return profileFilePreview{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, profiles.MaxArchiveBytes+1))
	if err != nil {
		return profileFilePreview{}, err
	}
	archive, err := profiles.Decode(data)
	if err != nil {
		return profileFilePreview{}, err
	}
	plan, err := profiles.PrepareArchive(archive, bindings)
	if err != nil {
		return profileFilePreview{}, err
	}
	before := states.Snapshot()
	document := store.CommittedDocument()
	after := states.Snapshot()
	if before.Revision != after.Revision {
		return profileFilePreview{}, errors.New("模块状态已变更，请重新预览")
	}
	sum := sha256.Sum256(data)
	result := profileFilePreview{archive: archive, Name: archive.Preferences.Name, SourceHash: hex.EncodeToString(sum[:]), ConfigurationRevision: document.Revision, ModuleRevision: before.Revision, Preferences: archive.Preferences, Plan: plan, Paths: []profilePathView{}}
	var global settings.Settings
	if err = json.Unmarshal(document.Values["host"], &global); err != nil {
		return profileFilePreview{}, err
	}
	result.CurrentShortcuts = map[string]string{"capture.region": global.CaptureShortcut, "always-on-top.toggle": global.PinShortcut}
	ids := []string{}
	for id := range archive.Paths {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	problems := map[string]string{}
	for _, id := range ids {
		reference := archive.Paths[id]
		value := bindings[id]
		view := profilePathView{ID: id, Label: reference.Label, Kind: reference.Kind, Value: value, State: "needs_mapping"}
		if value != "" {
			info, statErr := os.Stat(value)
			switch {
			case statErr != nil:
				view.State = "invalid"
				view.Error = "找不到此位置，或无法访问"
			case reference.Kind == "program" && !info.Mode().IsRegular():
				view.State = "invalid"
				view.Error = "请选择程序文件"
			case reference.Kind == "directory" && !info.IsDir():
				view.State = "invalid"
				view.Error = "请选择文件夹"
			default:
				view.State = "ready"
			}
		}
		if view.State == "invalid" {
			problems[id] = view.Error
		}
		result.Paths = append(result.Paths, view)
	}
	for index, item := range result.Plan.Modules {
		if item.State == "missing_module" || item.State == "invalid" {
			continue
		}
		if _, exists := document.Values[item.ID]; !exists {
			result.Plan.Modules[index].State = "invalid"
			result.Plan.Modules[index].Error = "请先完成此模块的配置迁移"
			result.Plan.Modules[index].Patch = nil
			continue
		}
		for _, id := range item.RequiredPaths {
			if problem := problems[id]; problem != "" {
				result.Plan.Modules[index].State = "invalid"
				result.Plan.Modules[index].Error = problem
				result.Plan.Modules[index].Patch = nil
				break
			}
		}
	}
	return result, nil
}

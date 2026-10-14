//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"winone/internal/configuration"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/internal/profiles"
)

func TestProfilePreviewChecksDeviceMappingsWithoutApplyingSettings(t *testing.T) {
	directory := t.TempDir()
	get := func() *modules.Lazy { return nil }
	store, err := configuration.OpenHost(directory, aotConfiguration(directory, get), captureConfiguration(directory, get), wintrayConfiguration(directory, get), selectionConfiguration(directory, nil), quickAssistantConfiguration(directory, nil), strokeConfiguration(directory, get))
	if err != nil {
		t.Fatal(err)
	}
	states, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := collectProfile(store, states, "portable")
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := profiles.Encode(archive)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(directory, "source.winone")
	if err = os.WriteFile(source, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(directory, "configuration.json"))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := previewProfileFile(source, nil, store, states)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Paths) != 1 || preview.Paths[0].State != "needs_mapping" || preview.SourceHash == "" {
		t.Fatal(preview)
	}
	preview, err = previewProfileFile(source, map[string]string{"capture.output": filepath.Join(directory, "absent")}, store, states)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Paths[0].State != "invalid" || preview.Plan.Modules[1].State != "invalid" {
		t.Fatal(preview)
	}
	preview, err = previewProfileFile(source, map[string]string{"capture.output": directory}, store, states)
	if err != nil || preview.Paths[0].State != "ready" || preview.Plan.Modules[1].State != "ready" {
		t.Fatal(preview, err)
	}
	after, err := os.ReadFile(filepath.Join(directory, "configuration.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("preview changed preferences")
	}
}

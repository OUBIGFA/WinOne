//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsWindowDoesNotAdoptStoredCapture(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "capture-draft.json"), []byte(`{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`), 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{captureDirectory: directory}
	request, err := app.CaptureLaunch()
	if err != nil || request != nil {
		t.Fatal("settings window adopted the screenshot", request, err)
	}
}

func TestCompletedCaptureIsNotReopenedWhileMetadataAwaitsCleanup(t *testing.T) {
	directory := t.TempDir()
	id := strings.Repeat("c", 32)
	write := func(name string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("capture-draft.json", map[string]any{"id": id, "revision": 7})
	write("retired-drafts.json", []map[string]any{{"id": id, "completedRevision": 7}})
	app := &App{dedicatedCapture: true, captureDirectory: directory}
	if request, err := app.CaptureLaunch(); err != nil || request != nil {
		t.Fatal("completed capture reopened", request, err)
	}
	// A completion marker must not hide a different revision restored later.
	write("capture-draft.json", map[string]any{"id": id, "revision": 8})
	app = &App{dedicatedCapture: true, captureDirectory: directory}
	if request, err := app.CaptureLaunch(); err != nil || request == nil || request.DraftID != id {
		t.Fatal("newer revision was hidden", request, err)
	}
}

func TestFailedCaptureOpeningReleasesOnlyItsCloseGuard(t *testing.T) {
	app := &App{captureEditing: true, captureOnly: true}
	app.AbortCaptureOpening()
	if app.captureEditing || app.isCaptureOnly() || app.blockCaptureClose() {
		t.Fatal("failed opening retained its guard")
	}
	app = &App{captureEditing: true, captureOnly: true, settingsDirty: true}
	app.AbortCaptureOpening()
	if !app.blockCaptureClose() {
		t.Fatal("opening cleanup bypassed unsaved settings")
	}
}

func TestCaptureLaunchRecoversStoredDraftOnce(t *testing.T) {
	directory := t.TempDir()
	id := strings.Repeat("b", 32)
	if err := os.WriteFile(filepath.Join(directory, "capture-draft.json"), []byte(`{"id":"`+id+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{dedicatedCapture: true, captureDirectory: directory}
	request, err := app.CaptureLaunch()
	if err != nil || request == nil || request.DraftID != id {
		t.Fatal(request, err)
	}
	app.AcknowledgeCaptureLaunch(request.ID)
	request, err = app.CaptureLaunch()
	if err != nil || request != nil {
		t.Fatal("recovery request repeated", request, err)
	}
}

func TestCaptureLaunchWithoutDraftDirectoryIsEmpty(t *testing.T) {
	app := &App{dedicatedCapture: true, captureDirectory: filepath.Join(t.TempDir(), "missing")}
	request, err := app.CaptureLaunch()
	if err != nil || request != nil {
		t.Fatal(request, err)
	}
}

func TestCaptureCloseWaitsForEditorDecision(t *testing.T) {
	app := &App{}
	if app.blockCaptureClose() {
		t.Fatal("ordinary close was blocked")
	}
	app.SetCaptureEditing(true)
	if !app.blockCaptureClose() || !app.CaptureClosePending() {
		t.Fatal("editor close was not deferred")
	}
	app.CancelCaptureClose()
	if app.CaptureClosePending() {
		t.Fatal("keep-editing did not cancel close")
	}
	app.SetCaptureEditing(false)
	if app.blockCaptureClose() {
		t.Fatal("completed editor still blocks close")
	}
}

func TestCaptureLaunchSurvivesBeforeWindowStartup(t *testing.T) {
	app := &App{dedicatedCapture: true}
	id := strings.Repeat("a", 32)
	if err := app.queueCaptureDraft(id); err != nil {
		t.Fatal(err)
	}
	first, err := app.CaptureLaunch()
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || first.DraftID != id {
		t.Fatal("startup request was lost")
	}
	first.DraftID = "changed by caller"
	again, err := app.CaptureLaunch()
	if err != nil {
		t.Fatal(err)
	}
	if again.DraftID != id {
		t.Fatal("caller changed the pending request")
	}
	if !app.AcknowledgeCaptureLaunch(first.ID) {
		t.Fatal("acknowledged request was retained")
	}
	again, err = app.CaptureLaunch()
	if err != nil || again != nil {
		t.Fatal(again, err)
	}
}

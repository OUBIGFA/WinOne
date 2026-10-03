//go:build windows

package main

import "testing"

func TestSettingsCloseProtectsDirtyAndBusyPages(t *testing.T) {
	app := &App{}
	app.SetSettingsActivity(true, false)
	if !app.blockCaptureClose() || !app.SettingsClosePending() {
		t.Fatal("dirty settings did not defer close")
	}
	app.CancelSettingsClose()
	if app.SettingsClosePending() {
		t.Fatal("cancel did not clear request")
	}
	if !app.blockCaptureClose() {
		t.Fatal("cancel discarded dirty state")
	}
	app.SetSettingsActivity(true, true)
	if err := app.ApproveSettingsClose(); err == nil {
		t.Fatal("busy operation discarded")
	}
	app.SetSettingsActivity(true, false)
	if err := app.ApproveSettingsClose(); err != nil {
		t.Fatal(err)
	}
	if app.blockCaptureClose() {
		t.Fatal("approved close blocked")
	}
	if !app.blockCaptureClose() {
		t.Fatal("approval leaked to another close attempt")
	}
}

func TestCaptureClosePrecedesSettingsClose(t *testing.T) {
	app := &App{}
	app.SetSettingsActivity(true, false)
	app.SetCaptureEditing(true)
	if !app.blockCaptureClose() || !app.CaptureClosePending() || app.SettingsClosePending() {
		t.Fatal("capture decision was bypassed")
	}
	app.SetCaptureEditing(false)
	if !app.blockCaptureClose() || !app.SettingsClosePending() {
		t.Fatal("settings decision was bypassed")
	}
}

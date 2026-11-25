package distribution

import (
	_ "embed"
	"fmt"
	"strings"
)

// Pinned runtime inventory prevents loose directory/extension allowances from
// shipping local scripts, caches or credentials alongside the native runtime.
// Regenerate deliberately with build/update-ocr-inventory.ps1 after review.
//
//go:embed ocr_runtime_paths.txt
var ocrRuntimeInventory string

//go:embed ai_plugin_runtime_paths.txt
var accountPluginInventory string

//go:embed native_capture_runtime_paths.txt
var nativeCaptureInventory string

var nativeCaptureFiles = strings.Fields(nativeCaptureInventory)

func hasNativeCapture(seen map[string]bool) bool {
	return seen["capture/native-runtime.json"] || seen["capture/WinOne.Ocr.exe"] || seen["capture/winone.ocr.exe"]
}

func validateCaptureRuntime(seen map[string]bool) error {
	if !hasNativeCapture(seen) {
		return nil
	}
	allowed := map[string]bool{}
	for _, list := range [][]string{nativeCaptureFiles, strings.Fields(ocrRuntimeInventory), recordingRuntimeFiles} {
		for _, name := range list {
			allowed[strings.ToLower(name)] = true
		}
	}
	for name := range seen {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "capture/") && !allowed[lower] {
			return fmt.Errorf("unexpected file in native capture runtime: %s", name)
		}
	}
	return nil
}

// Existing checked installations retain their original .NET payload for
// rollback. A new native capture marker selects the complete native inventory.
func requiredBaseForPayload(seen map[string]bool) []string {
	if !hasNativeCapture(seen) {
		return requiredFiles
	}
	var result []string
	for _, name := range requiredFiles {
		if !strings.HasPrefix(name, "capture/") {
			result = append(result, name)
		}
	}
	return append(result, nativeCaptureFiles...)
}

var recordingRuntimeFiles = []string{"capture/WinOne.Recording.dll", "capture/recording-runtime.json"}
var learningRuntimeFiles = []string{"shared/ui/learning/ecdict.db", "shared/ui/learning/manifest.json", "shared/ui/learning/ECDICT-LICENSE.txt"}

var accountPluginFiles = strings.Fields(accountPluginInventory)
var interconnectRuntimeFiles = []string{"shared/interconnect/WinOne.Interconnect.Lan.exe", "shared/interconnect/WinOne.Interconnect.Worker.exe"}
var strokeRuntimeFiles = []string{"stroke/WinOne.Stroke.exe", "stroke/Stroke.Base.dll", "stroke/Base.dll", "stroke/Stroke.dll", "stroke/Stroke-LICENSE.txt", "stroke/schema.json", "stroke/defaults.json", "stroke/migrations.json"}

var textRuntimeFiles = append(strings.Fields(ocrRuntimeInventory), "shared/ai/WinOne.AI.exe", "shared/ai/magpie-LICENSE.txt")
var textRuntimeAllowed = func() map[string]bool {
	result := map[string]bool{}
	for _, name := range textRuntimeFiles {
		result[name] = true
	}
	for _, name := range accountPluginFiles {
		result[name] = true
	}
	for _, name := range interconnectRuntimeFiles {
		result[name] = true
	}
	return result
}()

// Older verified packages stay readable for rollback. Presence of any text
// feature file requires the complete new runtime, not a partial OCR upgrade.
func requiredForPayload(seen map[string]bool) []string {
	var learningFiles []string
	for _, name := range learningRuntimeFiles {
		if seen[name] {
			learningFiles = learningRuntimeFiles
			break
		}
	}
	result := append([]string{}, requiredBaseForPayload(seen)...)
	text, plugins, stroke, recording, interconnect := false, false, false, false, false
	if hasNativeCapture(seen) {
		text, recording = true, true
	}
	for name := range seen {
		lower := strings.ToLower(name)
		if lower == "capture/winone.recording.dll" || lower == "capture/recording-runtime.json" {
			recording = true
		}
		if strings.HasPrefix(lower, "stroke/") {
			stroke = true
		}
		if strings.HasPrefix(lower, "capture/ocr/") || strings.HasPrefix(lower, "shared/ai/") {
			text = true
		}
		if strings.HasPrefix(lower, "shared/ai/plugins/") || strings.HasPrefix(lower, "shared/ai/runtime/") {
			plugins = true
		}
		if strings.HasPrefix(lower, "shared/interconnect/") {
			interconnect = true
		}
	}
	if text {
		result = append(result, textRuntimeFiles...)
	}
	if plugins {
		result = append(result, accountPluginFiles...)
	}
	if recording {
		result = append(result, recordingRuntimeFiles...)
	}
	if stroke {
		result = append(result, strokeRuntimeFiles...)
	}
	// The interconnect payload is one delivery: a partial copy must fail
	// instead of silently removing the LAN, peer or transfer tool.
	if interconnect {
		result = append(result, interconnectRuntimeFiles...)
	}
	return append(result, learningFiles...)
}

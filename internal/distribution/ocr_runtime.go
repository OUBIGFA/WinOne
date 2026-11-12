package distribution

import (
	_ "embed"
	"strings"
)

// Pinned runtime inventory prevents loose directory/extension allowances from
// shipping local scripts, caches or credentials alongside the Python runtime.
// Regenerate deliberately with build/update-ocr-inventory.ps1 after review.
//
//go:embed ocr_runtime_paths.txt
var ocrRuntimeInventory string

//go:embed ai_plugin_runtime_paths.txt
var accountPluginInventory string

var recordingRuntimeFiles = []string{"capture/WinOne.Recording.dll", "capture/recording-runtime.json"}
var learningRuntimeFiles = []string{"shared/ui/learning/ecdict.db", "shared/ui/learning/manifest.json", "shared/ui/learning/ECDICT-LICENSE.txt"}

//go:embed anki-runtime-files.txt
var ankiRuntimeInventory string
var ankiRuntimeFiles = strings.Fields(ankiRuntimeInventory)

var accountPluginFiles = strings.Fields(accountPluginInventory)
var interconnectRuntimeFiles = []string{"shared/interconnect/WinOne.Interconnect.Lan.exe", "shared/interconnect/WinOne.Interconnect.Peer.exe", "shared/interconnect/WinOne.Interconnect.Transfer.exe"}
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
	for _, name := range ankiRuntimeFiles {
		result[name] = true
	}
	return result
}()

// Older verified packages stay readable for rollback. Presence of any text
// feature file requires the complete new runtime, not a partial OCR upgrade.
func requiredForPayload(seen map[string]bool) []string {
	var ankiFiles []string
	for name := range seen {
		if strings.HasPrefix(strings.ToLower(name), "shared/anki/") {
			ankiFiles = ankiRuntimeFiles
			break
		}
	}
	var learningFiles []string
	for _, name := range learningRuntimeFiles {
		if seen[name] {
			learningFiles = learningRuntimeFiles
			break
		}
	}
	result := append([]string{}, requiredFiles...)
	result = append(result, ankiFiles...)
	text, plugins, stroke, recording, interconnect := len(ankiFiles) > 0, false, false, false, false
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

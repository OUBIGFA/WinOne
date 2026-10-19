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

// Keep this list in step with build-chat.ps1 and the reviewed
// third_party/chat-share-notices.json; never allow arbitrary notice files.
//
//go:embed chat_share_runtime_paths.txt
var chatShareInventory string

// Code Mode adds the VM worker, its WASM runtime and the reviewed notices.
// Keep this list in step with build-chat.ps1 and
// third_party/chat-codemode-notices.json.
//
//go:embed chat_codemode_runtime_paths.txt
var chatCodeModeInventory string

// The new search runtime remains optional for old verified rollback bundles.
//
//go:embed chat_search_runtime_paths.txt
var chatSearchInventory string
var chatSearchFiles = strings.Fields(chatSearchInventory)
var recordingRuntimeFiles = []string{"capture/WinOne.Recording.dll", "capture/recording-runtime.json"}

var accountPluginFiles = strings.Fields(accountPluginInventory)
var chatRuntimeFiles = []string{"shared/chat/worker.js", "shared/chat/runtime.json"}
var chatShareFiles = strings.Fields(chatShareInventory)
var chatCodeModeFiles = strings.Fields(chatCodeModeInventory)
var chatOptionalRuntimeFiles = []string{"shared/chat/scheduler.js", "shared/chat/script-runner.js", "shared/ai/runtime/photon_rs_bg.wasm"}
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
	for _, name := range chatRuntimeFiles {
		result[name] = true
	}
	for _, name := range chatSearchFiles {
		result[name] = true
	}
	for _, name := range chatShareFiles {
		result[name] = true
	}
	for _, name := range chatCodeModeFiles {
		result[name] = true
	}
	// These arrived after the first Chat payload and stay optional in legacy
	// rollback inputs. The newer share payload below requires the full set.
	for _, name := range chatOptionalRuntimeFiles {
		result[name] = true
	}
	return result
}()

// Older verified packages stay readable for rollback. Presence of any text
// feature file requires the complete new runtime, not a partial OCR upgrade.
func requiredForPayload(seen map[string]bool) []string {
	result := append([]string{}, requiredFiles...)
	text, plugins, chat, share, codeMode, stroke, search, recording := false, false, false, false, false, false, false, false
	for name := range seen {
		lower := strings.ToLower(name)
		if lower == "capture/winone.recording.dll" || lower == "capture/recording-runtime.json" {
			recording = true
		}
		for _, resource := range chatSearchFiles {
			if lower == strings.ToLower(resource) {
				search = true
				break
			}
		}
		for _, resource := range chatCodeModeFiles {
			if lower == strings.ToLower(resource) {
				codeMode = true
				break
			}
		}
		if strings.HasPrefix(lower, "stroke/") {
			stroke = true
		}
		if strings.HasPrefix(lower, "shared/chat/") {
			chat = true
			text = true
			plugins = true
			for _, resource := range chatShareFiles {
				if lower == strings.ToLower(resource) {
					share = true
					break
				}
			}
		}
		if strings.HasPrefix(lower, "capture/ocr/") || strings.HasPrefix(lower, "shared/ai/") {
			text = true
		}
		if strings.HasPrefix(lower, "shared/ai/plugins/") || strings.HasPrefix(lower, "shared/ai/runtime/") {
			plugins = true
		}
	}
	if text {
		result = append(result, textRuntimeFiles...)
	}
	if plugins {
		result = append(result, accountPluginFiles...)
	}
	if chat {
		result = append(result, chatRuntimeFiles...)
	}
	// Any share notice identifies the new delivery: require every notice and
	// resource so an incomplete copy cannot silently remove licenses or tools.
	if share {
		result = append(result, chatShareFiles...)
		result = append(result, chatOptionalRuntimeFiles...)
	}
	if codeMode {
		result = append(result, chatCodeModeFiles...)
	}
	if search {
		result = append(result, chatSearchFiles...)
	}
	if recording {
		result = append(result, recordingRuntimeFiles...)
	}
	if stroke {
		result = append(result, strokeRuntimeFiles...)
	}
	return result
}

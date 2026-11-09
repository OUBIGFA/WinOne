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

// Math font data and its original licenses/source archive form one delivery.
// Keep this separate from share notices so old rollback bundles remain valid.
//
//go:embed chat_math_runtime_paths.txt
var chatMathInventory string
var chatMathFiles = strings.Fields(chatMathInventory)

// Code Mode adds the VM worker, its WASM runtime and the reviewed notices.
// Keep this list in step with build-chat.ps1 and
// third_party/chat-codemode-notices.json.
//
//go:embed chat_codemode_runtime_paths.txt
var chatCodeModeInventory string

// OneBot is an all-or-nothing optional addition for legacy rollback packages.
//
//go:embed chat_onebot_runtime_paths.txt
var chatOneBotInventory string
var chatOneBotFiles = strings.Fields(chatOneBotInventory)

// The new search runtime remains optional for old verified rollback bundles.
//
//go:embed chat_search_runtime_paths.txt
var chatSearchInventory string
var chatSearchFiles = strings.Fields(chatSearchInventory)
var recordingRuntimeFiles = []string{"capture/WinOne.Recording.dll", "capture/recording-runtime.json"}
var learningRuntimeFiles = []string{"shared/ui/learning/ecdict.db", "shared/ui/learning/manifest.json", "shared/ui/learning/ECDICT-LICENSE.txt"}

//go:embed anki-runtime-files.txt
var ankiRuntimeInventory string
var ankiRuntimeFiles = strings.Fields(ankiRuntimeInventory)

var accountPluginFiles = strings.Fields(accountPluginInventory)
var chatRuntimeFiles = []string{"shared/chat/worker.js", "shared/chat/runtime.json"}
var chatShareFiles = strings.Fields(chatShareInventory)
var chatCodeModeFiles = strings.Fields(chatCodeModeInventory)
var chatOptionalRuntimeFiles = []string{"shared/chat/scheduler.js", "shared/chat/script-runner.js", "shared/ai/runtime/photon_rs_bg.wasm"}
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
	for _, name := range chatRuntimeFiles {
		result[name] = true
	}
	for _, name := range chatSearchFiles {
		result[name] = true
	}
	for _, name := range chatMathFiles {
		result[name] = true
	}
	for _, name := range chatShareFiles {
		result[name] = true
	}
	for _, name := range chatOneBotFiles {
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
	text, plugins, chat, share, codeMode, stroke, search, recording, interconnect := false, false, false, false, false, false, false, false, false
	if len(ankiFiles) > 0 {
		text = true
	}
	onebot, math, library := false, false, false
	for name := range seen {
		lower := strings.ToLower(name)
		if lower == "shared/chat/librarydbworker.js" || lower == "shared/chat/library-sqlite.js" {
			library = true
		}
		if lower == "capture/winone.recording.dll" || lower == "capture/recording-runtime.json" {
			recording = true
		}
		for _, resource := range chatMathFiles {
			if lower == strings.ToLower(resource) {
				math = true
				break
			}
		}
		for _, resource := range chatOneBotFiles {
			if lower == strings.ToLower(resource) {
				onebot = true
				break
			}
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
	if chat {
		result = append(result, chatRuntimeFiles...)
	}
	// These helpers are one protocol pair in new bundles. Historical packages
	// without either remain valid for rollback; a partial new pair is rejected.
	if library {
		result = append(result, "shared/chat/libraryDbWorker.js", "shared/chat/library-sqlite.js")
	}
	// Any share notice identifies the new delivery: require every notice and
	// resource so an incomplete copy cannot silently remove licenses or tools.
	if share {
		result = append(result, chatShareFiles...)
		result = append(result, chatOptionalRuntimeFiles...)
	}
	if math {
		result = append(result, chatMathFiles...)
	}
	if onebot {
		result = append(result, chatOneBotFiles...)
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
	// The interconnect payload is one delivery: a partial copy must fail
	// instead of silently removing the LAN, peer or transfer tool.
	if interconnect {
		result = append(result, interconnectRuntimeFiles...)
	}
	return append(result, learningFiles...)
}

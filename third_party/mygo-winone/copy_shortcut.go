package ui

import "runtime"

// Windows exposes two standard copy chords. Normalize at the shared editor
// boundary so fields and selectable paragraphs use the same copy command,
// including its existing empty-selection, rich-content and password checks.
func windowsCopyKey(key Key, mods Modifiers) Key {
	if runtime.GOOS == "windows" && key == KeyInsert && mods == Ctrl {
		return KeyC
	}
	return key
}

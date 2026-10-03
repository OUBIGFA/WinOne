//go:build windows

package main

import "bytes"

type collectedIconView interface {
	SetTitle(string)
	SetIconPNG([]byte) error
}
type collectedIconState struct {
	name        string
	image       []byte
	initialized bool
}

// Do not recreate native bitmaps for unchanged snapshots. Failed icon writes
// remain retryable; a later valid frame must still replace the visible image.
func (state *collectedIconState) apply(view collectedIconView, name string, image []byte) error {
	if state.name != name {
		view.SetTitle(name)
		state.name = name
	}
	if state.initialized && bytes.Equal(state.image, image) {
		return nil
	}
	if err := view.SetIconPNG(image); err != nil {
		return err
	}
	state.image = bytes.Clone(image)
	state.initialized = true
	return nil
}

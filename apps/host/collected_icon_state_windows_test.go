//go:build windows

package main

import (
	"errors"
	"testing"
)

type iconViewFixture struct {
	titles []string
	images [][]byte
	fail   bool
}

func (view *iconViewFixture) SetTitle(value string) { view.titles = append(view.titles, value) }
func (view *iconViewFixture) SetIconPNG(value []byte) error {
	if view.fail {
		return errors.New("bitmap unavailable")
	}
	view.images = append(view.images, append([]byte(nil), value...))
	return nil
}
func TestCollectedIconRefreshesOnlyChangedFrames(t *testing.T) {
	var state collectedIconState
	view := &iconViewFixture{}
	first := []byte{1, 2, 3}
	if err := state.apply(view, "app", first); err != nil {
		t.Fatal(err)
	}
	if err := state.apply(view, "app", first); err != nil {
		t.Fatal(err)
	}
	if len(view.images) != 1 || len(view.titles) != 1 {
		t.Fatal("unchanged frame recreated native resources")
	}
	first[0] = 4
	if err := state.apply(view, "renamed", first); err != nil {
		t.Fatal(err)
	}
	if len(view.images) != 2 || view.images[1][0] != 4 || view.titles[1] != "renamed" {
		t.Fatal("dynamic frame or title was not updated")
	}
	view.fail = true
	if err := state.apply(view, "renamed", nil); err == nil {
		t.Fatal("icon error was hidden")
	}
	view.fail = false
	if err := state.apply(view, "renamed", nil); err != nil {
		t.Fatal(err)
	}
	if len(view.images) != 3 || len(view.images[2]) != 0 {
		t.Fatal("icon removal was not retried")
	}
}

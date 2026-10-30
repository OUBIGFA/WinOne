package shell

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// uiState is what the window remembers about itself in the profile,
// beside the Host's settings: the sidebar's width and how it opens. The
// React window kept the same values in its WebView storage.
type uiState struct {
	Version int `json:"version"`
	// ExpandedWidth is the sidebar's width when it shows names.
	ExpandedWidth float32 `json:"expandedWidth"`
	// SidebarStartup is "collapsed" (icons only, the default) or
	// "expanded"; folding the sidebar by hand never changes it.
	SidebarStartup string `json:"sidebarStartup"`
}

const (
	uiStateFile          = "desktop-ui.json"
	collapsedWidth       = 64
	minExpandedWidth     = 180
	maxSidebarWidth      = 360
	defaultExpandedWidth = 256
	// compactWidth is where the sidebar shows icons only.
	compactWidth = 148
)

func loadUIState(directory string) uiState {
	state := uiState{Version: 1, ExpandedWidth: defaultExpandedWidth, SidebarStartup: "collapsed"}
	data, err := os.ReadFile(filepath.Join(directory, uiStateFile))
	if err != nil {
		return state
	}
	var saved uiState
	if json.Unmarshal(data, &saved) != nil || saved.Version != 1 {
		return state
	}
	if saved.ExpandedWidth >= minExpandedWidth && saved.ExpandedWidth <= maxSidebarWidth {
		state.ExpandedWidth = saved.ExpandedWidth
	}
	if saved.SidebarStartup == "expanded" {
		state.SidebarStartup = "expanded"
	}
	return state
}

// save writes the state through a temporary file, so a crash never
// leaves a partial document.
func (s uiState) save(directory string) error {
	if directory == "" {
		return errors.New("profile directory unknown")
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	temporary := filepath.Join(directory, ".desktop-ui-"+strconv.FormatInt(time.Now().UnixNano(), 36)+".tmp")
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, filepath.Join(directory, uiStateFile)); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

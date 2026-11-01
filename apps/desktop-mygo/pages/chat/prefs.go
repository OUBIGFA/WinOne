package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"winone/desktopmygo/core"
	"winone/desktopmygo/widgets"
)

// prefs are Chat's viewing choices: the last model chosen, the completion
// chime, collapsed projects and sections, and the panels' sizes and dock.
// The React window kept them in its WebView storage
// (winone.chat.last-model.v1, .sound.v1, .collapsed-projects.v1,
// .sections.v1, .expanded-panels.v1); this window keeps them in the
// profile beside its other window state, apart from the Host's settings.
type prefs struct {
	Version   int                  `json:"version"`
	LastModel *widgets.AISelection `json:"lastModel,omitempty"`
	// Sound plays the completion chime; on unless turned off.
	Sound             bool            `json:"sound"`
	CollapsedProjects []string        `json:"collapsedProjects,omitempty"`
	Sections          map[string]bool `json:"sections,omitempty"`
	// Sidebar, Right and Bottom are the expanded widths and height, in
	// DIPs, of the sidebar and of the side panel docked right or below.
	Sidebar float32 `json:"sidebar"`
	Right   float32 `json:"right"`
	Bottom  float32 `json:"bottom"`
	Dock    string  `json:"dock,omitempty"` // "right" or "bottom"

	directory string
}

const (
	prefsFile = "desktop-chat-view.json"
	// minimumSidebar is the narrowest readable sidebar; only its button
	// hides it.
	minimumSidebar = 224
)

func defaultPrefs(directory string) prefs {
	return prefs{Version: 1, Sound: true, Sidebar: 240, Right: 420, Bottom: 300, Dock: "right", directory: directory}
}

// loadPrefs reads the saved choices; a missing or unreadable file, or a
// size out of range, is the default, as the React page's readers fell
// back.
func loadPrefs(env *core.Env) prefs {
	p := defaultPrefs(env.Directory)
	if env.Directory == "" {
		return p
	}
	data, err := os.ReadFile(filepath.Join(env.Directory, prefsFile))
	if err != nil || len(data) > 64<<10 {
		return p
	}
	var saved prefs
	if json.Unmarshal(data, &saved) != nil || saved.Version != 1 {
		return p
	}
	if saved.LastModel != nil && saved.LastModel.Provider != "" && saved.LastModel.Model != "" {
		p.LastModel = saved.LastModel
	}
	p.Sound = saved.Sound
	p.CollapsedProjects, p.Sections = saved.CollapsedProjects, saved.Sections
	size := func(v float32, fallback float32) float32 {
		if v >= 40 && v <= 2000 {
			return v
		}
		return fallback
	}
	p.Sidebar = max(minimumSidebar, size(saved.Sidebar, p.Sidebar))
	p.Right, p.Bottom = size(saved.Right, p.Right), size(saved.Bottom, p.Bottom)
	if saved.Dock == "bottom" {
		p.Dock = "bottom"
	}
	return p
}

// save writes the choices through a temporary file. A choice that cannot
// be written stays for this window, as React kept the in-memory choice
// when its storage was unavailable.
func (p *prefs) save() {
	if p.directory == "" {
		return
	}
	data, err := json.Marshal(p)
	if err != nil {
		return
	}
	temporary := filepath.Join(p.directory, ".desktop-chat-view-"+strconv.FormatInt(time.Now().UnixNano(), 36)+".tmp")
	if os.WriteFile(temporary, data, 0o600) != nil {
		return
	}
	if os.Rename(temporary, filepath.Join(p.directory, prefsFile)) != nil {
		_ = os.Remove(temporary)
	}
}

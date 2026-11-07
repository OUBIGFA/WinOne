// Package globalpages adapts Cindy 2d8d3c3's globalTabs, pluginPlacement and
// tabGroups to native MyGo containers. It stores only placement/view choices;
// plugin documents are always loaded afresh through the authenticated Host.
package globalpages

import (
	"encoding/json"
	"slices"
	"strings"
)

// Declaration is the public, approved page metadata supplied by the Host.
// HTML is a package-relative identity, never a filesystem or navigation grant.
type Declaration struct {
	PluginID        string       `json:"pluginId"`
	Page            string       `json:"page"`
	Title           string       `json:"title"`
	HTML            string       `json:"html"`
	Version         string       `json:"version"`
	Enabled         bool         `json:"enabled"`
	Approved        bool         `json:"approved"`
	State           string       `json:"state,omitempty"`
	Position        string       `json:"position,omitempty"`
	MinWidth        float32      `json:"minWidth,omitempty"`
	DefaultFraction float32      `json:"defaultFraction,omitempty"`
	Open            string       `json:"open,omitempty"`
	Side            string       `json:"side,omitempty"`
	Multiple        bool         `json:"multiple,omitempty"`
	Icon            string       `json:"icon,omitempty"`
	SettingsHeight  *float32     `json:"settingsHeight,omitempty"`
	Unread          *UnreadEntry `json:"unread,omitempty"`
}

// UnreadEntry is the original Host unread-store receipt. seenAt protects a
// newer notification from an older visible page's asynchronous clear.
type UnreadEntry struct {
	GhostID string `json:"ghostId"`
	Summary string `json:"summary,omitempty"`
	At      int64  `json:"at"`
}

func (d Declaration) available() bool           { return d.Enabled && d.Approved }
func declarationKey(plugin, page string) string { return plugin + ":" + page }

var sides = []string{"left", "right", "top", "bottom"}

// Location remembers the user's placement independently of whether a tab is
// currently open. Closing a panel must not erase where reopening puts it.
type Location struct {
	Mode   string `json:"mode"`
	Side   string `json:"side,omitempty"`
	Window string `json:"window,omitempty"`
}

func defaultLocation(d Declaration) Location {
	if d.Page == "main" {
		mode := d.Open
		if mode == "" {
			mode = "page"
		}
		side := d.Side
		if side == "" {
			side = "right"
		}
		return Location{Mode: mode, Side: side}
	}
	side := d.Position
	if d.Page == "task" {
		side = "right"
	} else if side == "" {
		side = "left"
	}
	return Location{Mode: "sidebar", Side: side}
}

// Tab is a global contribution or a task-owned page. Task IDs include their
// session; switching the main task cannot retarget a detached page's session.
type Tab struct {
	ID       string   `json:"id"`
	PluginID string   `json:"pluginId"`
	Page     string   `json:"page"`
	Session  string   `json:"session,omitempty"`
	Location Location `json:"location"`
}

func tabID(plugin, session string) string {
	if session != "" {
		return "task:" + session + ":" + plugin
	}
	return "g_" + plugin
}

type heightRecord struct {
	Version string  `json:"version"`
	Height  float32 `json:"height"`
}

// preferences mirrors the source's owner-scoped global tab/placement caches.
// No guest bytes, rendered snapshots, credentials or runtime URLs are stored.
type preferences struct {
	Version    int                     `json:"version"`
	Tabs       []Tab                   `json:"tabs"`
	Dismissed  map[string]bool         `json:"dismissed"`
	Placements map[string]Location     `json:"placements"`
	Navigation map[string]bool         `json:"navigation"`
	Sizes      map[string]float32      `json:"sizes"`
	Heights    map[string]heightRecord `json:"heights"`
}

func emptyPreferences() preferences {
	return preferences{Version: 1, Dismissed: map[string]bool{}, Placements: map[string]Location{}, Navigation: map[string]bool{}, Sizes: map[string]float32{}, Heights: map[string]heightRecord{}}
}

// parsePreferences rejects corrupt entries rather than turning an old file
// into a guest capability. Actual declarations are reconciled separately.
func parsePreferences(raw []byte) preferences {
	var saved preferences
	if len(raw) > 1<<20 || json.Unmarshal(raw, &saved) != nil || saved.Version != 1 {
		return emptyPreferences()
	}
	out := emptyPreferences()
	seen := map[string]bool{}
	for _, tab := range saved.Tabs {
		valid := tab.PluginID != "" && (tab.Page == "panel" || tab.Page == "main" || tab.Page == "task") &&
			(tab.Page == "task") == (tab.Session != "") && tab.ID == tabID(tab.PluginID, tab.Session) &&
			(tab.Location.Mode == "sidebar" || tab.Location.Mode == "window") && slices.Contains(sides, tab.Location.Side)
		if valid && !seen[tab.ID] {
			if tab.Location.Mode != "window" {
				tab.Location.Window = ""
			}
			seen[tab.ID] = true
			out.Tabs = append(out.Tabs, tab)
		}
	}
	for id, dismissed := range saved.Dismissed {
		if id != "" && dismissed {
			out.Dismissed[id] = true
		}
	}
	for key, place := range saved.Placements {
		if key != "" && (place.Mode == "page" || place.Mode == "sidebar" || place.Mode == "window") && slices.Contains(sides, place.Side) {
			out.Placements[key] = place
		}
	}
	for id, visible := range saved.Navigation {
		if id != "" {
			out.Navigation[id] = visible
		}
	}
	for side, size := range saved.Sizes {
		if slices.Contains(sides, side) && size >= 96 && size <= 2400 {
			out.Sizes[side] = size
		}
	}
	for key, height := range saved.Heights {
		if key != "" && height.Version != "" && height.Height > 0 {
			height.Height = min(800, max(48, height.Height))
			out.Heights[key] = height
		}
	}
	return out
}

type pageState struct {
	preferences
	declarations map[string]Declaration
	selected     map[string]string
}

func newPageState(saved preferences) pageState {
	return pageState{preferences: saved, declarations: map[string]Declaration{}, selected: map[string]string{}}
}

func (s *pageState) declaration(plugin, page string) (Declaration, bool) {
	d, exists := s.declarations[declarationKey(plugin, page)]
	return d, exists
}
func (s *pageState) available(tab Tab) bool {
	d, exists := s.declaration(tab.PluginID, tab.Page)
	return exists && d.available() && !(tab.Page == "panel" && d.Position == "tab")
}
func (s *pageState) sameMain(tab Tab, main Declaration) bool {
	d, exists := s.declaration(tab.PluginID, tab.Page)
	return tab.PluginID == main.PluginID && (tab.Page == "main" || exists && d.HTML == main.HTML)
}

// reconcile preserves asleep tabs and their placement but projects them out.
// Uninstall removes the owner and all its view preferences, so reinstall is a
// genuine first appearance. Non-tab panels appear automatically once enabled.
func (s *pageState) reconcile(rows []Declaration) {
	s.declarations = map[string]Declaration{}
	installed := map[string]bool{}
	for _, d := range rows {
		installed[d.PluginID] = true
		s.declarations[declarationKey(d.PluginID, d.Page)] = d
	}
	s.Tabs = slices.DeleteFunc(s.Tabs, func(tab Tab) bool {
		_, declared := s.declaration(tab.PluginID, tab.Page)
		return !declared
	})
	for id := range s.Dismissed {
		if !installed[id] {
			delete(s.Dismissed, id)
		}
	}
	for id := range s.Navigation {
		if !installed[id] {
			delete(s.Navigation, id)
		}
	}
	for key := range s.Placements {
		if _, present := s.declarations[key]; !present {
			delete(s.Placements, key)
		}
	}
	for key := range s.Heights {
		if _, present := s.declarations[key]; !present {
			delete(s.Heights, key)
		}
	}
	for _, d := range rows {
		if d.Page != "panel" || d.Position == "tab" || !d.available() || s.Dismissed[d.PluginID] {
			continue
		}
		if slices.ContainsFunc(s.Tabs, func(tab Tab) bool { return tab.ID == tabID(d.PluginID, "") }) {
			continue
		}
		place, placed := s.Placements[declarationKey(d.PluginID, "panel")]
		if !placed {
			place = defaultLocation(d)
		}
		s.Tabs = append(s.Tabs, Tab{ID: tabID(d.PluginID, ""), PluginID: d.PluginID, Page: "panel", Location: place})
	}
}

func (s *pageState) findTab(id string) *Tab {
	for i := range s.Tabs {
		if s.Tabs[i].ID == id {
			return &s.Tabs[i]
		}
	}
	return nil
}

func (s *pageState) openTab(d Declaration, session string, place Location) Tab {
	id := tabID(d.PluginID, session)
	tab := Tab{ID: id, PluginID: d.PluginID, Page: d.Page, Session: session, Location: place}
	if old := s.findTab(id); old != nil {
		*old = tab
	} else {
		s.Tabs = append(s.Tabs, tab)
	}
	if session == "" {
		delete(s.Dismissed, d.PluginID)
	}
	s.Placements[declarationKey(d.PluginID, d.Page)] = place
	s.selectTab(tab)
	return tab
}

func (s *pageState) selectTab(tab Tab) { s.selected[tab.Location.Side+"|"+tab.Session] = tab.ID }
func (s *pageState) closeTab(id string) {
	if tab := s.findTab(id); tab != nil && tab.Session == "" {
		s.Dismissed[tab.PluginID] = true
	}
	s.Tabs = slices.DeleteFunc(s.Tabs, func(tab Tab) bool { return tab.ID == id })
	for key, value := range s.selected {
		if value == id {
			delete(s.selected, key)
		}
	}
}
func (s *pageState) moveTab(id, side string) {
	if tab := s.findTab(id); tab != nil {
		tab.Location = Location{Mode: "sidebar", Side: side}
		s.Placements[declarationKey(tab.PluginID, tab.Page)] = tab.Location
		s.selectTab(*tab)
	}
}
func (s *pageState) resetPlacement(plugin, page string) {
	delete(s.Placements, declarationKey(plugin, page))
	if d, declared := s.declaration(plugin, page); declared {
		for i := range s.Tabs {
			if s.Tabs[i].PluginID == plugin && s.Tabs[i].Page == page {
				place := defaultLocation(d)
				// A sidebar tab being restored from a window returns to the
				// declared edge. Explicit page/window opens remain user actions.
				place.Mode = "sidebar"
				s.Tabs[i].Location = place
				s.selectTab(s.Tabs[i])
			}
		}
	}
}
func (s *pageState) navigationVisible(plugin string) bool {
	visible, saved := s.Navigation[plugin]
	return !saved || visible
}
func (s *pageState) group(side, session string) []Tab {
	var tabs []Tab
	for _, tab := range s.Tabs {
		if s.available(tab) && tab.Location.Mode == "sidebar" && tab.Location.Side == side && (tab.Session == "" || tab.Session == session) {
			tabs = append(tabs, tab)
		}
	}
	return tabs
}
func (s *pageState) selectedTab(side, session string, tabs []Tab) Tab {
	for _, key := range []string{side + "|" + session, side + "|"} {
		for _, tab := range tabs {
			if tab.ID == s.selected[key] {
				return tab
			}
		}
	}
	return tabs[0]
}
func (s *pageState) reorder(id, before string) {
	from := slices.IndexFunc(s.Tabs, func(tab Tab) bool { return tab.ID == id })
	to := slices.IndexFunc(s.Tabs, func(tab Tab) bool { return tab.ID == before })
	if from < 0 || to < 0 || from == to {
		return
	}
	tab := s.Tabs[from]
	s.Tabs = slices.Delete(s.Tabs, from, from+1)
	to = slices.IndexFunc(s.Tabs, func(tab Tab) bool { return tab.ID == before })
	s.Tabs = slices.Insert(s.Tabs, to, tab)
}

func sortedMainViews(rows map[string]Declaration) []Declaration {
	var result []Declaration
	for _, row := range rows {
		if row.Page == "main" && row.available() {
			result = append(result, row)
		}
	}
	slices.SortFunc(result, func(a, b Declaration) int {
		if order := strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)); order != 0 {
			return order
		}
		return strings.Compare(a.PluginID, b.PluginID)
	})
	return result
}

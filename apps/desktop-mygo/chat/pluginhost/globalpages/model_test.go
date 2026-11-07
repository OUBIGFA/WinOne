package globalpages

import (
	"encoding/json"
	"math"
	"testing"
)

func declaration(plugin, page string) Declaration {
	return Declaration{PluginID: plugin, Page: page, Title: plugin + " " + page, HTML: page + ".html", Version: "1.0.0", Enabled: true, Approved: true}
}
func TestAutoPanelsUseAllFourEdgesAndTabPanelsRemainExclusive(t *testing.T) {
	s := newPageState(emptyPreferences())
	rows := []Declaration{}
	for _, side := range sides {
		d := declaration(side, "panel")
		d.Position = side
		rows = append(rows, d)
	}
	tab := declaration("exclusive", "panel")
	tab.Position = "tab"
	rows = append(rows, tab)
	main := declaration("main", "main")
	rows = append(rows, main)
	s.reconcile(rows)
	if len(s.Tabs) != 4 {
		t.Fatalf("automatic contributions were dropped or a page became a panel: %+v", s.Tabs)
	}
	for _, side := range sides {
		group := s.group(side, "first")
		if len(group) != 1 || group[0].PluginID != side || len(s.group(side, "next")) != 1 {
			t.Fatalf("global edge follows the conversation: %s %+v", side, group)
		}
	}
	if s.findTab(tabID("exclusive", "")) != nil || s.findTab(tabID("main", "")) != nil {
		t.Fatal("tab panel or main view appeared without its user's open action")
	}
}
func TestClosedAndAsleepPanelsPreservePlacementAndReinstallIsFresh(t *testing.T) {
	s := newPageState(emptyPreferences())
	d := declaration("fixture", "panel")
	s.reconcile([]Declaration{d})
	s.moveTab(tabID(d.PluginID, ""), "bottom")
	s.closeTab(tabID(d.PluginID, ""))
	s.reconcile([]Declaration{d})
	if len(s.Tabs) != 0 {
		t.Fatal("a dismissed panel auto-reopened")
	}
	s.openTab(d, "", s.Placements[declarationKey(d.PluginID, "panel")])
	d.Enabled = false
	s.reconcile([]Declaration{d})
	if len(s.Tabs) != 1 || len(s.group("bottom", "other")) != 0 {
		t.Fatal("sleep deleted a tab or kept rendering its guest")
	}
	d.Enabled = true
	s.reconcile([]Declaration{d})
	if len(s.group("bottom", "other")) != 1 {
		t.Fatal("wake lost the user's placement")
	}
	s.reconcile(nil)
	s.reconcile([]Declaration{d})
	if len(s.group("left", "other")) != 1 || len(s.Placements) != 0 {
		t.Fatal("uninstall retained another install's position")
	}
}
func TestTaskTabsKeepTheirOwnSessionAndDeduplicateRepeatedOpens(t *testing.T) {
	s := newPageState(emptyPreferences())
	d := declaration("fixture", "task")
	s.reconcile([]Declaration{d})
	s.openTab(d, "first", Location{Mode: "sidebar", Side: "right"})
	s.openTab(d, "first", Location{Mode: "sidebar", Side: "right"})
	s.openTab(d, "next", Location{Mode: "sidebar", Side: "right"})
	if len(s.Tabs) != 2 || len(s.group("right", "first")) != 1 || len(s.group("right", "next")) != 1 {
		t.Fatalf("task buckets were conflated: %+v", s.Tabs)
	}
	first := s.findTab(tabID("fixture", "first"))
	first.Location.Mode, first.Location.Window = "window", "group"
	if len(s.group("right", "first")) != 0 || first.Session != "first" {
		t.Fatal("a detached task changed owner")
	}
}
func TestMainViewIdentityKeepsSeparatePanelHTMLAndNavigationIsOnlyPresentation(t *testing.T) {
	s := newPageState(emptyPreferences())
	panel, main := declaration("fixture", "panel"), declaration("fixture", "main")
	s.reconcile([]Declaration{panel, main})
	tab := *s.findTab(tabID("fixture", ""))
	if s.sameMain(tab, main) {
		t.Fatal("a distinct panel was mistaken for the main view")
	}
	main.HTML = panel.HTML
	if !s.sameMain(tab, main) {
		t.Fatal("the same HTML escaped single-instance protection")
	}
	s.Navigation[main.PluginID] = false
	if s.navigationVisible(main.PluginID) || !s.available(tab) {
		t.Fatal("hiding navigation disabled the plugin")
	}
}
func TestLayoutRecoveryKeepsOnlyValidPlacementAndNeverGuestContent(t *testing.T) {
	saved := emptyPreferences()
	saved.Tabs = []Tab{{ID: tabID("fixture", ""), PluginID: "fixture", Page: "panel", Location: Location{Mode: "window", Side: "top", Window: "shared"}}, {ID: "forged", PluginID: "fixture", Page: "panel", Location: Location{Mode: "window", Side: "top"}}}
	saved.Heights[declarationKey("fixture", "settings")] = heightRecord{Version: "1", Height: 900}
	raw, _ := json.Marshal(saved)
	restored := parsePreferences(raw)
	if len(restored.Tabs) != 1 || restored.Tabs[0].Location.Window != "shared" || restored.Heights[declarationKey("fixture", "settings")].Height != 800 {
		t.Fatalf("bad recovery: %+v", restored)
	}
	if invalid := parsePreferences([]byte(`{"version":1,"tabs":[{"id":"g_x","pluginId":"x","page":"settings","url":"http://outside"}]}`)); len(invalid.Tabs) != 0 {
		t.Fatal("persisted guest URL became a page grant")
	}
}
func TestAdaptiveHeightUsesActualBoundedGeometryNotTheFixedMinimum(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  float32
		valid bool
	}{{float64(20), 48, true}, {float64(100.2), 101, true}, {float64(160), 160, true}, {float64(1800), 800, true}, {float64(0), 0, false}, {math.NaN(), 0, false}, {math.Inf(1), 0, false}, {"800", 0, false}} {
		got, valid := adaptiveHeight(tc.value)
		if got != tc.want || valid != tc.valid {
			t.Fatalf("height(%v)=%v/%v", tc.value, got, valid)
		}
	}
}

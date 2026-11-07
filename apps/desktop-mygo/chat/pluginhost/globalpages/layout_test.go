package globalpages

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

// Only the private Host pipe is a fixture. All tab/resize/navigation controls
// are production native components; this is not the real WebView/window check.
func layoutFixture(t *testing.T, rows []Declaration, width int) (*testhost.Harness, *Controller, *ui.Tester) {
	t.Helper()
	h := testhost.New(t, func(method string, raw json.RawMessage) (any, error) {
		switch method {
		case "chat.ghosts.pages":
			return rows, nil
		case "chat.ghosts.page-session":
			return map[string]string{"url": "http://127.0.0.1:1/0000000000000000000000000000000000000000000000000000000000000000/__page_panel__"}, nil
		}
		return nil, fmt.Errorf("unexpected %s", method)
	})
	c := For(h.Env)
	c.loaded, c.language = true, i18n.Locale()
	c.Apply(rows)
	t.Cleanup(c.Close)
	tt := ui.NewTester(func(ctx *ui.Context) {
		ctx.SetTheme(kit.Theme(ctx.Preferences(), false))
		c.Layout(ctx, true, func() { ui.Box(ctx).Fill().Label("Actual conversation area") })
	}, width, 900)
	for range 3 {
		h.Settle(tt)
	}
	return h, c, tt
}
func TestNativeGlobalGroupsAndNarrowReflowAreNotModalPreviews(t *testing.T) {
	rows := []Declaration{}
	for _, side := range sides {
		d := declaration(side, "panel")
		d.Position = side
		rows = append(rows, d)
	}
	h, c, tt := layoutFixture(t, rows, 1400)
	for _, width := range []int{1400, 560, 1400} {
		tt.SetSize(width, 900)
		for range 3 {
			h.Settle(tt)
		}
		conversation, ok := tt.Find("Actual conversation area")
		if !ok || conversation.W < 250 || conversation.H < 250 {
			t.Fatalf("groups displaced the conversation at %d: %+v", width, conversation)
		}
		for _, side := range sides {
			group, ok := tt.Find(tr(side))
			if !ok || group.X < 0 || group.X+group.W > float32(width)+1 {
				t.Fatalf("%s group escaped %d: %+v", side, width, group)
			}
		}
	}
	if len(c.state.Tabs) != 4 {
		t.Fatal("responsive reflow discarded a declared panel")
	}
}
func TestNativeSplitterKeyboardAndCloseReopenKeepTheUserPlacement(t *testing.T) {
	d := declaration("fixture", "panel")
	d.Position = "left"
	h, c, tt := layoutFixture(t, []Declaration{d}, 1200)
	if err := tt.Click(tr("resize", "side", tr("left"))); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	h.Settle(tt)
	wanted := c.state.Sizes["left"]
	if wanted <= 0 {
		t.Fatal("the native splitter did not record its keyboard adjustment")
	}
	tt.SetSize(560, 900)
	h.Settle(tt)
	if c.state.Sizes["left"] != wanted {
		t.Fatal("narrow reflow overwrote the desired width")
	}
	c.Move(tabID(d.PluginID, ""), "bottom")
	c.CloseTab(tabID(d.PluginID, ""))
	c.Open(d.PluginID, "panel")
	h.Settle(tt)
	if tab := c.state.findTab(tabID(d.PluginID, "")); tab == nil || tab.Location.Side != "bottom" {
		t.Fatal("closing erased the panel's edge")
	}
	raw, err := os.ReadFile(filepath.Join(h.Env.Directory, "desktop-plugin-views.json"))
	if err != nil {
		t.Fatal(err)
	}
	saved := parsePreferences(raw)
	if saved.Placements[declarationKey(d.PluginID, "panel")].Side != "bottom" {
		t.Fatal("reopening placement was not saved")
	}
}
func TestMainViewUsesARealPageLifecycleAndSingleVersusMultipleIsRespected(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		t.Run(fmt.Sprint(multiple), func(t *testing.T) {
			panel, main := declaration("fixture", "panel"), declaration("fixture", "main")
			main.HTML, main.Multiple = panel.HTML, multiple
			h, c, _ := layoutFixture(t, []Declaration{panel, main}, 1200)
			c.OpenAt(main.PluginID, "main", Location{Mode: "page"})
			page := NewMainPage(h.Env).(*routePage)
			tt := ui.NewTester(page.View, 900, 700)
			h.Settle(tt)
			if c.main != page || page.body == nil || page.plugin != main.PluginID {
				t.Fatal("main view did not become the actual route controller")
			}
			if (c.state.findTab(tabID(main.PluginID, "")) != nil) != multiple {
				t.Fatal("same-view single/multiple policy was lost")
			}
			c.SetNavigationVisible(main.PluginID, false)
			if !c.state.declarations[declarationKey(main.PluginID, "main")].available() {
				t.Fatal("navigation preference disabled the main-view owner")
			}
			page.Close()
			if c.HasMainPage() || page.body != nil {
				t.Fatal("leaving the route retained the plugin page")
			}
			if intents := h.Env.TakeIntents(MainRoute); len(intents) != 0 {
				t.Fatal("route controller did not consume the actual navigation intent")
			}
		})
	}
}
func TestNativeNavigationVisibilityHasTheSameIconAndExpandedProjection(t *testing.T) {
	a, b := declaration("a", "main"), declaration("b", "main")
	h, c, _ := layoutFixture(t, []Declaration{a, b}, 1200)
	c.SetNavigationVisible("b", false)
	for _, collapsed := range []bool{false, true} {
		tt := ui.NewTester(func(ctx *ui.Context) { c.Navigation(ctx, collapsed) }, 240, 300)
		h.Settle(tt)
		if !tt.HasText(a.Title) || tt.HasText(b.Title) {
			t.Fatal("compact and expanded navigation disagree")
		}
		if err := tt.Click(a.Title); err != nil {
			t.Fatal(err)
		}
		intents := h.Env.TakeIntents(MainRoute)
		if len(intents) != 1 || !intents[0].Requested {
			t.Fatal("main navigation lost the real route intent")
		}
	}
}
func TestTabPanelLifetimeEndsWhenItsPluginPageIsLeft(t *testing.T) {
	d := declaration("exclusive", "panel")
	d.Position = "tab"
	h, c, _ := layoutFixture(t, []Declaration{d}, 1000)
	c.Open(d.PluginID, "panel")
	page := NewPanelPage(h.Env).(*routePage)
	tt := ui.NewTester(page.View, 900, 700)
	h.Settle(tt)
	if len(c.state.Tabs) != 0 || page.body == nil {
		t.Fatal("the exclusive tab panel became a global contribution")
	}
	page.Close()
	if page.body != nil || c.HasMainPage() {
		t.Fatal("leaving plugin page kept its exclusive panel alive")
	}
}

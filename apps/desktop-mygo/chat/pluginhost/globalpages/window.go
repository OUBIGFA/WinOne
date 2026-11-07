package globalpages

import (
	"crypto/sha256"
	"fmt"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
)

// A detached group is a real OS window in the same native UI process. Its
// cloned Env creates guests in that window, not in the original parent. Moving
// a tab closes its old guest before loading a fresh document in the new host.
type detachedWindow struct {
	owner    *Controller
	win      *mygo.Window
	env      *core.Env
	id       string
	tabs     []string
	selected string
	bodies   map[string]*pageBody
	closing  bool
}

func (c *Controller) detach(id string) {
	tab := c.state.findTab(id)
	if tab == nil || !c.state.available(*tab) {
		return
	}
	if current := c.windows[id]; current != nil {
		if current.win.IsMinimized() {
			current.win.Restore()
		}
		current.win.Show()
		current.win.Focus()
		return
	}
	if c.root == nil || c.root.IsDestroyed() {
		c.report(core.Errorf(tr("nativeHostUnavailable")))
		return
	}
	groupID := tab.Location.Window
	if groupID == "" {
		groupID = id
	}
	for _, current := range c.windows {
		if current.id == groupID {
			c.joinWindow(id, current)
			return
		}
	}
	if body := c.bodies[id]; body != nil {
		body.Close()
		delete(c.bodies, id)
	}
	d := c.state.declarations[declarationKey(tab.PluginID, tab.Page)]
	w := &detachedWindow{owner: c, id: groupID, tabs: []string{id}, selected: id, bodies: map[string]*pageBody{}}
	env := *c.env
	env.Post = func(fn func()) {
		if w.win != nil && !w.win.IsDestroyed() {
			w.win.Update(fn)
		}
	}
	env.WebViews = func(o mygo.WebViewOptions) (*mygo.WebView, error) {
		if w.win == nil || w.win.IsDestroyed() {
			return nil, core.Errorf(tr("nativeHostUnavailable"))
		}
		return w.win.NewWebView(o)
	}
	w.env = &env
	// MyGo's existing window-state store validates connected displays and
	// preserves size/maximized state. The key includes the profile owner.
	key := fmt.Sprintf("plugin-views-%x", sha256.Sum256([]byte(c.env.Directory+"\x00"+groupID)))
	w.win = mygo.NewWindow(mygo.WindowOptions{Title: d.Title, Width: 760, Height: 560, MinWidth: 320, MinHeight: 180, Parent: c.root, Hidden: true,
		BackgroundColor: "light-dark(#fcfcfc, #191919)", StateKey: key,
		Content: ui.View(func(ctx *ui.Context) {
			if w.win != nil {
				w.View(ctx)
			}
		}),
	})
	w.win.OnClose(func(event *mygo.CloseEvent) {
		if w.closing {
			return
		}
		event.PreventDefault()
		for _, tab := range append([]string(nil), w.tabs...) {
			c.CloseTab(tab)
		}
	})
	w.win.OnClosed(func() {
		if w.closing {
			return
		}
		w.closing = true
		for _, tab := range w.tabs {
			delete(c.windows, tab)
		}
		for _, body := range w.bodies {
			body.Close()
		}
		clear(w.bodies)
	})
	c.windows[id] = w
	tab.Location.Mode, tab.Location.Window = "window", groupID
	c.state.Placements[declarationKey(tab.PluginID, tab.Page)] = tab.Location
	c.maximized = ""
	c.save()
	w.win.Invalidate()
	w.win.Show()
	w.win.Focus()
}
func (c *Controller) joinWindow(id string, destination *detachedWindow) {
	if destination == nil || destination.closing {
		return
	}
	tab := c.state.findTab(id)
	if tab == nil || !c.state.available(*tab) {
		return
	}
	if c.windows[id] == destination {
		destination.selected = id
		destination.win.Invalidate()
		return
	}
	c.closeWindow(id, false)
	if body := c.bodies[id]; body != nil {
		body.Close()
		delete(c.bodies, id)
	}
	tab.Location.Mode, tab.Location.Window = "window", destination.id
	c.state.Placements[declarationKey(tab.PluginID, tab.Page)] = tab.Location
	destination.tabs = append(destination.tabs, id)
	destination.selected = id
	c.windows[id] = destination
	c.save()
	destination.win.Invalidate()
}
func (c *Controller) closeWindow(id string, forget bool) {
	w := c.windows[id]
	if w == nil {
		return
	}
	delete(c.windows, id)
	if body := w.bodies[id]; body != nil {
		body.Close()
		delete(w.bodies, id)
	}
	w.tabs = slices.DeleteFunc(w.tabs, func(tab string) bool { return tab == id })
	if forget {
		c.state.closeTab(id)
	}
	if len(w.tabs) == 0 {
		w.closing = true
		if !w.win.IsDestroyed() {
			w.win.Destroy()
		}
	} else {
		if w.selected == id {
			w.selected = w.tabs[0]
		}
		w.win.Invalidate()
	}
}
func (w *detachedWindow) View(ctx *ui.Context) {
	if w.closing {
		return
	}
	ctx.SetTheme(kit.Theme(ctx.Preferences(), ctx.Theme().Dark))
	var tabs []Tab
	for _, id := range w.tabs {
		if tab := w.owner.state.findTab(id); tab != nil && w.owner.state.available(*tab) {
			tabs = append(tabs, *tab)
		}
	}
	if len(tabs) == 0 {
		return
	}
	selected := tabs[0]
	for _, tab := range tabs {
		if tab.ID == w.selected {
			selected = tab
		}
	}
	w.selected = selected.ID
	for id, body := range w.bodies {
		body.SetVisible(id == selected.ID)
	}
	ui.Column(ctx).Fill().MinWidth(0).MinHeight(0).Background(ctx.Theme().Background).Children(func() {
		w.owner.tabBar(ctx, tabs, selected.ID, selected.Location.Side, w)
		ui.Column(ctx).Grow(1).Basis(0).FillWidth().MinWidth(0).MinHeight(0).Children(func() {
			body := w.bodies[selected.ID]
			d := w.owner.state.declarations[declarationKey(selected.PluginID, selected.Page)]
			if body == nil {
				body = newPageBody(w.owner, w.env, d, selected.Session)
				w.bodies[selected.ID] = body
			}
			body.SetVisible(true)
			body.View(ctx)
		})
	})
}

func (c *Controller) WindowCount() int {
	seen := map[*detachedWindow]bool{}
	for _, w := range c.windows {
		if !w.closing && !w.win.IsDestroyed() {
			seen[w] = true
		}
	}
	return len(seen)
}

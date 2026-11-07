package globalpages

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

const MainRoute = "plugin-main"
const PanelRoute = "plugin-panel"

// Controller is shared by the root, navigation entries, plugin manager and
// detached windows in this UI process. One owner prevents stale window copies
// from overwriting each other's placement or creating duplicate single views.
type Controller struct {
	env                     *core.Env
	state                   pageState
	loaded, loading, closed bool
	language, err           string
	session                 string
	area                    ui.Handle
	groups                  map[string]*ui.Handle
	bodyShown               map[string]bool
	bodies                  map[string]*pageBody
	windows                 map[string]*detachedWindow
	root                    *mygo.Window
	main                    *routePage
	settings                map[*SettingsPage]bool
	maximized, dragSource   string
	media                   *mediaPreview
	mediaGeneration         uint64
	attachments             map[string][]attachmentRequest
	pending                 []pendingOpen
}

type pendingOpen struct {
	plugin, page, session string
	place                 *Location
}

var registry = struct {
	sync.Mutex
	owners map[*core.Env]*Controller
}{owners: map[*core.Env]*Controller{}}

func For(env *core.Env) *Controller {
	registry.Lock()
	defer registry.Unlock()
	if c := registry.owners[env]; c != nil {
		return c
	}
	saved := emptyPreferences()
	if env.Directory != "" {
		if raw, err := os.ReadFile(filepath.Join(env.Directory, "desktop-plugin-views.json")); err == nil {
			saved = parsePreferences(raw)
		}
	}
	c := &Controller{env: env, state: newPageState(saved), groups: map[string]*ui.Handle{}, bodies: map[string]*pageBody{}, windows: map[string]*detachedWindow{}, settings: map[*SettingsPage]bool{}}
	registry.owners[env] = c
	return c
}

func (c *Controller) save() {
	// A placement change may originate in a detached window. Refresh the
	// shared root as well, otherwise returned/removed guests never remount
	// until some unrelated root-window input happens.
	if c.root != nil && !c.root.IsDestroyed() {
		c.root.Invalidate()
	}
	if c.env.Directory == "" {
		return
	}
	raw, err := json.Marshal(c.state.preferences)
	if err == nil {
		var file *os.File
		file, err = os.CreateTemp(c.env.Directory, ".plugin-views-*.tmp")
		if err == nil {
			name := file.Name()
			defer os.Remove(name)
			_, err = file.Write(raw)
			closed := file.Close()
			if err == nil {
				err = closed
			}
			if err == nil {
				err = os.Rename(name, filepath.Join(c.env.Directory, "desktop-plugin-views.json"))
			}
		}
	}
	if err != nil {
		c.report(err)
	}
}
func (c *Controller) report(err error) {
	if err != nil && !c.closed {
		c.err = err.Error()
		if c.env.Toast != nil {
			c.env.Toast(c.err)
		}
	}
}

// Refresh uses the full declaration snapshot, including asleep owners. It is
// called on first use, language changes and the existing Host change channel,
// not by a second independent background polling loop.
func (c *Controller) Refresh() {
	if c.closed || c.loading {
		return
	}
	c.loading = true
	language := i18n.Locale()
	core.Go(c.env, func() ([]Declaration, error) {
		var rows []Declaration
		err := c.env.Host.CallInto("chat.ghosts.pages", map[string]string{"language": language}, &rows)
		return rows, err
	}, func(rows []Declaration, err error) {
		if c.closed {
			return
		}
		c.loading = false
		if err != nil {
			c.report(err)
			return
		}
		c.loaded, c.language, c.err = true, language, ""
		c.Apply(rows)
		pending := c.pending
		c.pending = nil
		for _, request := range pending {
			switch {
			case request.session != "":
				c.OpenTask(request.plugin, request.session)
			case request.place != nil:
				c.OpenAt(request.plugin, request.page, *request.place)
			default:
				c.Open(request.plugin, request.page)
			}
		}
	})
}

func (c *Controller) waitForDeclarations(request pendingOpen) {
	for i, old := range c.pending {
		if old.plugin == request.plugin && old.page == request.page && old.session == request.session {
			c.pending[i] = request
			c.Refresh()
			return
		}
	}
	c.pending = append(c.pending, request)
	c.Refresh()
}

// Declaration and Declarations expose only the Host's public page metadata for
// the native manager. They never supply document bytes or absolute install paths.
func (c *Controller) Declaration(plugin, page string) (Declaration, bool) {
	return c.state.declaration(plugin, page)
}
func (c *Controller) Declarations() []Declaration {
	var rows []Declaration
	for _, d := range c.state.declarations {
		rows = append(rows, d)
	}
	return rows
}
func (c *Controller) Available(plugin, page string) bool {
	d, exists := c.state.declaration(plugin, page)
	return exists && d.available()
}

// Apply is the only installed-snapshot reconciliation point. Asleep owners
// keep their preferences but every real guest/window is immediately released.
func (c *Controller) Apply(rows []Declaration) {
	if c.closed {
		return
	}
	c.state.reconcile(rows)
	for id, body := range c.bodies {
		tab := c.state.findTab(id)
		if tab == nil || !c.state.available(*tab) || body.changed(c.state.declarations[declarationKey(tab.PluginID, tab.Page)]) {
			body.Close()
			delete(c.bodies, id)
		}
	}
	for id, window := range c.windows {
		tab := c.state.findTab(id)
		body := window.bodies[id]
		if tab == nil || !c.state.available(*tab) || body != nil && body.changed(c.state.declarations[declarationKey(tab.PluginID, tab.Page)]) {
			c.closeWindow(id, false)
		}
	}
	if c.main != nil {
		d, exists := c.state.declaration(c.main.plugin, c.main.page)
		if !exists || !d.available() {
			c.main.Close()
			if c.env.Navigate != nil {
				c.env.Navigate("chat", nil)
			}
		}
	}
	c.save()
}

func (c *Controller) Session(id string) { c.session = id }

func (c *Controller) Badge(plugin string, unread *UnreadEntry) {
	for key, row := range c.state.declarations {
		if row.PluginID != plugin {
			continue
		}
		if unread != nil && row.Unread != nil && row.Unread.At > unread.At {
			continue
		}
		row.Unread = unread
		c.state.declarations[key] = row
	}
}
func (c *Controller) HasMainPage() bool { return c.main != nil && !c.main.closed }
func (c *Controller) LeaveMain() {
	if c.main != nil {
		c.main.Close()
	}
}

func (c *Controller) Open(plugin, page string) {
	if !c.loaded {
		c.waitForDeclarations(pendingOpen{plugin: plugin, page: page})
		return
	}
	d, present := c.state.declaration(plugin, page)
	if !present || !d.available() {
		c.report(core.Errorf(tr("unavailable")))
		return
	}
	if page == "panel" && d.Position == "tab" {
		c.openRoute(plugin, PanelRoute)
		return
	}
	if page == "main" {
		// Existing matching content wins over a default position. A separate
		// panel HTML is not the same main view and must not steal this click.
		if tab := c.state.findTab(tabID(plugin, "")); tab != nil && c.state.available(*tab) && c.state.sameMain(*tab, d) {
			c.focusTab(*tab)
			return
		}
	}
	place, remembered := c.state.Placements[declarationKey(plugin, page)]
	if !remembered {
		place = defaultLocation(d)
	}
	c.OpenAt(plugin, page, place)
}

func (c *Controller) OpenAt(plugin, page string, place Location) {
	if !c.loaded {
		c.waitForDeclarations(pendingOpen{plugin: plugin, page: page, place: &place})
		return
	}
	d, present := c.state.declaration(plugin, page)
	if !present || !d.available() {
		c.report(core.Errorf(tr("unavailable")))
		return
	}
	if page == "panel" && d.Position == "tab" {
		c.openRoute(plugin, PanelRoute)
		return
	}
	if place.Mode == "page" {
		c.openRoute(plugin, MainRoute)
		return
	}
	apply := func() {
		if c.closed {
			return
		}
		if place.Side == "" {
			place.Side = defaultLocation(d).Side
		}
		tab := c.state.openTab(d, "", place)
		if old := c.bodies[tab.ID]; old != nil && old.changed(d) {
			old.Close()
			delete(c.bodies, tab.ID)
		}
		if c.main != nil && !d.Multiple && c.main.plugin == plugin && (page == "main" || c.main.html == d.HTML) {
			// The accepted ordinary route transition releases the old native
			// page/controller; hiding it over an unrelated shell would not.
			c.main.Close()
		}
		if place.Mode == "window" {
			c.detach(tab.ID)
		} else {
			c.closeWindow(tab.ID, false)
			c.state.moveTab(tab.ID, place.Side)
		}
		c.save()
	}
	if c.HasMainPage() && !d.Multiple && c.main.plugin == plugin && (page == "main" || c.main.html == d.HTML) && c.env.Navigate != nil {
		c.env.Navigate("chat", apply)
	} else if c.env.RequestChange != nil {
		c.env.RequestChange(apply)
	} else {
		apply()
	}
}

func (c *Controller) openRoute(plugin, route string) {
	if c.env.Open == nil {
		c.report(core.Errorf(tr("unavailable")))
		return
	}
	raw, _ := json.Marshal(map[string]string{"pluginId": plugin})
	c.env.Open(route, core.Intent{Requested: true, Options: raw})
}

// OpenTask is deliberately task-scoped and does not navigate. Host focus:true
// has already consumed the real three-second gesture before emitting a reveal.
func (c *Controller) OpenTask(plugin, session string) {
	if !c.loaded {
		c.waitForDeclarations(pendingOpen{plugin: plugin, page: "task", session: session})
		return
	}
	d, present := c.state.declaration(plugin, "task")
	if !present || !d.available() || session == "" {
		return
	}
	if tab := c.state.findTab(tabID(plugin, session)); tab != nil {
		c.focusTab(*tab)
		return
	}
	place := defaultLocation(d)
	if remembered, exists := c.state.Placements[declarationKey(plugin, "task")]; exists {
		place = remembered
	}
	tab := c.state.openTab(d, session, place)
	if place.Mode == "window" {
		c.detach(tab.ID)
	}
	c.save()
}
func (c *Controller) focusTab(tab Tab) {
	c.state.selectTab(tab)
	if window := c.windows[tab.ID]; window != nil && !window.win.IsDestroyed() {
		if window.win.IsMinimized() {
			window.win.Restore()
		}
		window.selected = tab.ID
		window.win.Show()
		window.win.Focus()
		window.win.Invalidate()
	}
	if body := c.bodies[tab.ID]; body != nil {
		body.Focus()
	}
	c.save()
}
func (c *Controller) CloseTab(id string) {
	c.closeWindow(id, false)
	if body := c.bodies[id]; body != nil {
		body.Close()
		delete(c.bodies, id)
	}
	c.state.closeTab(id)
	if c.maximized == id {
		c.maximized = ""
	}
	c.save()
}
func (c *Controller) Move(id, side string) {
	c.closeWindow(id, false)
	c.state.moveTab(id, side)
	c.maximized = ""
	c.save()
}
func (c *Controller) RestoreDefault(plugin, page string) {
	for _, tab := range append([]Tab(nil), c.state.Tabs...) {
		if tab.PluginID == plugin && tab.Page == page {
			c.closeWindow(tab.ID, false)
		}
	}
	c.state.resetPlacement(plugin, page)
	c.maximized = ""
	c.save()
}
func (c *Controller) SetNavigationVisible(plugin string, visible bool) {
	c.state.Navigation[plugin] = visible
	c.save()
}
func (c *Controller) NavigationVisible(plugin string) bool { return c.state.navigationVisible(plugin) }

// Close is root-window cleanup, not plugin disable/uninstall. Scheduled Host
// work keeps running; window/tab placements survive the UI process restarting.
func (c *Controller) Close() {
	if c.closed {
		return
	}
	c.save()
	c.closed = true
	for id := range c.windows {
		c.closeWindow(id, false)
	}
	for _, body := range c.bodies {
		body.Close()
	}
	clear(c.bodies)
	if c.main != nil {
		c.main.Close()
	}
	for settings := range c.settings {
		settings.Close()
	}
	c.closeMedia()
	registry.Lock()
	if registry.owners[c.env] == c {
		delete(registry.owners, c.env)
	}
	registry.Unlock()
}

// routePage is an actual shell Page, so ordinary navigation/unsaved guards and
// Close release it. It is never a fullscreen-looking dialog over the old page.
type routePage struct {
	owner                     *Controller
	page, route, plugin, html string
	body                      *pageBody
	closed                    bool
}

func NewMainPage(env *core.Env) core.Page {
	return &routePage{owner: For(env), page: "main", route: MainRoute}
}
func NewPanelPage(env *core.Env) core.Page {
	return &routePage{owner: For(env), page: "panel", route: PanelRoute}
}
func (p *routePage) Close() {
	if p.closed {
		return
	}
	p.closed = true
	if p.body != nil {
		p.body.Close()
		p.body = nil
	}
	if p.owner.main == p {
		p.owner.main = nil
	}
}
func (p *routePage) View(ctx *ui.Context) {
	if p.closed {
		return
	}
	c := p.owner
	for _, intent := range c.env.TakeIntents(p.route) {
		var target struct {
			PluginID string `json:"pluginId"`
		}
		if json.Unmarshal(intent.Options, &target) != nil || target.PluginID == "" {
			continue
		}
		d, exists := c.state.declaration(target.PluginID, p.page)
		if !exists || !d.available() {
			c.err = tr("unavailable")
			continue
		}
		if p.body != nil {
			p.body.Close()
		}
		p.plugin, p.html = target.PluginID, d.HTML
		if p.page == "main" && !d.Multiple {
			for _, tab := range append([]Tab(nil), c.state.Tabs...) {
				if tab.Session == "" && c.state.sameMain(tab, d) {
					c.CloseTab(tab.ID)
				}
			}
		}
		p.body = newPageBody(c, c.env, d, "")
		c.main = p
	}
	if p.body != nil {
		p.body.View(ctx)
	} else if c.err != "" {
		ui.Text(ctx, c.err).MinWidth(0)
	}
}

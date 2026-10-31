package aiservices

import (
	"errors"
	"net/url"
	"regexp"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/web"
)

// hostPage is a page the Host serves for one tab: opened when the tab
// shows, closed when it leaves, opened again after a save replaced the
// configuration the page was made for.
type hostPage struct {
	p          *page
	open       string // capture.ai.<open> / .<close>
	close      string
	options    web.Options
	surface    *web.Surface
	url        string
	revision   float64
	opened     bool
	busy       bool
	err        string
	read       int
}

// show opens the page for the saved revision.
func (h *hostPage) show(revision float64) {
	if h.busy || (h.opened && h.revision == revision) || h.err != "" {
		return
	}
	h.release()
	h.read++
	owner := h.read
	h.busy, h.revision = true, revision
	p := h.p
	core.Go(p.env, func() (string, error) {
		var result struct {
			URL string `json:"url"`
		}
		err := p.env.Host.CallCompletingInto(h.open, nil, &result)
		return result.URL, err
	}, func(address string, err error) {
		h.busy = false
		if owner != h.read {
			// The tab left while it opened: close what opened.
			if err == nil {
				spawn(p.env, func() { _ = p.env.Host.CallCompletingInto(h.close, nil, nil) })
			}
			return
		}
		if err != nil {
			h.err = err.Error()
			return
		}
		h.opened, h.url = true, address
		options := h.options
		options.URL = address
		h.surface = web.New(p.env, options)
	})
}

// release closes the page and the Host's process for it.
func (h *hostPage) release() {
	h.read++
	if h.surface != nil {
		h.surface.Close()
		h.surface = nil
	}
	if h.opened {
		h.opened = false
		p := h.p
		spawn(p.env, func() { _ = p.env.Host.CallCompletingInto(h.close, nil, nil) })
	}
	h.busy = false
}

// retry opens the page again after an error.
func (h *hostPage) retry() { h.err = "" }

// routingPanel is Magpie's complete routing page, its editor, rules and
// live trace, below WinOne's switch for routing.
type routingPanel struct {
	hostPage
}

func newRoutingPanel(p *page) *routingPanel {
	r := &routingPanel{hostPage{p: p, open: "capture.ai.routing.open", close: "capture.ai.routing.close"}}
	r.options = web.Options{Label: i18n.T("routing.title"), Theme: "winone-routing-theme", Height: 600,
		Accept:     []string{"winone-routing-height", "winone-routing-masking", "winone-routing-native"},
		HeightType: "winone-routing-height", Receive: r.receive}
	return r
}

func (r *routingPanel) receive(m web.Message) {
	p := r.p
	switch m.String("type") {
	case "winone-routing-masking":
		// "See the record" opens the privacy records at it, in the same
		// draft.
		if p.locked() {
			return
		}
		p.maskRecord = m.String("key")
		p.advancedTab = advancedPrivacy
		p.switchTab(tabAdvanced)
	case "winone-routing-native":
		id := m.String("id")
		if id == "" {
			return
		}
		reply := func(err error) {
			result := map[string]any{"type": "winone-routing-native-result", "id": id}
			if err != nil {
				result["error"] = err.Error()
			}
			if r.surface != nil {
				r.surface.Post(result)
			}
		}
		// The routing page may only turn routing groups on.
		if m.String("method") != "enable-groups" {
			reply(errors.New(i18n.T("aiPlugins.unsupportedAction")))
			return
		}
		if p.locked() {
			reply(errors.New(i18n.T("aiPlugins.busyAction")))
			return
		}
		p.enableRoutingGroups(reply)
	}
}

func (r *routingPanel) Close() { r.release() }

// Leave closes the page when the routing tab no longer shows.
func (r *routingPanel) Leave() {
	if r.opened || r.busy || r.surface != nil {
		r.release()
	}
}

func (r *routingPanel) View(c *ui.Context) {
	p := r.p
	if page := p.extensions.replacement("routing"); page != nil {
		r.Leave()
		p.extensions.Page(c, *page, nil, p.locked())
		return
	}
	if p.saved != nil {
		r.show(p.saved.Value.Revision)
	}
	kit.Panel(c, func() {
		enabled := p.draft != nil && p.draft.Value.RoutingEnabled
		if kit.Toggle(c, &enabled, i18n.T("routing.enabled"), i18n.T("routing.hint")).Disabled(r.busy || p.locked() || p.draft == nil).Changed() {
			p.draft.Value.RoutingEnabled = enabled
		}
	})
	if r.err != "" {
		kit.Notice(c, kit.Failure, "", r.err, func() {
			if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
				r.retry()
			}
		})
	}
	if r.busy && r.surface == nil {
		kit.Status(c, i18n.T("capture.loading"))
	}
	if r.surface != nil {
		r.surface.View(c, p.locked())
	}
}

// pluginsPanel is the third-party plugin manager: installation and the
// plugins' own operations, which apply at once. Only its own page may ask
// WinOne to choose a folder or open an account connection.
type pluginsPanel struct {
	hostPage
	request *pageRequest
	shown   string // the nonce of the request the page was loaded for
}

func newPluginsPanel(p *page) *pluginsPanel {
	x := &pluginsPanel{hostPage: hostPage{p: p, open: "capture.ai.routing.plugins.open", close: "capture.ai.routing.plugins.close"}}
	x.options = web.Options{Label: i18n.T("aiPlugins.title"), Theme: "winone-plugin-theme", Height: 600,
		Accept:     []string{"winone-plugin-height", "winone-plugin-changed", "winone-plugin-native"},
		HeightType: "winone-plugin-height", Receive: x.receive}
	return x
}

func (x *pluginsPanel) Close() { x.release() }

func (x *pluginsPanel) Leave() {
	if x.opened || x.busy || x.surface != nil {
		x.release()
		x.shown = ""
	}
}

// pageURL adds a page request to the manager's address. Context is
// rebuilt from public identity, never a provider object or credential.
func pageURL(base string, r *pageRequest) string {
	u, err := url.Parse(base)
	if err != nil || r == nil {
		return base
	}
	q := u.Query()
	q.Set("page", r.key)
	q.Set("request", r.nonce)
	if r.action != "" && r.menu != "" && r.item != nil {
		q.Set("action", r.action)
		q.Set("menu", r.menu)
		q.Set("item-id", r.item.ID)
		q.Set("item-name", r.item.Name)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (x *pluginsPanel) receive(m web.Message) {
	p := x.p
	switch m.String("type") {
	case "winone-plugin-changed":
		// Only a successful registry change refreshes account discovery.
		p.extensions.Reload()
		p.reader.InvalidateCatalog()
		p.reader.Invalidate()
		p.catalog.loaded = false
	case "winone-plugin-native":
		id := m.String("id")
		if id == "" {
			return
		}
		reply := func(result any, err error) {
			message := map[string]any{"type": "winone-plugin-native-result", "id": id}
			if err != nil {
				message["error"] = err.Error()
			} else {
				message["result"] = result
			}
			if x.surface != nil {
				x.surface.Post(message)
			}
		}
		// A request posted before the page was locked still gets an
		// answer, so the page's promise never waits forever.
		if p.locked() {
			reply(nil, errors.New(i18n.T("aiPlugins.busyAction")))
			return
		}
		switch m.String("method") {
		case "choose":
			core.Go(p.env, func() (string, error) { return p.env.Dialogs.PickFolder(i18n.T("aiPlugins.title")) },
				func(dir string, err error) { reply(dir, err) })
		case "provider":
			agentID := m.String("provider")
			if agentID == "" {
				reply(nil, errors.New(i18n.T("aiPlugins.unsupportedAction")))
				return
			}
			name := m.String("name")
			if name == "" {
				name = agentID
			}
			signIn, _ := m["signIn"].(bool)
			reply(nil, p.openPluginProvider(agent{ID: agentID, Name: name, Backend: "plugin", Plugin: m.String("plugin")}, signIn))
		default:
			reply(nil, errors.New(i18n.T("aiPlugins.unsupportedAction")))
		}
	}
}

// openPluginProvider shows a plugin's account type on the accounts tab:
// its existing connection, or a new one; signIn starts its sign-in once
// the connection's methods are known.
func (p *page) openPluginProvider(a agent, signIn bool) error {
	if p.draft == nil {
		return errors.New(i18n.T("capture.loading"))
	}
	var existing *provider
	for i := range p.draft.Value.Providers {
		pr := &p.draft.Value.Providers[i].Value
		if pr.AuthMode == "account" && pr.Agent == a.ID && pr.AgentBackend == "plugin" {
			existing = pr
			break
		}
	}
	if existing == nil && len(p.draft.Value.Providers) >= maxProviders {
		return errors.New(i18n.T("aiPlugins.providerLimit"))
	}
	p.switchTab(tabAccounts)
	p.providerTab = providerConnection
	id := ""
	if existing != nil {
		id = existing.ID
	} else {
		id = p.add(&a)
	}
	p.selected = id
	if signIn {
		p.extensions.signIn = &signInRequest{provider: id, nonce: nonce()}
	}
	return nil
}

func (x *pluginsPanel) View(c *ui.Context) {
	p := x.p
	if p.saved != nil {
		x.show(p.saved.Value.Revision)
	}
	if x.err != "" {
		kit.Notice(c, kit.Failure, "", x.err, func() {
			if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
				x.retry()
			}
		})
	}
	if x.busy && x.surface == nil {
		kit.Status(c, i18n.T("aiPlugins.loading"))
	}
	if x.surface == nil {
		return
	}
	// Each request loads the manager at its page anew, as React's iframe
	// keyed by the request did.
	nonce := ""
	if x.request != nil {
		nonce = x.request.nonce
	}
	if nonce != x.shown {
		x.shown = nonce
		x.surface.Load(pageURL(x.url, x.request))
	}
	x.surface.View(c, p.locked())
}

// maskKinds are masking kinds, as Magpie names them.
var maskKinds = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

package aiservices

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/web"
)

// titled is a plugin's title, with translations.
type titled struct {
	Title  string            `json:"title"`
	Titles map[string]string `json:"titles,omitempty"`
}

// title picks the translation of the window's language.
func (t titled) title() string {
	locale := i18n.Locale()
	switch {
	case strings.HasPrefix(locale, "zh-CN"):
		locale = "zh"
	case strings.HasPrefix(locale, "en"):
		locale = "en"
	}
	if v, ok := t.Titles[locale]; ok {
		return v
	}
	if v, ok := t.Titles[strings.Split(locale, "-")[0]]; ok {
		return v
	}
	return t.Title
}

// pluginAction is an action a plugin page offers on a provider or an
// account type.
type pluginAction struct {
	titled
	ID   string `json:"id"`
	Menu string `json:"menu"`
}

// pluginPage is a plugin's page, which may replace the routing or usage
// tab.
type pluginPage struct {
	titled
	Key      string         `json:"key"`
	Replaces string         `json:"replaces,omitempty"`
	Actions  []pluginAction `json:"actions,omitempty"`
}

type pluginPanel struct {
	titled
	Key  string `json:"key"`
	Slot string `json:"slot"`
}

// pluginExtensions are the pages and panels installed plugins add, served
// at url by the Host's plugin process.
type pluginExtensions struct {
	URL    string        `json:"url"`
	Pages  []pluginPage  `json:"pages"`
	Panels []pluginPanel `json:"panels"`
}

// pageRequest opens a plugin page in the plugin manager, for an action on
// a provider or account type.
type pageRequest struct {
	key, action, menu string
	item              *named
	nonce             string
}

// signInRequest continues a sign-in the plugin manager asked for.
type signInRequest struct{ provider, nonce string }

func nonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// extensions holds the page's lease of the plugin process, which one
// viewer ID owns, so a late reply or close never closes a newer page's
// runtime. It opens once the configuration is saved, and again after a
// save or a plugin change.
type extensions struct {
	env      *core.Env
	state    *pluginExtensions
	err      string
	viewer   string
	revision float64
	gen      int
	opened   int // the gen the lease is for, -1 before one
	surfaces map[string]*web.Surface
	signIn   *signInRequest
	// open shows the plugin manager at a page; the page sets it.
	open func(*pageRequest)
}

func newExtensions(env *core.Env) *extensions {
	return &extensions{env: env, opened: -1, surfaces: map[string]*web.Surface{}}
}

// Follow opens the lease for the saved revision, replacing an older one.
func (x *extensions) Follow(revision float64) {
	if x.opened == x.gen && x.revision == revision {
		return
	}
	x.release()
	x.revision, x.opened = revision, x.gen
	viewer := nonce()
	x.viewer, x.err, x.state = viewer, "", nil
	core.Go(x.env, func() (pluginExtensions, error) {
		var value pluginExtensions
		err := x.env.Host.CallCompletingInto("capture.ai.routing.extensions.open", map[string]string{"viewer": viewer}, &value)
		if err == nil && (value.URL == "" || value.Pages == nil || value.Panels == nil) {
			err = errors.New(i18n.T("aiPlugins.noPlugins"))
		}
		return value, err
	}, func(value pluginExtensions, err error) {
		if x.viewer != viewer {
			// A newer lease replaced this one while it opened.
			if err == nil {
				x.close(viewer)
			}
			return
		}
		if err != nil {
			x.err = err.Error()
			return
		}
		x.state = &value
	})
}

// Reload opens the lease again, after a plugin was installed or changed.
func (x *extensions) Reload() { x.gen++ }

func (x *extensions) close(viewer string) {
	spawn(x.env, func() {
		_ = x.env.Host.CallCompletingInto("capture.ai.routing.extensions.close", map[string]string{"viewer": viewer}, nil)
	})
}

// release ends the lease and the surfaces it served.
func (x *extensions) release() {
	for key, s := range x.surfaces {
		s.Close()
		delete(x.surfaces, key)
	}
	if x.viewer != "" {
		x.close(x.viewer)
		x.viewer = ""
	}
	x.state = nil
}

func (x *extensions) Close() { x.release() }

// replacement is the plugin page that replaces a tab, if any.
func (x *extensions) replacement(tab string) *pluginPage {
	if x.state == nil {
		return nil
	}
	for i := range x.state.Pages {
		if x.state.Pages[i].Replaces == tab {
			return &x.state.Pages[i]
		}
	}
	return nil
}

// surface returns the surface for a page or slot of the plugin process,
// made the first time it shows. Context is rebuilt from public identity:
// never a provider, account, draft or credential.
func (x *extensions) surface(query url.Values, height float32, label string) *web.Surface {
	base, err := url.Parse(x.state.URL)
	if err != nil {
		return nil
	}
	q := base.Query()
	for key, values := range query {
		q[key] = values
	}
	base.RawQuery = q.Encode()
	address := base.String()
	s := x.surfaces[address]
	if s == nil {
		s = web.New(x.env, web.Options{URL: address, Label: label, Theme: "winone-plugin-theme",
			Accept: []string{"winone-plugin-height"}, HeightType: "winone-plugin-height", Height: height})
		x.surfaces[address] = s
	}
	return s
}

// Slot shows the panels plugins add to a place of the page; nothing when
// none does.
func (x *extensions) Slot(c *ui.Context, slot string, item *named, disabled bool) {
	if x.state == nil {
		return
	}
	found := false
	for _, panel := range x.state.Panels {
		found = found || panel.Slot == slot
	}
	if !found {
		return
	}
	query := url.Values{"surface": {"1"}, "slot": {slot}}
	if item != nil {
		query.Set("item-id", item.ID)
		query.Set("item-name", item.Name)
	}
	if s := x.surface(query, 60, i18n.T("aiPlugins.panels")); s != nil {
		s.View(c, disabled)
	}
}

// Page shows a plugin page in place of a tab.
func (x *extensions) Page(c *ui.Context, page pluginPage, item *named, disabled bool) {
	if x.state == nil {
		return
	}
	query := url.Values{"surface": {"1"}, "page": {page.Key}}
	if item != nil {
		query.Set("item-id", item.ID)
		query.Set("item-name", item.Name)
	}
	if s := x.surface(query, 600, page.title()); s != nil {
		s.View(c, disabled)
	}
}

// providerMenu builds the plugins' actions for a saved provider and, for
// an account, its type; nil when no plugin offers one.
func (x *extensions) providerMenu(old *provider, catalog catalogState) func(*ui.Menu) {
	if x.state == nil || old == nil {
		return nil
	}
	type group struct {
		menu, label string
		item        named
		actions     []struct {
			page   pluginPage
			action pluginAction
		}
	}
	groups := []group{{menu: "provider", label: "aiPlugins.providerActions", item: named{ID: old.ID, Name: old.Name}}}
	if old.AuthMode == "account" {
		name := old.Agent
		if a := catalog.agent(old.Agent); a != nil {
			name = a.Name
		}
		groups = append(groups, group{menu: "agent", label: "aiPlugins.agentActions", item: named{ID: old.Agent, Name: name}})
	}
	found := false
	for i := range groups {
		for _, page := range x.state.Pages {
			for _, action := range page.Actions {
				if action.Menu == groups[i].menu {
					groups[i].actions = append(groups[i].actions, struct {
						page   pluginPage
						action pluginAction
					}{page, action})
					found = true
				}
			}
		}
	}
	if !found {
		return nil
	}
	return func(m *ui.Menu) {
		first := true
		for _, g := range groups {
			if len(g.actions) == 0 {
				continue
			}
			if !first {
				m.Separator()
			}
			first = false
			m.Item(i18n.T(g.label)).Disabled(true)
			for _, a := range g.actions {
				if m.Item(a.action.title()).Chosen() {
					item := g.item
					x.openPage(&pageRequest{key: a.page.Key, action: a.action.ID, menu: g.menu, item: &item, nonce: nonce()})
				}
			}
		}
	}
}

// openPage is set by the page: it shows the plugin manager at a page.
func (x *extensions) openPage(r *pageRequest) {
	if x.open != nil {
		x.open(r)
	}
}

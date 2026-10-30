package shell

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// pageSpec is one page of the sidebar.
type pageSpec struct {
	id     string
	title  string // i18n key
	icon   *ui.SVG
	layout core.Layout
	// footer pages sit at the bottom of the sidebar; separated marks the
	// first page after the divider there.
	footer, separated bool
	make              func(env *core.Env) core.Page
}

// registry lists every page in the sidebar's order: Home, the modules in
// their shared order (module-order.ts), OneBot, then Chat, Flash chat, AI
// services and Settings at the bottom. Pages without make show the
// unavailable state until they are built.
var registry []pageSpec

// Register adds or replaces the constructor of a page; page packages call
// it from init through the window's page list.
func Register(id string, layout core.Layout, make func(env *core.Env) core.Page) {
	for i := range registry {
		if registry[i].id == id {
			registry[i].make, registry[i].layout = make, layout
			return
		}
	}
	panic("shell: unknown page " + id)
}

func init() {
	registry = []pageSpec{
		{id: "overview", title: "nav.overview", icon: icons.House},
		{id: "capture", title: "nav.capture", icon: icons.ScanLine},
		{id: "selection", title: "nav.selection", icon: icons.MousePointer2},
		{id: "quick-assistant", title: "nav.quick-assistant", icon: icons.Astroid},
		{id: "interconnect", title: "interconnect.title", icon: icons.FolderUp},
		{id: "wintray", title: "nav.wintray", icon: icons.LaptopMinimal},
		{id: "convenience", title: "nav.convenience", icon: icons.Rocket},
		{id: "onebot", title: "onebot.title", icon: icons.Panda, layout: core.Conversation},
		{id: "chat", title: "chat.title", icon: icons.Bot, layout: core.Conversation, footer: true},
		{id: "flash-chat", title: "interconnect.peerChat", icon: icons.MessagesCircle, layout: core.Conversation, footer: true},
		{id: "ai-services", title: "ai.serviceTitle", icon: icons.Sparkles, footer: true, separated: true},
		{id: "settings", title: "nav.settings", icon: icons.Settings, footer: true},
	}
}

func pages() []pageSpec { return registry }

func spec(id string) pageSpec {
	for _, p := range registry {
		if p.id == id {
			return p
		}
	}
	return registry[0]
}

// unavailable is the state of a page that has no view yet.
func unavailable(env *core.Env, s pageSpec) core.Page {
	return core.PageFunc(func(c *ui.Context) {
		kit.Empty(c, s.icon, i18n.T("shell.unavailable"), i18n.T("shell.unavailableHint"), func() {
			if kit.Button(c, i18n.T("common.back"), nil, false).Clicked() {
				env.Navigate("overview", nil)
			}
		})
	})
}

// PageInfo describes a page of the sidebar for other pages, as the
// default page setting lists them.
type PageInfo struct {
	ID string
	// Title is the page's i18n key.
	Title string
	Icon  *ui.SVG
}

// Catalog lists the sidebar's pages in its order.
func Catalog() []PageInfo {
	catalog := make([]PageInfo, len(registry))
	for i, p := range registry {
		catalog[i] = PageInfo{ID: p.id, Title: p.title, Icon: p.icon}
	}
	return catalog
}

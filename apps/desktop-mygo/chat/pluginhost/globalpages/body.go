package globalpages

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/pluginhost/pluginpage"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

type pageBody struct {
	owner                    *Controller
	env                      *core.Env
	decl                     Declaration
	session, locale          string
	page                     *pluginpage.Page
	loading, closed, visible bool
	err                      string
	off                      []func()
	ctx                      context.Context
	cancel                   context.CancelFunc
	heightCancel             context.CancelFunc
	height                   float32
	readAttempt              int64
}

func newPageBody(owner *Controller, env *core.Env, d Declaration, session string) *pageBody {
	ctx, cancel := context.WithCancel(context.Background())
	p := &pageBody{owner: owner, env: env, decl: d, session: session, locale: i18n.Locale(), ctx: ctx, cancel: cancel, visible: true, height: 160}
	if record, exists := owner.state.Heights[declarationKey(d.PluginID, "settings")]; exists && record.Version == d.Version {
		p.height = record.Height
	}
	if d.SettingsHeight != nil {
		p.height = *d.SettingsHeight
	}
	return p
}
func (p *pageBody) changed(d Declaration) bool {
	return p.decl.HTML != d.HTML || p.decl.Version != d.Version || p.decl.Page != d.Page || p.locale != i18n.Locale()
}
func (p *pageBody) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.cancel()
	if p.heightCancel != nil {
		p.heightCancel()
	}
	for _, off := range p.off {
		off()
	}
	p.off = nil
	if p.page != nil {
		p.page.Close()
		p.page = nil
	}
}
func (p *pageBody) SetVisible(visible bool) {
	if p.visible != visible {
		p.readAttempt = 0
	}
	p.visible = visible
	if p.page != nil {
		p.page.SetVisible(visible)
	}
}
func (p *pageBody) Focus() {
	if p.page != nil && !p.closed {
		p.page.Focus()
	}
}

func (p *pageBody) load() {
	if p.loading || p.closed || p.page != nil {
		return
	}
	p.loading = true
	params := map[string]string{"pluginId": p.decl.PluginID, "page": p.decl.Page, "language": p.locale}
	if p.session != "" {
		params["sessionId"] = p.session
	}
	method := "chat.ghosts.page-session"
	if p.decl.Page == "settings" {
		method = "chat.ghosts.settings-session"
	}
	core.Go(p.env, func() (json.RawMessage, error) { return p.env.Host.Call(method, params) }, func(raw json.RawMessage, err error) {
		if p.closed {
			return
		}
		p.loading = false
		var reply struct{ URL, Title string }
		if err == nil {
			err = json.Unmarshal(raw, &reply)
		}
		if err == nil {
			p.page, err = pluginpage.New(p.env, reply.URL, p.decl.Title, p.decl.PluginID)
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		p.page.SetVisible(p.visible)
		web := p.page.WebView().Page()
		p.off = append(p.off,
			web.OnDidFailLoad(func(err *mygo.LoadError) {
				if !p.closed {
					p.err = err.Error()
				}
			}),
			web.OnRenderProcessGone(func(string) {
				if !p.closed {
					p.err = tr("pageCrashed")
				}
			}),
		)
		if p.decl.Page == "settings" && p.decl.SettingsHeight == nil {
			// The initial IsLoading=false is not navigation completion, and
			// DOMReady depends on MyGo's in-document bridge. Secure plugin
			// pages need the native load event before observing their geometry.
			p.off = append(p.off, web.OnDidFinishLoad(p.observeHeight))
		} else if p.decl.Page != "settings" {
			p.off = append(p.off, web.OnContextMenu(p.mediaMenu))
		}
	})
}
func (p *pageBody) View(c *ui.Context) {
	if p.closed || !p.visible {
		return
	}
	if p.page == nil && p.err == "" {
		p.load()
	}
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		if p.err != "" {
			kit.InlineError(c, p.err)
			if kit.Button(c, tr("retry"), nil, false).Clicked() {
				if p.page != nil {
					p.page.Close()
					p.page = nil
				}
				for _, off := range p.off {
					off()
				}
				p.off = nil
				if p.heightCancel != nil {
					p.heightCancel()
					p.heightCancel = nil
				}
				p.err = ""
			}
			return
		}
		if p.loading {
			kit.Loader(c)
		}
		if p.page != nil {
			p.page.View(c)
			p.readPanelBadge()
		}
	})
}

// Only a genuinely visible, focused panel reads its own badge. Main/task
// views do not clear panel attention, and rendering alone grants no receipt.
func (p *pageBody) readPanelBadge() {
	if p.decl.Page != "panel" || !p.visible || p.page == nil || !p.page.IsFocused() {
		return
	}
	current, exists := p.owner.state.declaration(p.decl.PluginID, "panel")
	if !exists || current.Unread == nil || current.Unread.At == p.readAttempt {
		return
	}
	seenAt := current.Unread.At
	p.readAttempt = seenAt
	core.Go(p.env, func() (bool, error) {
		var result struct {
			Cleared bool `json:"cleared"`
		}
		err := p.env.Host.CallInto("chat.ghosts.badge-clear", map[string]any{"pluginId": p.decl.PluginID, "seenAt": seenAt}, &result)
		return result.Cleared, err
	}, func(cleared bool, err error) {
		if p.closed {
			return
		}
		if err != nil {
			p.owner.report(err)
			return
		}
		if row, exists := p.owner.state.declaration(p.decl.PluginID, "panel"); cleared && exists && row.Unread != nil && row.Unread.At == seenAt {
			p.owner.Badge(p.decl.PluginID, nil)
		}
	})
}

// Adaptive sizing ports GhostSettingsWebview's geometry, not a supplied guest
// height. ResizeObserver is awaited through the native EvalContext channel;
// there is no guest Go bridge and no steady polling/Eval timer.
const settingsPrepare = `(() => {
 const id='__winone_settings_layout'; if(!document.getElementById(id)) {
  const style=document.createElement('style'); style.id=id;
  style.textContent='html,body{width:100%!important;max-width:100%!important;overflow-x:hidden!important;height:auto!important;min-height:0!important}*,*::before,*::after{box-sizing:border-box!important;min-width:0!important;max-width:100%!important}';
  (document.head||document.documentElement).append(style);
 }
 return true;
})()`
const settingsNextHeight = `(async () => {
 const de=document.documentElement,b=document.body;
 const read=()=>{
  if(!b)return de?de.scrollHeight:0;
  const sy=window.scrollY||(de?de.scrollTop:0)||0;
  let bottom=b.getBoundingClientRect().bottom+(parseFloat(getComputedStyle(b).marginBottom)||0);
  for(const child of b.children){const r=child.getBoundingClientRect();if(r.height>0)bottom=Math.max(bottom,r.bottom+(parseFloat(getComputedStyle(child).marginBottom)||0))}
  const height=sy+bottom;if(de)de.style.overflowY=height>800?'auto':'hidden';return height;
 };
 let height=read();
 if(window.__winoneSettingsLastHeight!==height){window.__winoneSettingsLastHeight=height;return height}
 return await new Promise(resolve=>{
  const observer=new ResizeObserver(()=>{const next=read();if(next!==window.__winoneSettingsLastHeight){window.__winoneSettingsLastHeight=next;observer.disconnect();resolve(next)}});
  observer.observe(b||de);
 });
})()`

func adaptiveHeight(value any) (float32, bool) {
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
		return 0, false
	}
	return float32(min(800, max(48, math.Ceil(number)))), true
}
func (p *pageBody) observeHeight() {
	if p.closed || p.page == nil || p.decl.SettingsHeight != nil {
		return
	}
	if p.heightCancel != nil {
		p.heightCancel()
	}
	ctx, cancel := context.WithCancel(p.ctx)
	p.heightCancel = cancel
	page := p.page.WebView().Page()
	// The long observer wait is lifecycle work, not a user action or a pending
	// Host mutation. It terminates with its document and never holds Save busy.
	go func() {
		if _, err := page.EvalContext(ctx, settingsPrepare); err != nil {
			return
		}
		for ctx.Err() == nil {
			value, err := page.EvalContext(ctx, settingsNextHeight)
			if err != nil {
				return
			}
			height, valid := adaptiveHeight(value)
			if !valid {
				return
			}
			p.env.Post(func() {
				if p.closed || ctx.Err() != nil {
					return
				}
				p.height = height
				p.owner.state.Heights[declarationKey(p.decl.PluginID, "settings")] = heightRecord{Version: p.decl.Version, Height: height}
				p.owner.save()
			})
		}
	}()
}

// Native WebView2 context-menu hit data is a real input event. Only candidate
// media URLs are offered; the Host still checks its ledger and captured page
// owner before returning a path to copy or reveal.
func (p *pageBody) mediaMenu(event *mygo.ContextMenuEvent) {
	if p.closed || !p.visible || p.page == nil || !p.page.IsFocused() {
		return
	}
	uri := ""
	for _, candidate := range []string{event.ImageURL, event.MediaURL, event.LinkURL} {
		if strings.Contains(candidate, "/media/") || strings.Contains(candidate, "/preview/") {
			uri = candidate
			break
		}
	}
	if uri == "" {
		return
	}
	pageID := p.page.ID()
	items := []*mygo.MenuItem{}
	for _, action := range []string{"copy", "reveal"} {
		label := i18n.T("chat.fileMenuCopyFile")
		if action == "reveal" {
			label = i18n.T("chat.revealFile")
		}
		items = append(items, &mygo.MenuItem{Label: label, Click: func(*mygo.MenuItem, *mygo.Window) {
			if p.closed || p.page == nil || p.page.ID() != pageID {
				return
			}
			core.Do(p.env, func() error {
				return p.env.Host.CallInto("chat.ghosts.panel-media-action", map[string]string{"pageId": pageID, "sourcePluginId": p.decl.PluginID, "uri": uri, "action": action}, nil)
			}, func(err error) {
				if !p.closed && p.page != nil && p.page.ID() == pageID {
					p.owner.report(err)
				}
			})
		}})
	}
	event.Menu = mygo.NewMenu(items)
}

// TaskBody adapts older persisted workspace plugin tabs to the same current
// declaration/visibility gate as the new four-edge task containers.
type TaskBody struct {
	owner           *Controller
	env             *core.Env
	plugin, session string
	body            *pageBody
	closed, visible bool
}

func NewTaskBody(env *core.Env, plugin, session string) *TaskBody {
	return &TaskBody{owner: For(env), env: env, plugin: plugin, session: session, visible: true}
}
func (p *TaskBody) Close() {
	p.closed = true
	if p.body != nil {
		p.body.Close()
		p.body = nil
	}
}
func (p *TaskBody) SetVisible(value bool) {
	p.visible = value
	if p.body != nil {
		p.body.SetVisible(value)
	}
}
func (p *TaskBody) View(c *ui.Context) {
	if p.closed || !p.visible {
		return
	}
	if !p.owner.loaded {
		p.owner.Refresh()
		return
	}
	d, exists := p.owner.state.declaration(p.plugin, "task")
	if !exists || !d.available() {
		if p.body != nil {
			p.body.Close()
			p.body = nil
		}
		return
	}
	if p.body != nil && p.body.changed(d) {
		p.body.Close()
		p.body = nil
	}
	if p.body == nil {
		p.body = newPageBody(p.owner, p.env, d, p.session)
	}
	p.body.SetVisible(p.visible)
	p.body.View(c)
}

// SettingsPage reuses the same secure page but gives it its actual content
// height. A fixed declaration is not clamped to the adaptive minimum of 48.
type SettingsPage struct {
	owner  *Controller
	env    *core.Env
	plugin string
	body   *pageBody
	closed bool
}

func NewSettingsPage(env *core.Env, plugin string) (*SettingsPage, error) {
	owner := For(env)
	page := &SettingsPage{owner: owner, env: env, plugin: plugin}
	owner.settings[page] = true
	// A fresh management click may precede the installed-change poll. Read
	// its real declaration, never fabricate settings metadata from list HTML.
	owner.Refresh()
	return page, nil
}
func (p *SettingsPage) View(c *ui.Context) {
	if p.closed {
		return
	}
	d, exists := p.owner.state.declaration(p.plugin, "settings")
	if !exists || !d.available() {
		if p.body != nil {
			p.body.Close()
			p.body = nil
		}
		if p.owner.loading {
			kit.Loader(c)
		} else {
			kit.InlineError(c, tr("unavailable"))
			if kit.Button(c, tr("retry"), nil, false).Clicked() {
				p.owner.Refresh()
			}
		}
		return
	}
	if p.body != nil && p.body.changed(d) {
		p.body.Close()
		p.body = nil
	}
	if p.body == nil {
		p.body = newPageBody(p.owner, p.env, d, "")
	}
	ui.Column(c).FillWidth().MinWidth(0).Height(p.body.height).Children(func() { p.body.View(c) })
}
func (p *SettingsPage) Close() {
	p.closed = true
	delete(p.owner.settings, p)
	if p.body != nil {
		p.body.Close()
		p.body = nil
	}
}
func (p *SettingsPage) Height() float32 {
	if p.body != nil {
		return p.body.height
	}
	return 160
}

package aiservices

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// proxyDraft is the proxy form: automatic, direct, or an address with its
// login kept apart. It applies to the page's draft on demand, as React's
// form did; until then leaving asks about it.
type proxyDraft struct {
	mode, address, username, password string
	fields                            proxyParts
}

func readProxy(s magpieSettings) proxyDraft {
	value := s.Proxy
	mode := "custom"
	switch value {
	case "":
		mode = "auto"
	case "direct":
		mode = "direct"
	}
	source := s.ProxyCustom
	if mode == "custom" {
		source = value
	}
	f := proxyFields(source)
	return proxyDraft{mode: mode, address: f.Address, username: f.Username, password: f.Password, fields: f}
}

func (d proxyDraft) differs(o proxyDraft) bool {
	return d.mode != o.mode || d.mode == "custom" && (d.address != o.address || d.username != o.username || d.password != o.password)
}

// syncProxy reads the form again when the settings it came from change:
// after a save, an undo or a reload.
func (a *advancedPanel) syncProxy() proxyDraft {
	s := a.p.draft.Value.magpie()
	original := readProxy(s)
	if source := s.Proxy + "\x00" + s.ProxyCustom; source != a.proxyOf {
		a.proxyOf, a.proxy, a.proxyErr = source, original, ""
	}
	return original
}

func (a *advancedPanel) reportProxy(shown bool) {
	if a.p.draft == nil {
		a.p.env.Activity.Report("ai-proxy", false, false, nil)
		return
	}
	original := a.syncProxy()
	a.p.env.Activity.Report("ai-proxy", a.proxy.differs(original), false, func() {
		a.proxy, a.proxyErr = readProxy(a.p.draft.Value.magpie()), ""
	})
}

// applyProxy puts the form into the draft; the page's Save saves it.
func (a *advancedPanel) applyProxy() {
	s := a.settings()
	a.proxyErr = ""
	if a.proxy.mode != "custom" {
		s.Proxy = ""
		if a.proxy.mode == "direct" {
			s.Proxy = "direct"
		}
		return
	}
	if strings.TrimSpace(a.proxy.address) == "" {
		a.proxyErr = i18n.T("ai.proxyAddressRequired")
		return
	}
	proxy, ok := proxyAddress(a.proxy.address, a.proxy.fields, a.proxy.username, a.proxy.password)
	if !ok {
		a.proxyErr = i18n.T("ai.proxyEncodingError")
		return
	}
	s.Proxy, s.ProxyCustom = proxy, proxy
}

// proxyView is the proxy form. Magpie's helpers parse and encode the
// login, so a pasted address with a login splits into its fields.
func (a *advancedPanel) proxyView(c *ui.Context, locked bool) {
	original := a.syncProxy()
	a.reportProxy(true)
	d := &a.proxy
	kit.Group(c, icons.Network, i18n.T("ai.proxyTitle"), i18n.T("ai.proxyHint"), func() {
		choiceRow(c, i18n.T("ai.proxyMode"), "", &d.mode, options("auto", i18n.T("ai.proxyAuto"), "direct", i18n.T("ai.proxyDirect"),
			"custom", i18n.T("ai.proxyCustom")), locked)
		if d.mode == "custom" {
			kit.Stack(c, i18n.T("ai.proxyAddress"), "", func() {
				if ui.TextInput(c, &d.address).FillWidth().Placeholder("http://127.0.0.1:7890").Label(i18n.T("ai.proxyAddress")).
					Disabled(locked).Changed() {
					if f := proxyFields(d.address); f.Username != "" || f.Password != "" {
						d.fields, d.address, d.username, d.password = f, f.Address, f.Username, f.Password
					}
				}
				ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().Children(func() {
					ui.Field(c, i18n.T("ai.proxyUsername"), func() {
						ui.TextInput(c, &d.username).FillWidth().Label(i18n.T("ai.proxyUsername")).Disabled(locked)
					}).Basis(200).Grow(1)
					ui.Field(c, i18n.T("ai.proxyPassword"), func() {
						ui.TextInput(c, &d.password).FillWidth().Password().Label(i18n.T("ai.proxyPassword")).Disabled(locked)
					}).Basis(200).Grow(1)
				})
			})
		}
		kit.Stack(c, "", "", func() {
			if a.proxyErr != "" {
				kit.Notice(c, kit.Failure, "", a.proxyErr, nil)
			}
			ui.Row(c).FillWidth().Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
				if kit.Button(c, i18n.T("ai.proxyApply"), nil, false).Disabled(locked || !d.differs(original)).Clicked() {
					a.applyProxy()
				}
				kit.Help(c, i18n.T("ai.proxySaveHint"))
			})
		})
	})
}

package automations

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Readiness is read from the same Host connector mapping used for execution.
// It is not an account login claim; actual delivery errors stay on each run.
type supportState struct {
	Feishu struct {
		Status string `json:"status"`
	} `json:"feishu"`
	WecomGroup   struct{ Configured, Enabled bool } `json:"wecomGroup"`
	Capabilities map[string]string                  `json:"-"`
}

func (p *Page) RefreshSupport() {
	if p.closed {
		return
	}
	if p.supportLoading {
		p.supportAgain = true
		return
	}
	p.supportLoading = true
	core.Go(p.env, func() (supportState, error) {
		var state supportState
		if err := p.call("channels", nil, &state); err != nil {
			return state, err
		}
		var capabilities struct {
			Statuses []struct {
				Capability string `json:"capability"`
				State      string `json:"state"`
			} `json:"statuses"`
		}
		if err := p.call("capabilityStatus", nil, &capabilities); err != nil {
			return state, err
		}
		state.Capabilities = map[string]string{}
		for _, status := range capabilities.Statuses {
			state.Capabilities[status.Capability] = status.State
		}
		return state, nil
	}, func(state supportState, err error) {
		if p.closed {
			return
		}
		p.supportLoading = false
		defer func() {
			if p.supportAgain {
				p.supportAgain = false
				p.RefreshSupport()
			}
		}()
		p.supportLoaded = err == nil
		if err != nil {
			p.supportError = err.Error()
			return
		}
		p.support = state
		p.supportError = ""
	})
}
func (p *Page) configureConnections() {
	if p.options.ConfigureConnections == nil {
		return
	}
	// Keep both the form and the destination unchanged while the owner decides.
	p.env.RequestChange(func() { p.Discard(); p.options.ConfigureConnections() })
}
func (p *Page) notificationsFields(c *ui.Context, d *Draft) {
	hint := tr("connectorUnconfigured")
	if p.supportLoading {
		hint = tr("connectorLoading")
	} else if p.supportError != "" {
		hint = tr("connectorReadFailed")
	}
	feishu := p.supportLoaded && p.support.Feishu.Status == "connected"
	wecom := p.supportLoaded && p.support.WecomGroup.Configured && p.support.WecomGroup.Enabled
	kit.Group(c, nil, tr("notifications"), "", func() {
		kit.Toggle(c, &d.Value.SilentWhenIdle, tr("silent"), tr("silentHint"))
		kit.Toggle(c, &d.Value.Notify.Desktop, tr("desktopNotify"), "")
		explain := hint
		if feishu {
			explain = tr("connectorConfigured")
		}
		disabled := !feishu && !d.Value.Notify.Feishu
		kit.Toggle(c, &d.Value.Notify.Feishu, tr("feishuNotify"), explain).Disabled(disabled)
		explain = hint
		if wecom {
			explain = tr("connectorConfigured")
		}
		disabled = !wecom && !d.Value.Notify.WecomGroup
		kit.Toggle(c, &d.Value.Notify.WecomGroup, tr("wecomNotify"), explain).Disabled(disabled)
	})
	if p.supportError != "" {
		kit.InlineError(c, p.supportError)
	}
	p.connectionActions(c)
}
func (p *Page) connectionActions(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if p.options.ConfigureConnections != nil && kit.Button(c, tr("configureConnections"), icons.Settings2, false).Clicked() {
			p.configureConnections()
		}
		if kit.GhostButton(c, tr("refreshConnections"), icons.RefreshCw, p.supportLoading).Clicked() {
			p.RefreshSupport()
		}
	})
}

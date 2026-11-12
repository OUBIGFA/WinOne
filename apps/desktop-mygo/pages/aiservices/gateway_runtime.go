package aiservices

import (
	"crypto/rand"
	"errors"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// Only gateway and account diagnostics need the private management listener.
// Other AI tabs load no background service just to render their navigation.
func (g *gatewayPanel) ensureRuntime() {
	if g.runtimeURL != "" || g.runtimeLoading || g.runtimeError != "" {
		return
	}
	g.runtimeLoading = true
	viewer := rand.Text()
	g.viewer = viewer
	core.Go(g.env, func() (string, error) {
		var value struct {
			URL string `json:"url"`
		}
		err := g.env.Host.CallCompletingInto("capture.ai.gateway.open", map[string]string{"viewer": viewer}, &value)
		if err == nil && value.URL == "" {
			err = errors.New(i18n.T("aiNative.invalidRuntime"))
		}
		return value.URL, err
	}, func(address string, err error) {
		if viewer != g.viewer {
			if err == nil {
				g.releaseViewer(viewer)
			}
			return
		}
		g.runtimeLoading = false
		if err != nil {
			g.runtimeError = err.Error()
			return
		}
		g.runtimeURL = address
	})
}

func (g *gatewayPanel) releaseViewer(viewer string) {
	core.Go(g.env, func() (bool, error) {
		err := g.env.Host.CallCompletingInto("capture.ai.gateway.close", map[string]string{"viewer": viewer}, nil)
		return err == nil, err
	}, func(_ bool, err error) {
		if err != nil {
			g.env.Toast(err.Error())
		}
	})
}

// Invalidating a saved snapshot also invalidates the private capability. A
// late reply can release only its own lease, never the replacement window.
func (g *gatewayPanel) releaseRuntime() {
	if g.viewer != "" {
		g.releaseViewer(g.viewer)
	}
	g.viewer, g.runtimeURL, g.runtimeError = "", "", ""
	g.runtimeLoading = false
	g.calls.close()
	g.calls = gatewayCalls{}
	g.access.close()
	g.history.api.close()
}

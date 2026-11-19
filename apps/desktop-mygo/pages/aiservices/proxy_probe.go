package aiservices

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type proxyProbeResult struct {
	OK    bool   `json:"ok"`
	MS    int64  `json:"ms"`
	IP    string `json:"ip"`
	Loc   string `json:"loc"`
	Error string `json:"error"`
	Why   string `json:"why"`
	Addr  string `json:"addr"`
	Was   string `json:"was"`
	Now   string `json:"now"`
}
type proxyProbePanel struct {
	value  string
	busy   bool
	err    string
	result *proxyProbeResult
}

func (e *editor) proxyProbeView(c *ui.Context, key, choice string, locked bool) {
	if e.proxyProbes == nil {
		e.proxyProbes = map[string]*proxyProbePanel{}
	}
	w := e.proxyProbes[key]
	if w == nil {
		w = &proxyProbePanel{}
		e.proxyProbes[key] = w
	}
	if w.value != choice {
		w.value, w.err, w.result = choice, "", nil
	}
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		if kit.Button(c, i18n.T("aiSync.proxyTest"), icons.Activity, w.busy).Disabled(locked || w.busy).Clicked() {
			w.busy, w.err, w.result = true, "", nil
			core.Go(e.p.env, func() (proxyProbeResult, error) {
				var result proxyProbeResult
				err := e.p.env.Host.CallCompletingInto("capture.ai.proxies.test", map[string]string{"proxy": choice}, &result)
				return result, err
			}, func(result proxyProbeResult, err error) {
				w.busy = false
				if e.closed || w.value != choice {
					return
				}
				if err != nil {
					w.err = err.Error()
					return
				}
				w.result = &result
			})
		}
		kit.Help(c, i18n.T("aiSync.proxyTestHint"))
		kit.InlineError(c, w.err)
		if result := w.result; result != nil {
			if result.OK {
				kit.Help(c, i18n.T("aiSync.proxyTestResult", "ip", result.IP, "region", result.Loc, "ms", result.MS))
			} else {
				why := strings.NewReplacer("{addr}", result.Addr, "{was}", result.Was, "{now}", result.Now).Replace(result.Why)
				if why != "" && result.Error != "" {
					why += " · "
				}
				kit.InlineError(c, why+result.Error)
			}
		}
	})
}

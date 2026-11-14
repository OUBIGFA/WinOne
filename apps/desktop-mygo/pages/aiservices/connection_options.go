package aiservices

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

type serverCertificate struct {
	Host, SHA256, Subject, Issuer, Why string
	NotBefore, NotAfter                time.Time
	Checks                             bool
}

// These are the actual transport options of the draft connection. The
// upstream provider applies them to discovery, tests and inference alike.
func (e *editor) connectionOptions(c *ui.Context, pr *provider, locked bool) {
	s, p := &pr.ProviderPreferences, e.p
	kit.Group(c, icons.Network, nativeT("resolve"), nativeT("resolveHint"), func() {
		for _, host := range sortedKeys(s.Resolve) {
			kit.Stack(c.Key(host), host, "", func() {
				values := s.Resolve[host]
				e.preferenceList(c, "resolve:"+host, host, &values, locked)
				s.Resolve[host] = values
				if kit.GhostButton(c, i18n.T("common.delete")+" "+host, icons.Trash2, false).Disabled(locked).Clicked() {
					delete(s.Resolve, host)
				}
			})
		}
		kit.Stack(c, "", "", func() {
			field := e.fields.get("newResolveHost")
			ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				kit.TextInput(c, &field.text).Grow(1).MinWidth(180).Label(nativeT("resolveHost")).Placeholder("api.example.com").Disabled(locked)
				host := strings.ToLower(strings.TrimSpace(field.text))
				if kit.Button(c, nativeT("addResolve"), icons.Plus, false).Disabled(locked || host == "" || len(s.Resolve) >= 32).Clicked() {
					if s.Resolve == nil {
						s.Resolve = map[string][]string{}
					}
					if _, exists := s.Resolve[host]; !exists {
						s.Resolve[host] = []string{}
					}
					field.text = ""
				}
			})
		})
	})
	kit.Group(c, icons.ShieldCheck, nativeT("certificates"), nativeT("certificatesHint"), func() {
		for _, field := range []struct {
			key   string
			value *string
		}{{"clientCert", &s.ClientCert}, {"clientKey", &s.ClientKey}, {"serverCA", &s.ServerCA}} {
			kit.Stack(c.Key(field.key), nativeT(field.key), "", func() {
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					input := kit.TextInput(c, field.value).Grow(1).MinWidth(0).Label(nativeT(field.key)).Disabled(locked)
					accept := func(paths []string, err error) {
						if p.editor != e {
							return
						}
						if err != nil {
							p.err = err.Error()
							return
						}
						if len(paths) > 0 {
							*field.value = paths[0]
						}
					}
					widgets.DropArea(c, p.env, input, widgets.DropFiles, false, locked, accept)
					if kit.IconButton(c, icons.FolderOpen, nativeT("chooseCertificate")).Disabled(locked).Clicked() {
						p.connectionBusy = true
						core.Go(p.env, func() ([]string, error) {
							path, err := p.env.Dialogs.PickFile(nativeT(field.key), core.Filter{Name: "PEM", Extensions: []string{"pem", "crt", "cer", "key"}})
							if err != nil || path == "" {
								return nil, err
							}
							return []string{path}, nil
						}, func(paths []string, err error) { p.connectionBusy = false; accept(paths, err) })
					}
				})
			})
		}
		kit.Stack(c, nativeT("serverPin"), nativeT("serverPinHint"), func() { kit.TextInput(c, &s.ServerPin).FillWidth().Label(nativeT("serverPin")).Disabled(locked) })
		// A certificate read belongs to these endpoints and transport options.
		// Editing the draft hides the previous read before it can be trusted.
		source, _ := json.Marshal(map[string]any{"base": pr.BaseURL, "protocol": pr.Protocol, "endpoints": s.Endpoints, "proxy": s.Proxy, "resolve": s.Resolve, "cert": s.ClientCert, "key": s.ClientKey, "ca": s.ServerCA})
		if string(source) != e.certificateSource {
			e.serverCertificate = nil
		}
		kit.Stack(c, "", "", func() {
			if kit.Button(c, nativeT("readCertificate"), icons.RefreshCw, p.connectionBusy).Disabled(locked || pr.BaseURL == "").Clicked() {
				p.connectionBusy, p.err = true, ""
				payload := p.connectionPayload(pr)
				core.Go(p.env, func() (serverCertificate, error) {
					var out struct {
						Certificate serverCertificate `json:"certificate"`
					}
					err := p.env.Host.CallCompletingInto("capture.ai.models.certificate", payload, &out)
					return out.Certificate, err
				}, func(seen serverCertificate, err error) {
					p.connectionBusy = false
					if p.editor != e {
						return
					}
					if err != nil {
						p.err = err.Error()
						return
					}
					e.serverCertificate, e.certificateSource = &seen, string(source)
				})
			}
			if seen := e.serverCertificate; seen != nil {
				kit.Help(c, seen.Host+" · "+seen.Subject).Selectable()
				kit.Help(c, seen.SHA256).Selectable()
				kit.Help(c, seen.NotBefore.Local().Format("2006-01-02")+" — "+seen.NotAfter.Local().Format("2006-01-02"))
				if seen.Why != "" {
					kit.Help(c, seen.Why).Selectable()
				}
				if kit.Button(c, nativeT("trustCertificate"), icons.ShieldCheck, false).Disabled(locked || seen.Checks || s.ServerPin == seen.SHA256).Clicked() {
					s.ServerPin = seen.SHA256
				}
			}
		})
	})
}

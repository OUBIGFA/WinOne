package aiservices

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// providerKey is upstream's access.ProviderKey: a gateway key as a
// provider's editor shows it, with how far it reaches the provider.
type providerKey struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Off   bool     `json:"off,omitempty"`
	Reach string   `json:"reach"`
	Only  bool     `json:"only,omitempty"`
	Via   []string `json:"via,omitempty"`
}

// The provider's side of the gateway keys' models. Ticks are each key's own
// Models (upstream access.SetProviderKeys), so this list and the gateway key
// editor always agree. Nothing is read until asked; a tick is written at once
// because it changes gateway keys, not the provider draft.
type providerKeysPanel struct {
	api               nativeAPI
	requested, loaded bool
	keys              []providerKey
}

func (s *providerKeysPanel) accept(raw json.RawMessage) {
	var state struct {
		Keys []providerKey `json:"keys"`
	}
	if nativeDecode(&s.api, raw, &state) {
		s.keys, s.loaded = state.Keys, true
	}
}

func providerKeyReaches(k providerKey) bool {
	return k.Reach == "all" || k.Reach == "whole" || k.Reach == "some"
}

// providerKeyNote is upstream's noteOf, in its words.
func providerKeyNote(k providerKey) string {
	off := ""
	if k.Off {
		off = upstreamT("Off") + " · "
	}
	switch {
	case k.Reach == "all":
		return off + upstreamT("Every provider; unticked, it keeps the others there are now")
	case k.Only:
		return off + upstreamT("Only this provider: change it on the Gateway page")
	case k.Reach == "whole":
		return off + upstreamT("Every model")
	case k.Reach == "some":
		return off + upstreamT("Some models")
	case len(k.Via) > 0:
		return off + upstreamT("Maybe some, through {list}", "list", strings.Join(k.Via, ", "))
	}
	return strings.TrimSuffix(off, " · ")
}

func (e *editor) providerKeysView(c *ui.Context, pr *provider, locked bool) {
	if !e.p.connectionSaved(pr) {
		return
	}
	s := &e.providerKeys
	kit.Group(c, nil, upstreamT("Gateway keys that may use this provider"), "", func() {
		if kit.GhostButton(c, upstreamT("Gateway keys that may use this provider"), icons.RefreshCw, s.api.busy).Disabled(locked || s.api.busy).Clicked() {
			s.requested, s.loaded = true, false
			s.api.err = ""
			e.p.gateway.ensureRuntime()
		}
		if s.requested && !s.loaded && !s.api.busy && s.api.err == "" {
			if e.p.gateway.runtimeURL != "" {
				s.api.request(e.p, "GET", "/api/provider-keys?provider="+url.QueryEscape(pr.ID), nil, s.accept)
			} else if e.p.gateway.runtimeError != "" {
				s.api.err = e.p.gateway.runtimeError
			} else {
				c.After(100 * time.Millisecond)
			}
		}
		s.api.notice(c, nil)
		if !s.loaded {
			return
		}
		if len(s.keys) == 0 {
			kit.Help(c, upstreamT("No key"))
		}
		for _, k := range s.keys {
			k := k
			ticked := providerKeyReaches(k)
			name := k.Name
			if name == "" {
				name = k.ID
			}
			ui.Column(c.Key("provider-key:" + k.ID)).FillWidth().Children(func() {
				next := ticked
				// A key held to this provider alone would be left no model,
				// which reads as every model: upstream changes it on the key.
				if kit.Toggle(c, &next, name, providerKeyNote(k)).Disabled(locked || s.api.busy || k.Only && ticked).Changed() && next != ticked {
					body := map[string]any{"provider": pr.ID, "add": []string{}, "remove": []string{}}
					if next {
						body["add"] = []string{k.ID}
					} else {
						body["remove"] = []string{k.ID}
					}
					s.api.request(e.p, "POST", "/api/provider-keys", body, s.accept)
				}
			})
		}
	})
}

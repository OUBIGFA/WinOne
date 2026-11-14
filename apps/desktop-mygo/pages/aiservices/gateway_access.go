package aiservices

import (
	"cmp"
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"slices"
	"strings"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type callerLimit struct {
	Period     string   `json:"period"`
	Days       int      `json:"days,omitempty"`
	Hours      int      `json:"hours,omitempty"`
	Tokens     int      `json:"tokens,omitempty"`
	Cost       float64  `json:"cost,omitempty"`
	CacheReads bool     `json:"cacheReads,omitempty"`
	Models     []string `json:"models,omitempty"`
}
type callerKey struct {
	MaxConcurrency int               `json:"maxConcurrency,omitempty"`
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Masked         string            `json:"masked"`
	Off            bool              `json:"off"`
	Browser        bool              `json:"browser"`
	ReadOnly       bool              `json:"readOnly"`
	Quotas         bool              `json:"quotas"`
	Issues         bool              `json:"issues"`
	Models         []string          `json:"models,omitempty"`
	Aliases        map[string]string `json:"aliases,omitempty"`
	ServiceTier    string            `json:"serviceTier,omitempty"`
	// Retain inactive full-version policy metadata on unrelated native edits.
	Admin    string        `json:"admin,omitempty"`
	Redact   string        `json:"redact,omitempty"`
	MCP      []string      `json:"mcp,omitempty"`
	Accounts []string      `json:"accounts,omitempty"`
	Expires  string        `json:"expires,omitempty"`
	Issuer   string        `json:"issuer,omitempty"`
	Limit    *callerLimit  `json:"limit,omitempty"`
	More     []callerLimit `json:"moreLimits,omitempty"`
	UsedAll  []struct {
		Tokens int64   `json:"tokens"`
		Cost   float64 `json:"cost"`
		Reset  string  `json:"reset"`
		Spent  bool    `json:"spent"`
	} `json:"usedAll"`
}
type callerKeys struct {
	Keys   []callerKey `json:"keys"`
	Secret string      `json:"secret"`
}
type accessChoice struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderName string `json:"providerName"`
}
type accessCatalog struct {
	Models   []accessChoice `json:"models"`
	Accounts []accessChoice `json:"accounts"`
}
type gatewayOIDCRule struct {
	ID       string            `json:"id"`
	Key      string            `json:"key"`
	Issuer   string            `json:"issuer"`
	Audience string            `json:"audience"`
	Claims   map[string]string `json:"claims"`
	TTL      int               `json:"ttl"`
}

// Gateway policies use the shared runtime's administrative API. The native
// form is a draft; one policy save is atomic, including all budget windows.
type gatewayAccessPanel struct {
	api                                  nativeAPI
	state                                *callerKeys
	source                               string
	edit, saved                          *callerKey
	fields                               fields
	tab                                  int
	creating, temporary, browser         bool
	name, secret, ttl, revealed, confirm string
	issueModels, issueAccounts           []string
	issueTokens                          int
	issueCost                            float64
	issueReadOnly                        bool
	identityOpen                         bool
	rules                                []gatewayOIDCRule
	rule                                 gatewayOIDCRule
	claimName                            string
	aliasName, aliasModel                string
	catalog                              *accessCatalog
}

func accessT(key string) string { return i18n.T("aiAccess." + key) }
func (a *gatewayAccessPanel) dirty() bool {
	return a.edit != nil && !equal(a.edit, a.saved) || a.creating && (a.name != "" || a.secret != "" || a.ttl != "" || len(a.issueModels)+len(a.issueAccounts) > 0 || a.issueTokens != 0 || a.issueCost != 0 || a.issueReadOnly || a.browser) || a.rule.Key != "" || a.rule.Issuer != "" || a.rule.Audience != "" || a.rule.TTL != 0 || len(a.rule.Claims) > 0 || a.claimName != "" || a.aliasName != "" || a.aliasModel != ""
}
func (a *gatewayAccessPanel) discard() {
	a.edit, a.saved = nil, nil
	a.creating = false
	a.name, a.secret = "", ""
	a.ttl, a.claimName = "", ""
	a.aliasName, a.aliasModel = "", ""
	a.issueModels, a.issueAccounts = nil, nil
	a.issueTokens, a.issueCost = 0, 0
	a.issueReadOnly, a.browser = false, false
	a.rule = gatewayOIDCRule{}
	a.fields = fields{}
}
func (a *gatewayAccessPanel) close() { a.api.close(); a.revealed = "" }
func (a *gatewayAccessPanel) load(p *page) {
	a.api.request(p, "GET", "/api/caller-keys", nil, func(raw json.RawMessage) {
		var state callerKeys
		if nativeDecode(&a.api, raw, &state) {
			a.state = &state
		}
	})
}
func (a *gatewayAccessPanel) accept(raw json.RawMessage) bool {
	var state callerKeys
	if !nativeDecode(&a.api, raw, &state) {
		return false
	}
	a.state = &state
	if state.Secret != "" {
		a.revealed = state.Secret
		a.state.Secret = ""
	}
	return true
}
func (a *gatewayAccessPanel) begin(p *page, k callerKey) {
	apply := func() {
		raw, _ := json.Marshal(k)
		a.edit, a.saved = &callerKey{}, &callerKey{}
		_ = json.Unmarshal(raw, a.edit)
		_ = json.Unmarshal(raw, a.saved)
		a.fields = fields{}
		a.tab = 0
		a.creating = false
		a.confirm = ""
		a.aliasName, a.aliasModel = "", ""
	}
	if a.dirty() && p.env.RequestChange != nil {
		p.env.RequestChange(apply)
	} else {
		apply()
	}
}
func (a *gatewayAccessPanel) action(p *page, action string, k callerKey, extra ...map[string]any) {
	body := map[string]any{"key": k.ID}
	for _, values := range extra {
		for name, value := range values {
			body[name] = value
		}
	}
	a.api.request(p, "POST", "/api/caller-keys/"+action, body, func(raw json.RawMessage) {
		if a.accept(raw) {
			a.confirm = ""
			a.secret = ""
			a.edit, a.saved = nil, nil
		}
	})
}
func (a *gatewayAccessPanel) View(c *ui.Context, p *page) {
	p.env.Activity.Report("ai-gateway-access", a.dirty(), a.api.busy, a.discard)
	if a.fields == nil {
		a.fields = fields{}
	}
	if p.gateway.runtimeURL == "" {
		return
	}
	if a.source != p.gateway.runtimeURL {
		a.api.close()
		a.api.err = ""
		a.source = p.gateway.runtimeURL
		a.state = nil
		a.catalog = nil
	}
	if a.state == nil && !a.api.busy && a.api.err == "" {
		a.load(p)
	}
	if a.state != nil && a.catalog == nil && !a.api.busy && a.api.err == "" {
		a.api.request(p, "GET", "/api/caller-keys/models", nil, func(raw json.RawMessage) {
			var catalog accessCatalog
			if nativeDecode(&a.api, raw, &catalog) {
				a.catalog = &catalog
			}
		})
	}

	a.api.notice(c, func() { a.load(p) })
	kit.Group(c, icons.ShieldCheck, accessT("title"), accessT("hint"), func() {
		ui.Row(c).FillWidth().Padding(12, 16).Gap(8).Wrap().Children(func() {
			if kit.Button(c, accessT("add"), icons.Plus, false).Disabled(a.api.busy || a.dirty()).Clicked() {
				a.creating = true
				a.temporary = false
				a.browser = false
				a.name, a.secret = "", ""
				a.edit = nil
			}
			if kit.Button(c, accessT("issue"), icons.Clock, false).Disabled(a.api.busy || a.dirty()).Clicked() {
				a.creating = true
				a.temporary = true
				a.ttl = "1h"
				a.name = ""
				a.issueModels, a.issueAccounts = nil, nil
				a.issueTokens, a.issueCost = 0, 0
				a.issueReadOnly = false
				a.edit = nil
			}
			if kit.IconButton(c, icons.RefreshCw, i18n.T("common.refresh")).Disabled(a.api.busy || a.dirty()).Clicked() {
				a.load(p)
			}
		})
		if a.state != nil {
			for index, k := range a.state.Keys {
				c := c.Key(k.ID)
				hint := k.Masked
				if k.Off {
					hint += " · " + accessT("disabled")
				}
				if k.Expires != "" {
					hint += " · " + accessT("expires") + " " + k.Expires
				}
				kit.Row(c, k.Name, hint, func() {
					if kit.IconButton(c, icons.ArrowUp, accessT("moveUp")).Disabled(a.api.busy || a.dirty() || index == 0).Clicked() {
						a.action(p, "move-key", k, map[string]any{"index": index - 1})
					}
					if kit.IconButton(c, icons.ArrowDown, accessT("moveDown")).Disabled(a.api.busy || a.dirty() || index+1 == len(a.state.Keys)).Clicked() {
						a.action(p, "move-key", k, map[string]any{"index": index + 1})
					}
					if kit.Button(c, i18n.T("common.edit"), icons.Pencil, false).Disabled(a.api.busy).Clicked() {
						a.begin(p, k)
					}
					if kit.IconButton(c, icons.Copy, accessT("copy")).Disabled(a.api.busy).Clicked() {
						services := c.Services()
						a.api.request(p, "POST", "/api/winone/gateway/copy", map[string]string{"id": k.ID}, func(raw json.RawMessage) {
							var out struct {
								Secret string `json:"secret"`
							}
							if nativeDecode(&a.api, raw, &out) {
								services.WriteClipboard(out.Secret)
								p.env.Toast(i18n.T("gateway.copied"))
							}
						})
					}
				})
			}
		}
		if a.revealed != "" {
			kit.Stack(c, accessT("newSecret"), accessT("newSecretHint"), func() {
				kit.TextInput(c, &a.revealed).FillWidth().ReadOnly(true)
				ui.Row(c).Gap(8).Children(func() {
					if kit.Button(c, accessT("copy"), icons.Copy, false).Clicked() {
						c.Services().WriteClipboard(a.revealed)
					}
					if kit.Button(c, i18n.T("gateway.hideKey"), icons.EyeOff, false).Clicked() {
						a.revealed = ""
					}
				})
			})
		}
		if a.creating {
			a.createView(c, p)
		}
		if a.edit != nil {
			a.editorView(c, p)
		}
	})
	a.oidcView(c, p)
}

func (a *gatewayAccessPanel) createView(c *ui.Context, p *page) {
	ui.Column(c).FillWidth().Padding(16).Gap(16).Children(func() {
		nativeText(c, accessT("name"), &a.name, a.api.busy)
		if a.temporary {
			nativeText(c, accessT("ttl"), &a.ttl, a.api.busy)
			kit.Help(c, accessT("ttlHint"))
			a.scopeList(c, "issueModels", accessT("models"), &a.issueModels, a.modelChoices(false), a.api.busy)
			a.scopeList(c, "issueAccounts", accessT("accounts"), &a.issueAccounts, a.modelChoices(true), a.api.busy)
			nativeInt(c, accessT("tokens"), &a.issueTokens, 0, 1<<53-1, a.api.busy)
			numberRow(c, accessT("cost"), &a.issueCost, 0, 1e15, false, "0", a.api.busy, func(v *float64) { a.issueCost = value(v) })
			kit.Toggle(c, &a.issueReadOnly, accessT("readOnly"), accessT("readOnlyHint")).Disabled(a.api.busy)
		} else {
			ui.Field(c, accessT("ownSecret"), func() { kit.TextInput(c, &a.secret).FillWidth().Password().Disabled(a.api.busy) })
			kit.Help(c, accessT("ownSecretHint"))
			kit.Toggle(c, &a.browser, accessT("browser"), accessT("browserHint")).Disabled(a.api.busy)
		}
		ui.Row(c).Gap(8).Children(func() {
			if kit.Button(c, i18n.T("common.save"), icons.Check, a.api.busy).Disabled(a.api.busy || a.name == "").Clicked() {
				if a.temporary {
					a.api.request(p, "POST", "/api/winone/gateway/issue", map[string]any{"name": a.name, "ttl": a.ttl, "models": a.issueModels, "accounts": a.issueAccounts, "tokens": a.issueTokens, "cost": a.issueCost, "readOnly": a.issueReadOnly}, func(raw json.RawMessage) {
						var out struct {
							Key string `json:"key"`
						}
						if nativeDecode(&a.api, raw, &out) {
							a.revealed = out.Key
							a.creating = false
							a.name = ""
							a.state = nil
						}
					})
				} else {
					a.api.request(p, "POST", "/api/caller-keys/add-key", map[string]any{"name": a.name, "secret": a.secret, "browser": a.browser}, func(raw json.RawMessage) {
						if a.accept(raw) {
							a.creating = false
							a.name, a.secret = "", ""
						}
					})
				}
			}
			if kit.Button(c, i18n.T("common.cancel"), nil, false).Disabled(a.api.busy).Clicked() {
				a.discard()
			}
		})
	})
}

func (a *gatewayAccessPanel) editorView(c *ui.Context, p *page) {
	k := a.edit
	locked := a.api.busy
	ui.Column(c).FillWidth().Padding(16).Gap(16).Children(func() {
		kit.Tabs(c, &a.tab, []kit.Tab{{Label: accessT("policy")}, {Label: accessT("scope")}, {Label: accessT("limits")}}, nil)
		switch a.tab {
		case 0:
			nativeText(c, accessT("name"), &k.Name, locked)
			nativeInt(c, upstreamT("Concurrency"), &k.MaxConcurrency, 0, 1000, locked)
			nativeSelect(c, nativeT("keyServiceTier"), &k.ServiceTier, options("", nativeT("followRequest"), "default", nativeT("standardTier"), "priority", "priority", "ultrafast", "ultrafast"), locked)
			kit.Help(c, upstreamT("How many requests this key may have in flight at once. Beyond it they wait in order; 0 for no limit."))
			kit.Toggle(c, &k.ReadOnly, accessT("readOnly"), accessT("readOnlyHint")).Disabled(locked || k.Browser)
			kit.Toggle(c, &k.Quotas, accessT("quotas"), accessT("quotasHint")).Disabled(locked)
			kit.Toggle(c, &k.Issues, accessT("issuer"), accessT("issuerHint")).Disabled(locked || k.Browser || k.Issuer != "" || k.Expires != "")
		case 1:
			kit.Help(c, accessT("scopeHint"))
			a.scopeList(c, "models", accessT("models"), &k.Models, a.modelChoices(false), locked)
			a.scopeList(c, "accounts", accessT("accounts"), &k.Accounts, a.modelChoices(true), locked)
			kit.Help(c, nativeT("keyAliasesHint"))
			for _, name := range sortedKeys(k.Aliases) {
				ui.Row(c.Key(name)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					model := k.Aliases[name]
					ui.Column(c).Grow(1).MinWidth(0).Children(func() { nativeText(c, name, &model, locked) })
					k.Aliases[name] = model
					if kit.IconButton(c, icons.Trash2, i18n.T("common.delete")+" "+name).Disabled(locked).Clicked() {
						delete(k.Aliases, name)
					}
				})
			}
			nativeText(c, nativeT("aliasName"), &a.aliasName, locked)
			nativeSelect(c, nativeT("aliasModel"), &a.aliasModel, a.modelChoices(false), locked)
			if kit.Button(c, nativeT("addAlias"), icons.Plus, false).Disabled(locked || strings.TrimSpace(a.aliasName) == "" || a.aliasModel == "").Clicked() {
				if k.Aliases == nil {
					k.Aliases = map[string]string{}
				}
				k.Aliases[strings.TrimSpace(a.aliasName)] = a.aliasModel
				a.aliasName, a.aliasModel = "", ""
			}
		case 2:
			limits := append([]callerLimit{}, k.More...)
			if k.Limit != nil {
				limits = append([]callerLimit{*k.Limit}, limits...)
			}
			remove := -1
			for n := range limits {
				w := &limits[n]
				cc := c.Key(n)
				ui.Column(cc).FillWidth().Padding(12, 0).Gap(12).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
					nativeSelect(cc, accessT("period"), &w.Period, options("day", accessT("day"), "week", accessT("week"), "month", accessT("month"), "days", accessT("days"), "hours", accessT("hours")), locked)
					if w.Period == "days" {
						nativeInt(cc, accessT("days"), &w.Days, 1, 36500, locked)
					}
					if w.Period == "hours" {
						nativeInt(cc, accessT("hours"), &w.Hours, 1, 876000, locked)
					}
					nativeInt(cc, accessT("tokens"), &w.Tokens, 0, 1<<53-1, locked)
					numberRow(cc, accessT("cost"), &w.Cost, 0, 1e15, false, "0", locked, func(v *float64) { w.Cost = value(v) })
					kit.Toggle(cc, &w.CacheReads, accessT("cacheReads"), "").Disabled(locked)
					a.scopeList(cc, fmt.Sprintf("limitModels%d", n), accessT("limitModels"), &w.Models, a.modelChoices(false), locked)
					if n < len(k.UsedAll) {
						u := k.UsedAll[n]
						kit.Help(cc, fmt.Sprintf("%s: %d tokens · $%.4f · %s %s", accessT("used"), u.Tokens, u.Cost, accessT("reset"), u.Reset))
						if kit.GhostButton(cc, accessT("resetOneLimit"), icons.RotateCcw, false).Disabled(locked || !equal(k, a.saved)).Clicked() {
							a.action(p, "reset-limit-key", *k, map[string]any{"index": n})
						}
					}
					if kit.GhostButton(cc, i18n.T("common.delete"), icons.Trash2, false).Disabled(locked).Clicked() {
						remove = n
					}
				})
			}
			if remove >= 0 {
				limits = slices.Delete(limits, remove, remove+1)
				a.fields = fields{}
			}
			if kit.Button(c, accessT("addLimit"), icons.Plus, false).Disabled(locked || len(limits) >= 5).Clicked() {
				limits = append(limits, callerLimit{Period: "day"})
			}
			k.Limit = nil
			k.More = nil
			if len(limits) > 0 {
				k.Limit = &limits[0]
				k.More = limits[1:]
			}
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if kit.Button(c, i18n.T("common.save"), icons.Check, locked).Disabled(locked || equal(k, a.saved)).Clicked() {
				a.api.request(p, "POST", "/api/winone/gateway/configure", k, func(raw json.RawMessage) {
					if a.accept(raw) {
						for _, row := range a.state.Keys {
							if row.ID == k.ID {
								a.edit, a.saved = nil, nil
								a.begin(p, row)
								break
							}
						}
						p.env.Toast(i18n.T("common.saved"))
					}
				})
			}
			if kit.Button(c, i18n.T("common.cancel"), nil, false).Disabled(locked).Clicked() {
				a.discard()
			}
			if kit.Button(c, accessT("resetLimits"), icons.RotateCcw, false).Disabled(locked || !equal(k, a.saved) || k.Limit == nil).Clicked() {
				a.action(p, "reset-limit-key", *k)
			}
			label, action := accessT("disable"), "off-key"
			if k.Off {
				label, action = accessT("enable"), "on-key"
			}
			if kit.Button(c, label, nil, false).Disabled(locked || !equal(k, a.saved)).Clicked() {
				a.action(p, action, *k)
			}
			for _, v := range []struct{ label, action string }{{accessT("rotate"), "rotate-key"}, {i18n.T("common.delete"), "remove-key"}} {
				if kit.GhostButton(c, v.label, nil, false).Disabled(locked || !equal(k, a.saved)).Clicked() {
					a.confirm = v.action
				}
			}
		})
		if a.confirm != "" {
			if a.confirm == "rotate-key" {
				ui.Field(c, accessT("ownSecret"), func() { kit.TextInput(c, &a.secret).FillWidth().Password().Disabled(locked) })
			}
			kit.Notice(c, kit.Caution, "", accessT("revokeHint"), func() {
				if kit.Button(c, accessT("confirm"), icons.Check, false).Disabled(locked).Clicked() {
					a.action(p, a.confirm, *k, map[string]any{"secret": a.secret})
				}
			})
		}
	})
}

// Offer runtime identities while retaining manual patterns and currently
// unavailable entries; opening a picker must never broaden an existing scope.
func (a *gatewayAccessPanel) modelChoices(accounts bool) []kit.SelectOption {
	if a.catalog == nil {
		return nil
	}
	rows := a.catalog.Models
	if accounts {
		rows = a.catalog.Accounts
	}
	out := make([]kit.SelectOption, 0, len(rows))
	for _, row := range rows {
		label := cmp.Or(row.Name, row.ID)
		if row.ProviderName != "" {
			label = row.ProviderName + " / " + label
		}
		out = append(out, kit.SelectOption{Value: row.ID, Label: label})
	}
	return out
}
func (a *gatewayAccessPanel) scopeList(c *ui.Context, key, label string, values *[]string, choices []kit.SelectOption, locked bool) {
	nativeList(c, a.fields, key, label, values, locked)
	if len(choices) == 0 {
		return
	}
	selected := ""
	if nativeSelect(c.Key(key+"-picker"), accessT("chooseScope"), &selected, append(options("", accessT("chooseScope")), choices...), locked) && selected != "" && !slices.Contains(*values, selected) {
		if selected == "-" {
			*values = []string{"-"}
		} else {
			*values = append(slices.DeleteFunc(*values, func(v string) bool { return v == "-" }), selected)
		}
		delete(a.fields, key)
	}
}

func (a *gatewayAccessPanel) oidcView(c *ui.Context, p *page) {
	ui.Collapsible(c, accessT("oidc"), &a.identityOpen, func() {
		kit.Help(c, accessT("oidcHint"))
		if a.rules == nil && !a.api.busy && a.api.err == "" {
			a.api.request(p, "GET", "/api/winone/gateway/oidc", nil, func(raw json.RawMessage) {
				var out struct {
					Rules []gatewayOIDCRule `json:"rules"`
				}
				if nativeDecode(&a.api, raw, &out) {
					a.rules = append([]gatewayOIDCRule{}, out.Rules...)
				}
			})
		}
		for _, r := range a.rules {
			kit.Row(c.Key(r.ID), r.Issuer, r.Audience, func() {
				if kit.IconButton(c, icons.Trash2, i18n.T("common.delete")).Disabled(a.api.busy).Clicked() {
					a.api.request(p, "POST", "/api/winone/gateway/oidc/remove", map[string]string{"id": r.ID}, func(json.RawMessage) { a.rules = nil })
				}
			})
		}
		choices := options("", accessT("chooseIssuer"))
		if a.state != nil {
			for _, k := range a.state.Keys {
				if k.Issues && !k.Off {
					choices = append(choices, kit.SelectOption{Value: k.ID, Label: k.Name})
				}
			}
		}
		nativeSelect(c, accessT("issuerKey"), &a.rule.Key, choices, a.api.busy)
		nativeText(c, accessT("identityURL"), &a.rule.Issuer, a.api.busy)
		nativeText(c, accessT("audience"), &a.rule.Audience, a.api.busy)
		nativeInt(c, accessT("ttlSeconds"), &a.rule.TTL, 0, 2592000, a.api.busy)
		for _, name := range sortedKeys(a.rule.Claims) {
			ui.Row(c.Key(name)).FillWidth().Gap(8).Children(func() {
				v := a.rule.Claims[name]
				ui.Column(c).Grow(1).Children(func() { nativeText(c, name, &v, a.api.busy) })
				a.rule.Claims[name] = v
				if kit.IconButton(c, icons.Trash2, i18n.T("common.delete")+" "+name).Disabled(a.api.busy).Clicked() {
					delete(a.rule.Claims, name)
				}
			})
		}
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			kit.TextInput(c, &a.claimName).Grow(1).Label(accessT("claim"))
			if kit.Button(c, accessT("addClaim"), icons.Plus, false).Disabled(a.api.busy || a.claimName == "").Clicked() {
				if a.rule.Claims == nil {
					a.rule.Claims = map[string]string{}
				}
				a.rule.Claims[a.claimName] = ""
				a.claimName = ""
			}
		})
		if kit.Button(c, accessT("addRule"), icons.Plus, false).Disabled(a.api.busy || a.rule.Key == "" || len(a.rule.Claims) == 0).Clicked() {
			a.api.request(p, "POST", "/api/winone/gateway/oidc", a.rule, func(json.RawMessage) { a.rule = gatewayOIDCRule{}; a.rules = nil })
		}
	})
}

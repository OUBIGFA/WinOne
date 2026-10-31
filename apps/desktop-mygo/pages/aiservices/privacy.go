package aiservices

import (
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/web"
)

// The kinds personal masking finds; the first six are on by default.
var redactKinds = []string{"ssn", "passport", "iban", "birthday", "mac", "serial", "home", "userhost", "ip", "bucket"}

// The kinds an action can be set for: secrets, personal information, the
// custom rules' and custom words'.
var (
	secretKinds   = []string{"API_KEY", "PRIVATE_KEY", "TOKEN", "PASSWORD", "SECRET"}
	personalKinds = []string{"EMAIL", "PHONE", "ID_CARD", "BANK_CARD", "PERSON", "ADDRESS", "ACCOUNT", "SSN", "PASSPORT", "IBAN",
		"BIRTHDAY", "MAC", "SERIAL", "USERNAME", "HOSTNAME", "IP", "BUCKET"}
)

// listField is a comma separated list typed into one field: it applies
// when the field is left, so a separator just typed is kept.
type listField struct {
	text, of string
	focused  bool
}

func (a *advancedPanel) listInput(c *ui.Context, key, label string, values []string, locked bool, set func([]string)) {
	f := a.lists(key)
	joined := strings.Join(values, ", ")
	if !f.focused && joined != f.of {
		f.text, f.of = joined, joined
	}
	input := ui.TextInput(c, &f.text).FillWidth().Label(label).Disabled(locked)
	focused := input.Focused()
	if f.focused && !focused {
		next := splitList(f.text)
		f.of = strings.Join(next, ", ")
		f.text = f.of
		set(next)
	}
	f.focused = focused
}

func (a *advancedPanel) lists(key string) *listField {
	if a.listFields == nil {
		a.listFields = map[string]*listField{}
	}
	if a.listFields[key] == nil {
		a.listFields[key] = &listField{}
	}
	return a.listFields[key]
}

// ruleForm is the custom masking rule being added or edited.
type ruleForm struct {
	editing     int // -1 adds a rule
	kind, by    string
	match       string
	initialized bool
}

// privacy is masking, the local privacy model, custom rules and actions,
// statistics, the request archive, private downloads and account reads,
// below Magpie's own privacy records.
func (a *advancedPanel) privacy(c *ui.Context, catalog *magpieCatalog, locked bool) {
	p := a.p
	s := a.settings()
	a.recordsView(c, locked)
	form := kit.Group(c, icons.ShieldCheck, i18n.T("ai.privacySettings"), "", func() {
		kit.Toggle(c, &s.Redact, i18n.T("ai.redact"), i18n.T("ai.redactHint")).Disabled(locked)
		kit.Toggle(c, &s.RedactShape, i18n.T("ai.redactShape"), i18n.T("ai.redactShapeHint")).Disabled(locked)
		personal := s.RedactPersonal
		if kit.Toggle(c, &personal, i18n.T("ai.redactPersonal"), i18n.T("ai.redactPersonalHint")).Disabled(locked).Changed() {
			s.RedactPersonal = personal
			if personal {
				// Masking people hides the accounts shown too.
				s.HideAccounts = true
			}
		}
		if s.RedactPersonal {
			kit.Indented(c, func() {
				for i, kind := range redactKinds {
					kind := kind
					on, set := s.RedactKinds[kind]
					if !set {
						on = i < 6
					}
					if kit.Toggle(c.Key(kind), &on, i18n.T("ai.redactKind"+kind), "").Disabled(locked).Changed() {
						next := map[string]bool{}
						for k, v := range s.RedactKinds {
							next[k] = v
						}
						next[kind] = on
						s.RedactKinds = next
					}
				}
			})
		}
		if a.model == nil {
			a.model = newPrivacyModel(p)
		}
		a.model.View(c, locked)
		kit.Toggle(c, &s.HideAccounts, i18n.T("ai.hideAccounts"), i18n.T("ai.hideAccountsHint")).Disabled(locked)
		kit.Stack(c, i18n.T("ai.redactWords"), i18n.T("ai.redactWordsHint"), func() {
			a.listInput(c, "words", i18n.T("ai.redactWords"), s.RedactWords, locked, func(v []string) { s.RedactWords = v })
		})
		kit.Toggle(c, &s.RedactWordsAnyCase, i18n.T("ai.redactWordsAnyCase"), "").Disabled(locked)
		a.rules(c, locked)
		a.actions(c, locked)
		stats := !s.NoStats
		if kit.Toggle(c, &stats, i18n.T("ai.sendStats"), i18n.T("ai.sendStatsHint")).Disabled(locked).Changed() {
			s.NoStats = !stats
		}
		if !s.NoStats {
			usage := !s.NoUsageStats
			if kit.Toggle(c, &usage, i18n.T("ai.sendUsageStats"), i18n.T("ai.sendUsageStatsHint")).Disabled(locked).Changed() {
				s.NoUsageStats = !usage
			}
		}
	})
	if a.focusPrivacy {
		a.focusPrivacy = false
		form.ScrollIntoView()
	}
	kit.Group(c, nil, i18n.T("ai.requestArchive"), "", func() {
		kit.Toggle(c, &s.RequestArchiveLocal, i18n.T("ai.requestArchiveLocal"), i18n.T("ai.requestArchiveHint")).Disabled(locked)
		local, most := value(s.RequestArchiveLocalMB), value(s.RequestArchiveMaxMB)
		numberRow(c, i18n.T("ai.requestArchiveLocalMB"), &local, 0, 1048576, true, "", locked, func(v *float64) { s.RequestArchiveLocalMB = ptr(value(v)) })
		numberRow(c, i18n.T("ai.requestArchiveMaxMB"), &most, 0, 1024, true, "", locked, func(v *float64) { s.RequestArchiveMaxMB = ptr(value(v)) })
		kit.Stack(c, "", i18n.T("ai.requestArchiveLimitsHint"), func() {})
	})
	kit.Group(c, nil, i18n.T("ai.fetchPrivate"), "", func() {
		kit.Stack(c, i18n.T("ai.fetchPrivate"), i18n.T("ai.fetchPrivateHint"), func() {
			a.listInput(c, "fetchPrivate", i18n.T("ai.fetchPrivate"), s.FetchPrivate, locked, func(v []string) { s.FetchPrivate = v })
		})
	})
	kit.Group(c, nil, i18n.T("ai.accountDiscovery"), "", func() {
		discovery := !s.NoAccountDiscovery && !catalog.NoAccountsEnv
		hint := "ai.accountDiscoveryHint"
		if catalog.NoAccountsEnv {
			hint = "ai.accountDiscoveryLocked"
		}
		if kit.Toggle(c, &discovery, i18n.T("ai.subscriptionAccounts"), i18n.T(hint)).Disabled(locked || catalog.NoAccountsEnv).Changed() {
			s.NoAccountDiscovery = !discovery
		}
	})
}

// rules are the custom masking rules: a kind, matched by prefix or regex.
func (a *advancedPanel) rules(c *ui.Context, locked bool) {
	s := a.settings()
	f := &a.rule
	if !f.initialized {
		f.editing, f.by, f.initialized = -1, "prefix", true
	}
	reset := func() { f.editing, f.kind, f.by, f.match = -1, "", "prefix", "" }
	kit.Stack(c, i18n.T("ai.redactRules"), "", func() {
		ui.Field(c, i18n.T("ai.redactRuleKind"), func() {
			ui.TextInput(c, &f.kind).FillWidth().Placeholder("API_KEY").Label(i18n.T("ai.redactRuleKind")).Disabled(locked)
		})
		ui.Field(c, i18n.T("ai.redactRuleMatch"), func() {
			ui.Column(c).Disabled(locked).Children(func() {
				kit.Segmented(c, &f.by, options("prefix", i18n.T("ai.prefix"), "regex", i18n.T("ai.regex")), i18n.T("ai.redactRuleMatch"))
			})
		})
		ui.Field(c, i18n.T("ai.redactRuleContent"), func() {
			ui.TextInput(c, &f.match).FillWidth().Label(i18n.T("ai.redactRuleContent")).Disabled(locked)
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			label, icon := "ai.addRedactRule", icons.Plus
			if f.editing >= 0 {
				label, icon = "ai.applyRedactRule", icons.Pencil
			}
			if kit.Button(c, i18n.T(label), icon, false).Disabled(locked || strings.TrimSpace(f.match) == "").Clicked() {
				kind := strings.TrimSpace(f.kind)
				if kind == "" {
					kind = "SECRET"
				}
				rule := core.Preserved[redactRule]{Value: redactRule{Kind: kind}}
				if f.by == "regex" {
					rule.Value.Regex = strings.TrimSpace(f.match)
				} else {
					rule.Value.Prefix = strings.TrimSpace(f.match)
				}
				next := append([]core.Preserved[redactRule]{}, s.RedactRules...)
				if f.editing >= 0 && f.editing < len(next) {
					next[f.editing] = rule
				} else {
					next = append(next, rule)
				}
				s.RedactRules = next
				reset()
			}
			if f.editing >= 0 && kit.Button(c, i18n.T("common.cancel"), nil, false).Disabled(locked).Clicked() {
				reset()
			}
		})
		for i, d := range s.RedactRules {
			i, rule := i, d.Value
			ui.Row(c.Key(i)).FillWidth().Gap(8).AlignItems(ui.Center).Padding(8, 0, 0, 0).BorderWidth(1, 0, 0, 0).
				BorderColor(kit.Colors(c).Hairline).Children(func() {
				by, match := "ai.prefix", rule.Prefix
				if rule.Prefix == "" {
					by, match = "ai.regex", rule.Regex
				}
				ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Wrap().AlignItems(ui.Center).Children(func() {
					ui.Text(c, rule.Kind+" · "+i18n.T(by)+":").FontSize(kit.HelpSize(c))
					kit.Mono(c, match).Selectable().MinWidth(0)
				})
				if kit.IconButton(c, icons.Pencil, i18n.T("ai.editRedactRule", "index", i+1)).Disabled(locked).Clicked() {
					f.editing, f.kind, f.by, f.match = i, rule.Kind, strings.TrimPrefix(by, "ai."), match
				}
				if kit.IconButton(c, icons.Trash2, i18n.T("ai.removeRedactRule", "index", i+1)).Disabled(locked).Clicked() {
					s.RedactRules = slices.Delete(append([]core.Preserved[redactRule]{}, s.RedactRules...), i, i+1)
					switch {
					case f.editing == i:
						reset()
					case f.editing > i:
						f.editing--
					}
				}
			})
		}
	})
}

// actions decide, per kind of sensitive information, whether it is
// masked, sent with a warning or blocks the request; and which values are
// always sent as they are.
func (a *advancedPanel) actions(c *ui.Context, locked bool) {
	s := a.settings()
	var kinds []string
	if s.Redact {
		kinds = append(kinds, secretKinds...)
		for _, r := range s.RedactRules {
			kinds = append(kinds, ruleKind(r.Value.Kind))
		}
	}
	if len(s.RedactWords) > 0 {
		kinds = append(kinds, "TERM")
	}
	kinds = append(kinds, sortedKeys(s.RedactActions)...)
	if s.RedactPersonal {
		kinds = append(kinds, personalKinds...)
	}
	kinds = unique(kinds)
	if _, ok := a.opens["actions"]; !ok {
		a.opens["actions"] = len(s.RedactActions) > 0 || len(s.RedactAllow) > 0
	}
	open := a.opens["actions"]
	kit.Stack(c, "", "", func() {
		ui.Collapsible(c, i18n.T("ai.redactActions"), &open, func() {
			ui.Column(c).FillWidth().Gap(12).Padding(8, 0).Children(func() {
				kit.Help(c, i18n.T("ai.redactActionsHint"))
				for _, kind := range kinds {
					kind := kind
					name := kind
					if key := "ai.redactType" + kind; i18n.Has(key) {
						name = i18n.T(key)
					}
					action := s.RedactActions[kind]
					if action == "" {
						action = "mask"
					}
					ui.Field(c.Key(kind), name+" · "+kind, func() {
						ui.Column(c).FillWidth().Gap(4).Disabled(locked).Children(func() {
							if kit.Segmented(c, &action, options("mask", i18n.T("ai.redactActionmask"), "warn", i18n.T("ai.redactActionwarn"),
								"block", i18n.T("ai.redactActionblock")), name) {
								next := map[string]string{}
								for k, v := range s.RedactActions {
									next[k] = v
								}
								if action == "mask" {
									delete(next, kind)
								} else {
									next[kind] = action
								}
								s.RedactActions = next
							}
							kit.Help(c, i18n.T("ai.redactActionHint"+action))
						})
					})
				}
				ui.Field(c, i18n.T("ai.redactAllow"), func() {
					a.listInput(c, "allow", i18n.T("ai.redactAllow"), s.RedactAllow, locked, func(v []string) { s.RedactAllow = v })
				})
				kit.Help(c, i18n.T("ai.redactAllowHint"))
			})
		}).FontSize(kit.HelpSize(c))
	})
	a.opens["actions"] = open
}

// recordsView is Magpie's privacy records page, served beside the plugin
// pages: its "turn on" and "see the rules" open the form below, which the
// page saves, so unsaved-change protection applies.
func (a *advancedPanel) recordsView(c *ui.Context, locked bool) {
	p := a.p
	x := p.extensions.state
	if x == nil {
		return
	}
	base, err := url.Parse(x.URL)
	if err != nil {
		return
	}
	page := base.ResolveReference(&url.URL{Path: "masking.html"})
	if p.maskRecord != "" {
		q := page.Query()
		q.Set("record", p.maskRecord)
		page.RawQuery = q.Encode()
	}
	address := page.String()
	if a.records != nil && a.recordsFor != address {
		a.records.Close()
		a.records = nil
	}
	if a.records == nil {
		title := "Masking records"
		if strings.HasPrefix(i18n.Locale(), "zh") {
			title = "脱敏记录"
		}
		a.recordsFor = address
		a.records = web.New(p.env, web.Options{URL: address, Label: title, Theme: "winone-plugin-theme", Height: 320,
			Accept:     []string{"winone-plugin-height", "winone-masking-actions", "winone-masking-settings"},
			HeightType: "winone-plugin-height", Receive: a.receiveRecords})
	}
	a.records.View(c, locked)
}

func (a *advancedPanel) receiveRecords(m web.Message) {
	switch m.String("type") {
	case "winone-masking-actions":
		kinds, _ := m["kinds"].([]any)
		if a.p.locked() {
			return
		}
		var names []string
		for _, k := range kinds {
			name, ok := k.(string)
			if !ok || !maskKinds.MatchString(name) {
				return
			}
			names = append(names, name)
		}
		// Turning masking on for these kinds removes their warn or block
		// action, staged in the draft.
		s := a.settings()
		next := map[string]string{}
		for k, v := range s.RedactActions {
			if !slices.Contains(names, k) {
				next[k] = v
			}
		}
		s.RedactActions = next
		a.focusPrivacy = true
	case "winone-masking-settings":
		a.focusPrivacy = true
	}
}

// privacyStatus is the local privacy model's state.
type privacyStatus struct {
	On          *bool   `json:"on"`
	Supported   *bool   `json:"supported"`
	Installed   bool    `json:"installed"`
	Size        float64 `json:"size"`
	Downloading bool    `json:"downloading,omitempty"`
	Done        float64 `json:"done,omitempty"`
	Total       float64 `json:"total,omitempty"`
	Error       string  `json:"error,omitempty"`
	Running     bool    `json:"running,omitempty"`
	Device      string  `json:"device,omitempty"`
	GPU         bool    `json:"gpu,omitempty"`
	LoadMS      float64 `json:"loadMs,omitempty"`
	GPUError    string  `json:"gpuError,omitempty"`
	Failed      string  `json:"failed,omitempty"`
}

// privacyModel is Magpie's local model for masking, with its own
// lifecycle: switching it, downloading and removing apply at once and
// save no draft. The page holds a viewer lease while it shows, and
// nothing downloads merely because the page opened.
type privacyModel struct {
	p        *page
	viewer   string
	state    *privacyStatus
	err      string
	busy     bool
	confirm  bool
	sequence int
	starts   int
	polling  bool
	closed   bool
}

func newPrivacyModel(p *page) *privacyModel {
	m := &privacyModel{p: p, viewer: nonce()}
	m.load("open", "")
	return m
}

// load asks the Host for the state, or acts; a newer request wins.
func (m *privacyModel) load(method, action string) {
	p := m.p
	m.sequence++
	ticket := m.sequence
	if action != "" {
		m.busy, m.starts = true, 0
	}
	if m.state == nil {
		m.busy = true
	}
	params := map[string]string{"viewer": m.viewer}
	if action != "" {
		params["action"] = action
	}
	core.Go(p.env, func() (privacyStatus, error) {
		var status privacyStatus
		err := p.env.Host.CallCompletingInto("capture.ai.magpie.privacy."+method, params, &status)
		if err == nil && (status.On == nil || status.Supported == nil) {
			err = errors.New(i18n.T("ai.privacyInvalidStatus"))
		}
		return status, err
	}, func(status privacyStatus, err error) {
		if m.closed || ticket != m.sequence {
			return
		}
		m.busy = false
		if err != nil {
			m.err = err.Error()
			return
		}
		m.state, m.err = &status, ""
		// A download, or a model starting, is followed each second.
		if status.Downloading || (*status.On && status.Installed && !status.Running && status.Failed == "" && m.starts < 30) {
			m.starts++
			m.follow(ticket)
		}
	})
}

func (m *privacyModel) follow(ticket int) {
	core.Do(m.p.env, func() error {
		time.Sleep(time.Second)
		return nil
	}, func(error) {
		if !m.closed && ticket == m.sequence {
			m.load("state", "")
		}
	})
}

// Close releases the lease, after any request still on its way.
func (m *privacyModel) Close() {
	if m.closed {
		return
	}
	m.closed = true
	m.sequence++
	p, viewer := m.p, m.viewer
	spawn(p.env, func() {
		_ = p.env.Host.CallCompletingInto("capture.ai.magpie.privacy.close", map[string]string{"viewer": viewer}, nil)
	})
}

func megabytes(v float64) string { return strconv.FormatFloat(v/1e6, 'f', 0, 64) }

func (m *privacyModel) View(c *ui.Context, disabled bool) {
	s := m.state
	size := 1637572192.0
	if s != nil && s.Size > 0 {
		size = s.Size
	}
	sizeText := strconv.FormatFloat(size/1e9, 'f', 1, 64) + " GB"
	description := i18n.T("ai.privacyModelHint", "size", sizeText)
	if s != nil {
		supported, on := *s.Supported, *s.On
		switch {
		case !supported && !s.Installed:
			description = i18n.T("ai.privacyUnsupported")
		case s.Downloading:
			total := s.Total
			if total == 0 {
				total = s.Size
			}
			description = i18n.T("ai.privacyDownloading", "done", megabytes(s.Done), "total", megabytes(total))
		case on && s.Running && s.GPU:
			device := s.Device
			if device == "" {
				device = "GPU"
			}
			description = i18n.T("ai.privacyGPU", "device", device)
		case on && s.Running:
			description = i18n.T("ai.privacyCPU", "device", s.Device)
		case on:
			description = i18n.T("ai.privacyReady")
		case s.Installed:
			description = i18n.T("ai.privacyInstalled", "size", sizeText)
		}
	}
	on := s != nil && (*s.On || s.Downloading)
	unavailable := s == nil || (!*s.Supported && !s.Installed)
	if kit.Toggle(c, &on, i18n.T("ai.privacyModel"), description).Disabled(disabled || m.busy || unavailable).Changed() {
		action := "off"
		if on {
			action = "on"
		}
		m.load("action", action)
	}
	kit.Stack(c, "", i18n.T("ai.privacyModelScope"), func() {
		if m.busy && s == nil {
			kit.Status(c, i18n.T("capture.loading"))
		}
		if s != nil && s.Downloading {
			total := s.Total
			if total == 0 {
				total = s.Size
			}
			if total > 0 {
				ui.Progress(c, float64(min(s.Done, total)/total)).FillWidth().Label(i18n.T("ai.privacyDownloadProgress"))
			}
		}
		if s != nil && s.LoadMS > 0 && s.Running {
			kit.Help(c, i18n.T("ai.privacyLoadTime", "ms", fmtNumber(s.LoadMS)))
		}
		if s != nil && s.GPUError != "" {
			kit.Notice(c, kit.Info, "", i18n.T("ai.privacyGPUFallback", "error", s.GPUError), nil)
		}
		failed := s != nil && *s.On && s.Failed != ""
		if m.err != "" || (s != nil && s.Error != "") || failed {
			message := m.err
			if message == "" {
				if s.Failed != "" {
					message = i18n.T("ai.privacyFailed", "error", s.Failed)
				} else {
					message = i18n.T("ai.privacyDownloadFailed", "error", s.Error)
				}
			}
			kit.Notice(c, kit.Failure, "", message, func() {
				if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Disabled(disabled || m.busy).Clicked() {
					if s != nil && s.Error != "" {
						m.load("action", "on")
					} else {
						m.load("open", "")
					}
				}
			})
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if s == nil {
				return
			}
			if s.Downloading && kit.Button(c, i18n.T("ai.privacyCancel"), icons.X, false).Disabled(disabled || m.busy).Clicked() {
				m.load("action", "cancel")
			}
			if s.Installed && !*s.On && !s.Downloading && kit.Button(c, i18n.T("ai.privacyDelete"), icons.Trash2, false).Disabled(disabled || m.busy).Clicked() {
				m.confirm = true
			}
			if !s.Installed && !s.Downloading && *s.Supported && kit.Button(c, i18n.T("ai.privacyDownloadEnable"), icons.Download, false).
				Disabled(disabled || m.busy).Clicked() {
				m.load("action", "on")
			}
		})
	})
	if kit.Dialog(c, &m.confirm, kit.DialogOptions{Title: i18n.T("ai.privacyDeleteQuestion"), Message: i18n.T("ai.privacyDeleteHint", "size", sizeText),
		Confirm: i18n.T("ai.privacyDelete"), Cancel: i18n.T("common.cancel"), Danger: true, ConfirmDisabled: disabled || m.busy}) {
		m.confirm = false
		m.load("action", "remove")
	}
}

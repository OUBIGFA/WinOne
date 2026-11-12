package aiservices

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Keep unfinished structured options with the provider's draft, including
// across provider and tab changes. Invalid JSON blocks Save, never discards
// the last valid value or silently saves it in place of the user's text.
type preferenceField struct {
	text, shown string
	invalid     bool
}

func (p *page) preferencesInvalid() bool {
	for _, f := range p.preferenceFields {
		if f.invalid {
			return true
		}
	}
	return false
}

func preferenceJSON[T any](c *ui.Context, p *page, key, label, hint string, value *T, locked bool) {
	if p.preferenceFields == nil {
		p.preferenceFields = map[string]*preferenceField{}
	}
	f := p.preferenceFields[key]
	if f == nil {
		b, _ := json.MarshalIndent(value, "", "  ")
		f = &preferenceField{text: string(b), shown: string(b)}
		p.preferenceFields[key] = f
	}
	if f.text != f.shown {
		f.shown = f.text
		var next T
		err := json.Unmarshal([]byte(f.text), &next)
		f.invalid = err != nil
		if err == nil {
			*value = next
		}
	}
	kit.Stack(c.Key(key), label, hint, func() {
		ui.TextArea(c, &f.text).FillWidth().Lines(2, 8).Font(kit.MonoFont).Label(label).Disabled(locked)
		if f.invalid {
			kit.Notice(c, kit.Caution, "", i18n.T("ai.optionsJSONInvalid"), nil)
		}
	})
}

// These are views of one provider draft. Every option keeps its original
// Magpie field; ordinary structured values use controls rather than JSON.
func (e *editor) preferences(c *ui.Context, pr *provider, locked bool) {
	kit.SubTabs(c, &e.preferenceTab, []string{i18n.T("aiOptions.requests"), i18n.T("aiOptions.accounts"), i18n.T("aiOptions.billing"), i18n.T("aiOptions.compatibility"), i18n.T("aiComplete.discovery"), i18n.T("aiComplete.service")}, e.p.env.RequestChange)
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Children(func() {
		switch e.preferenceTab {
		case 0:
			e.requestPreferences(c, pr, locked)
		case 1:
			e.accountPreferences(c, pr, locked)
		case 2:
			e.billingPreferences(c, pr, locked)
		case 3:
			e.compatibilityPreferences(c, pr, locked)
		case 4:
			e.modelDiscoveryPreferences(c, pr, locked)
		case 5:
			e.servicePreferences(c, pr, locked)
		}
	})
}

func (e *editor) requestPreferences(c *ui.Context, pr *provider, locked bool) {
	s := &pr.ProviderPreferences
	kit.Group(c, nil, i18n.T("aiOptions.transport"), "", func() {
		kit.Stack(c, "", "", func() { e.proxyPreference(c, "provider", &s.Proxy, locked) })
		choiceRow(c, i18n.T("ai.authHeader"), "", &s.AuthHeader, options("", i18n.T("ai.automatic"), "x-api-key", "x-api-key", "bearer", "Authorization: Bearer"), locked)
		kit.Toggle(c, &s.InsecureTLS, i18n.T("ai.insecureTLS"), i18n.T("ai.insecureTLSHint")).Disabled(locked)
		wait := float64(s.Patience)
		numberRow(c, i18n.T("ai.providerPatience"), &wait, -1, 240, true, "", locked, func(v *float64) { s.Patience = int(value(v)) })
	})
	kit.Group(c, nil, i18n.T("aiOptions.response"), "", func() {
		kit.Stack(c, i18n.T("ai.passHeaders"), i18n.T("ai.passHeadersHint"), func() { e.preferenceList(c, "passHeaders", i18n.T("ai.passHeaders"), &s.PassHeaders, locked) })
		kit.Stack(c, i18n.T("ai.replyHeaders"), i18n.T("ai.passHeadersHint"), func() { e.preferenceList(c, "replyHeaders", i18n.T("ai.replyHeaders"), &s.ReplyHeaders, locked) })
	})
	kit.Group(c, nil, i18n.T("aiComplete.fallback"), i18n.T("aiComplete.fallbackHint"), func() {
		kit.Stack(c, "", "", func() { e.preferenceList(c, "fallback", i18n.T("aiComplete.fallback"), &s.Fallback, locked) })
	})
}

func (e *editor) preferenceList(c *ui.Context, key, label string, values *[]string, locked bool) {
	f := e.fields.get("preferences:" + key)
	if text, changed := f.sync(strings.Join(*values, "\n")); changed {
		var next []string
		for _, line := range strings.Split(text, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				next = append(next, line)
			}
		}
		*values = unique(next)
	}
	ui.TextArea(c, &f.text).FillWidth().Lines(2, 5).Label(label).Disabled(locked)
}

func (e *editor) proxyPreference(c *ui.Context, key string, value *string, locked bool) {
	mode := ""
	if *value == "direct" {
		mode = "direct"
	} else if *value != "" {
		mode = "custom"
	}
	// Custom with an empty address is an editing state. Keep it until the
	// stored value changes externally (Save/Undo); otherwise Direct would
	// replace the user's new selection on the next frame.
	type proxyDraft struct{ mode, shown string }
	state := ui.Local(ui.Column(c.Key(key)).FillWidth(), "proxy-mode", func() proxyDraft { return proxyDraft{mode, *value} })
	if state.shown != *value {
		state.mode = mode
	}
	label := i18n.T("ai.providerProxy")
	if key != "provider" {
		label = i18n.T("ai.accountProxies")
	}
	if nativeSelect(c, label, &state.mode, options("", i18n.T("aiOptions.default"), "direct", i18n.T("aiOptions.direct"), "custom", i18n.T("aiOptions.customProxy")), locked) {
		if state.mode != "custom" {
			*value = state.mode
		} else if *value == "direct" {
			*value = ""
		}
	}
	if state.mode == "custom" {
		f := e.fields.get("proxy:" + key)
		current := *value
		if current == "direct" {
			current = ""
		}
		if text, changed := f.sync(current); changed {
			*value = text
		}
		kit.TextInput(c, &f.text).FillWidth().Placeholder("http://127.0.0.1:7890").Label(i18n.T("aiOptions.proxyAddress")).Disabled(locked)
	}
	state.shown = *value
}

func (e *editor) compatibilityPreferences(c *ui.Context, pr *provider, locked bool) {
	s := &pr.ProviderPreferences
	kit.Panel(c, func() {
		kit.Toggle(c, &s.AdditionalToolsOff, i18n.T("ai.additionalToolsOff"), "").Disabled(locked)
		kit.Toggle(c, &s.NativeCompaction, i18n.T("aiComplete.nativeCompaction"), i18n.T("aiComplete.nativeCompactionHint")).Disabled(locked)
		kit.Toggle(c, &s.PinUpstream, i18n.T("aiComplete.pinUpstream"), i18n.T("aiComplete.pinUpstreamHint")).Disabled(locked)
		if pr.Agent == "antigravity" {
			kit.Toggle(c, &s.PromptAsWritten, i18n.T("ai.promptAsWritten"), "").Disabled(locked)
		}
		kit.Toggle(c, &s.CodexAsSent, i18n.T("ai.codexAsSent"), "").Disabled(locked)
		if pr.AuthMode == "api-key" {
			kit.Toggle(c, &s.HTTP1, nativeT("http1"), nativeT("http1Hint")).Disabled(locked)
			kit.Toggle(c, &s.ImageFilesOff, nativeT("inlineImages"), nativeT("inlineImagesHint")).Disabled(locked)
			kit.Toggle(c, &s.PassErrors, nativeT("passErrors"), nativeT("passErrorsHint")).Disabled(locked)
			ownAgent := !s.MagpieUserAgent
			if kit.Toggle(c, &ownAgent, nativeT("passUserAgent"), nativeT("passUserAgentHint")).Disabled(locked).Changed() {
				s.MagpieUserAgent = !ownAgent
			}
			kit.Toggle(c, &s.KeepSelf, nativeT("keepSelf"), nativeT("keepSelfHint")).Disabled(locked)
		}
		kit.Toggle(c, &s.UseFirst, nativeT("useFirst"), nativeT("useFirstHint")).Disabled(locked)
	})
	kit.Section(c, nil, i18n.T("ai.extraBody"), i18n.T("ai.extraBodyHint"), func() { preferenceJSON(c, e.p, pr.ID+":extraBody", "", "", &s.ExtraBody, locked) })
}

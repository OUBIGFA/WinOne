package aiservices

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type maskingValue struct {
	Placeholder string `json:"p"`
	Kind        string `json:"k"`
	By          string `json:"b"`
	Head        string `json:"h"`
	Length      int    `json:"l"`
	Count       int    `json:"n"`
	Action      string `json:"a"`
	Known       bool   `json:"known"`
}
type maskingRecord struct {
	Key      string            `json:"key"`
	Time     string            `json:"t"`
	Agent    string            `json:"agent"`
	Provider string            `json:"provider"`
	Model    string            `json:"model"`
	Count    int               `json:"n"`
	Blocked  bool              `json:"blocked"`
	Values   []maskingValue    `json:"values"`
	Snips    map[string]string `json:"snips"`
}

// Records keep keys as strings, never rounded floating-point JSON numbers.
// Original values are requested explicitly and forgotten when the view closes.
type maskingRecords struct {
	owner                                  *advancedPanel
	source                                 string
	api, summaryAPI, detailAPI, previewAPI nativeAPI
	loaded                                 bool
	kind, by, agent, provider              string
	blocked                                bool
	days                                   int
	records                                []maskingRecord
	more                                   bool
	summary                                json.RawMessage
	selected                               *maskingRecord
	values                                 map[string]string
	sent                                   string
	original                               bool
	preview                                string
	previewResult                          json.RawMessage
	confirmClear                           bool
	shown                                  string
}

func (m *maskingRecords) Close() {
	m.api.close()
	m.summaryAPI.close()
	m.detailAPI.close()
	m.previewAPI.close()
	m.values = nil
	m.sent = ""
	m.owner.p.env.Activity.Report("ai-masking-records", false, false, nil)
}
func (m *maskingRecords) load(more bool) {
	q := url.Values{"limit": {"50"}, "kind": {m.kind}, "by": {m.by}, "agent": {m.agent}, "provider": {m.provider}}
	if m.blocked {
		q.Set("blocked", "1")
	}
	if more && len(m.records) > 0 {
		q.Set("before", m.records[len(m.records)-1].Key)
	}
	m.api.request(m.owner.p, "GET", "/api/masking/records?"+q.Encode(), nil, func(raw json.RawMessage) {
		var value struct {
			Records []maskingRecord `json:"records"`
			More    bool            `json:"more"`
		}
		if nativeDecode(&m.api, raw, &value) {
			if value.Records == nil {
				m.api.err = nativeT("invalidResponse")
				return
			}
			if more {
				m.records = append(m.records, value.Records...)
			} else {
				m.records = value.Records
			}
			m.more = value.More
			m.loaded = true
		}
	})
}
func (m *maskingRecords) loadSummary() {
	m.summaryAPI.request(m.owner.p, "GET", "/api/masking/summary?days="+kit.Itoa(m.days), nil, func(raw json.RawMessage) { m.summary = raw })
}
func (m *maskingRecords) selectRecord(key string) {
	m.detailAPI.request(m.owner.p, "GET", "/api/masking/record?key="+url.QueryEscape(key), nil, func(raw json.RawMessage) {
		var value maskingRecord
		if nativeDecode(&m.detailAPI, raw, &value) {
			m.selected = &value
			m.values = map[string]string{}
			m.sent = ""
		}
	})
}
func (m *maskingRecords) View(c *ui.Context, locked bool) {
	p := m.owner.p
	if p.extensions.state == nil {
		return
	}
	if m.source != p.extensions.state.URL {
		m.Close()
		m.source = p.extensions.state.URL
		m.loaded, m.selected = false, nil
		m.api.err, m.summaryAPI.err, m.detailAPI.err, m.previewAPI.err = "", "", "", ""
		m.records, m.summary, m.previewResult = nil, nil, nil
		m.shown = ""
	}
	p.env.Activity.Report("ai-masking-records", false, m.confirmClear && m.api.busy, nil)
	if !m.loaded && !m.api.busy && m.api.err == "" {
		m.load(false)
		m.loadSummary()
	}
	if p.maskRecord != "" && p.maskRecord != m.shown && !m.detailAPI.busy {
		m.shown = p.maskRecord
		m.selectRecord(p.maskRecord)
	}
	kit.Group(c, icons.ShieldCheck, nativeT("maskingRecords"), nativeT("maskingRecordsHint"), func() {
		ui.Row(c).FillWidth().Padding(12, 16).Gap(12).Wrap().Children(func() {
			for _, f := range []struct {
				key   string
				value *string
			}{{"maskKind", &m.kind}, {"maskFinder", &m.by}, {"client", &m.agent}, {"provider", &m.provider}} {
				ui.Column(c.Key(f.key)).Basis(160).Grow(1).Children(func() { nativeText(c, nativeT(f.key), f.value, locked) })
			}
		})
		kit.Toggle(c, &m.blocked, nativeT("blockedOnly"), "").Disabled(locked)
		nativeInt(c, nativeT("summaryDays"), &m.days, 1, 30, locked)
		ui.Row(c).Padding(12, 16).Gap(8).Wrap().Children(func() {
			if kit.Button(c, i18n.T("common.refresh"), icons.RefreshCw, false).Disabled(locked || m.api.busy || m.summaryAPI.busy).Clicked() {
				m.load(false)
				m.loadSummary()
			}
			if kit.GhostButton(c, nativeT("clearRecords"), icons.Trash2, false).Disabled(locked || m.api.busy).Clicked() {
				m.confirmClear = true
			}
			if kit.GhostButton(c, nativeT("privacySettings"), icons.Settings2, false).Clicked() {
				m.owner.focusPrivacy = true
			}
		})
		m.api.notice(c, func() { m.load(false) })
		m.summaryAPI.notice(c, m.loadSummary)
		if m.summary != nil {
			var sum struct {
				Requests int `json:"requests"`
				N        int `json:"n"`
				Distinct int `json:"distinct"`
				Blocked  int `json:"blocked"`
			}
			_ = json.Unmarshal(m.summary, &sum)
			kit.Help(c, nativeT("requests")+": "+kit.Itoa(sum.Requests)+" · "+nativeT("maskedValues")+": "+kit.Itoa(sum.N)+" · "+nativeT("distinctValues")+": "+kit.Itoa(sum.Distinct)+" · "+nativeT("blocked")+": "+kit.Itoa(sum.Blocked)).Padding(12, 16)
			open := ui.Local(ui.Column(c), "mask-summary", func() bool { return false })
			ui.Collapsible(c, nativeT("summaryDetails"), open, func() { nativeJSON(c, m.summary) }).Padding(12, 16)
		}
		for _, record := range m.records {
			label := record.Time + " · " + record.Provider + " · " + record.Model + " · " + kit.Itoa(record.Count)
			if record.Blocked {
				label += " · " + nativeT("blocked")
			}
			if kit.GhostButton(c.Key(record.Key), label, icons.FileText, false).FillWidth().Padding(12, 16).Disabled(m.detailAPI.busy).Clicked() {
				m.selectRecord(record.Key)
			}
		}
		if m.loaded && len(m.records) == 0 {
			kit.Help(c, nativeT("noMaskingRecords")).Padding(12, 16)
		}
		if m.more && kit.Button(c, nativeT("loadMore"), icons.ChevronDown, false).Disabled(m.api.busy).Clicked() {
			m.load(true)
		}
		kit.Stack(c, "", "", func() { m.previewView(c, locked) })
	})
	if kit.Dialog(c, &m.confirmClear, kit.DialogOptions{Title: nativeT("clearRecords"), Message: nativeT("clearRecordsHint"), Confirm: nativeT("clear"), Cancel: i18n.T("common.cancel"), Danger: true, Busy: m.api.busy}) {
		m.detailAPI.close()
		m.api.request(p, "POST", "/api/masking/clear", nil, func(raw json.RawMessage) {
			var receipt struct {
				Cleared bool `json:"cleared"`
			}
			if !nativeDecode(&m.api, raw, &receipt) || !receipt.Cleared {
				m.api.err = nativeT("invalidResponse")
				return
			}
			m.summaryAPI.close()
			m.summary = nil
			p.maskRecord = ""
			m.confirmClear = false
			m.loaded = false
			m.selected = nil
			m.values = nil
			m.sent = ""
		})
	}
	m.detailView(c, locked)
}
func (m *maskingRecords) previewView(c *ui.Context, locked bool) {
	open := ui.Local(ui.Column(c), "mask-preview", func() bool { return false })
	ui.Collapsible(c, nativeT("maskPreview"), open, func() {
		kit.Help(c, nativeT("maskPreviewHint"))
		ui.TextArea(c, &m.preview).FillWidth().MinHeight(120).Label(nativeT("previewText")).Disabled(locked || m.previewAPI.busy)
		if kit.Button(c, nativeT("preview"), icons.Play, false).Disabled(locked || m.previewAPI.busy || m.preview == "").Clicked() {
			m.previewAPI.request(m.owner.p, "POST", "/api/masking/preview", map[string]string{"text": m.preview}, func(raw json.RawMessage) { m.previewResult = raw })
		}
		m.previewAPI.notice(c, nil)
		if m.previewResult != nil {
			nativeJSON(c, m.previewResult)
		}
	})
}
func (m *maskingRecords) detailView(c *ui.Context, locked bool) {
	if m.selected == nil {
		m.detailAPI.notice(c, nil)
		return
	}
	p := m.owner.p
	record := m.selected
	open := true
	kit.Dialog(c, &open, kit.DialogOptions{Title: nativeT("maskingRecord"), Cancel: i18n.T("common.close"), Width: 840, Body: func() {
		kit.Help(c, record.Time+" · "+record.Agent+" · "+record.Provider+" · "+record.Model)
		m.detailAPI.notice(c, nil)
		for _, value := range record.Values {
			ui.Column(c.Key(value.Placeholder)).FillWidth().Gap(6).Padding(8, 0).Children(func() {
				kit.Strong(c, value.Kind+" · "+value.Action+" · "+value.By)
				kit.Mono(c, value.Placeholder).Selectable()
				kit.Help(c, value.Head+"… · "+kit.Itoa(value.Length)+" · ×"+kit.Itoa(value.Count))
				if text, ok := m.values[value.Placeholder]; ok {
					kit.Mono(c, text).Selectable()
				}
				ui.Row(c).Gap(8).Wrap().Children(func() {
					read := func(done func(string)) {
						if text, ok := m.values[value.Placeholder]; ok {
							done(text)
							return
						}
						m.detailAPI.request(p, "GET", "/api/masking/value?p="+url.QueryEscape(value.Placeholder), nil, func(raw json.RawMessage) {
							var v struct {
								Value string `json:"value"`
							}
							if nativeDecode(&m.detailAPI, raw, &v) {
								done(v.Value)
							}
						})
					}
					if kit.Button(c, nativeT("copy"), icons.Copy, false).Disabled(locked || m.detailAPI.busy || !value.Known).Clicked() {
						services := c.Services()
						read(func(text string) { services.WriteClipboard(text) })
					}
					if kit.Button(c, nativeT("neverMaskValue"), icons.ShieldCheck, false).Disabled(locked || m.detailAPI.busy || !value.Known).Clicked() {
						read(func(text string) {
							if text != "" {
								s := m.owner.settings()
								s.RedactAllow = unique(append(s.RedactAllow, text))
								m.owner.focusPrivacy = true
								m.selected, m.values, m.sent = nil, nil, ""
							}
						})
					}
					if kit.Button(c, nativeT("revealValue"), icons.Eye, false).Disabled(locked || m.detailAPI.busy || !value.Known).Clicked() {
						m.detailAPI.request(p, "GET", "/api/masking/value?p="+url.QueryEscape(value.Placeholder), nil, func(raw json.RawMessage) {
							var v struct {
								Value string `json:"value"`
							}
							if nativeDecode(&m.detailAPI, raw, &v) {
								m.values[value.Placeholder] = v.Value
							}
						})
					}
					if kit.Button(c, nativeT("maskThisKind"), icons.ShieldCheck, false).Disabled(locked).Clicked() {
						m.owner.stageMaskKinds([]string{value.Kind})
						open = false
					}
				})
				if !value.Known {
					kit.Help(c, nativeT("valueNotRetained"))
				}
				if snip := record.Snips[value.Placeholder]; snip != "" {
					kit.Mono(c, snip).Selectable()
				}
			})
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c, nativeT("sentBody"), icons.FileText, false).Disabled(m.detailAPI.busy).Clicked() {
				m.detailAPI.request(p, "GET", "/api/masking/sent?key="+url.QueryEscape(record.Key), nil, func(raw json.RawMessage) {
					var v struct {
						Body string `json:"body"`
					}
					if nativeDecode(&m.detailAPI, raw, &v) {
						m.sent = v.Body
						m.original = false
					}
				})
			}
			if kit.Button(c, nativeT("originalBody"), icons.Eye, false).Disabled(m.detailAPI.busy || m.sent == "").Clicked() {
				var ps []string
				for _, v := range record.Values {
					ps = append(ps, v.Placeholder)
				}
				m.detailAPI.request(p, "POST", "/api/masking/values", map[string]any{"ps": ps}, func(raw json.RawMessage) {
					var v struct {
						Values map[string]string `json:"values"`
					}
					if nativeDecode(&m.detailAPI, raw, &v) {
						for placeholder, value := range v.Values {
							m.values[placeholder] = value
						}
						m.original = true
					}
				})
			}
		})
		if m.sent != "" {
			text := m.sent
			if m.original {
				for placeholder, value := range m.values {
					text = strings.ReplaceAll(text, placeholder, value)
				}
			}
			kit.Mono(c, text).FillWidth().MinWidth(0).Selectable()
		}
	}})
	if !open {
		m.selected = nil
		m.values = nil
		m.sent = ""
		m.detailAPI.close()
	}
}

// Staging a record's action changes the same saved privacy draft, never an
// unrelated live runtime snapshot; unknown/malformed kinds cannot be injected.
func (a *advancedPanel) stageMaskKinds(kinds []string) {
	if a.p.locked() {
		return
	}
	for _, kind := range kinds {
		if !maskKinds.MatchString(kind) {
			return
		}
	}
	s := a.settings()
	next := map[string]string{}
	for kind, action := range s.RedactActions {
		keep := true
		for _, asked := range kinds {
			if asked == kind {
				keep = false
			}
		}
		if keep {
			next[kind] = action
		}
	}
	s.RedactActions = next
	a.focusPrivacy = true
}

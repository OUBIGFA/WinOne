package aiservices

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// These are read-only projections of upstream routing evidence. The complete
// original remains available below, while common decisions and actions have
// labeled native controls rather than making the owner navigate raw JSON.
type traceEvidence struct {
	Order []traceCandidate `json:"order"`
	Left  []traceCandidate `json:"left"`
	Tries []struct {
		ID     string     `json:"id"`
		Model  string     `json:"model"`
		Status int        `json:"status"`
		Error  string     `json:"error"`
		Fail   string     `json:"fail"`
		Millis int        `json:"ms"`
		Effort string     `json:"effort"`
		Rest   *traceRest `json:"rest"`
	} `json:"tries"`
	Group    json.RawMessage `json:"group"`
	Rule     json.RawMessage `json:"rule"`
	Nested   json.RawMessage `json:"nested"`
	Affinity json.RawMessage `json:"affinity"`
	Prompt   struct {
		Window int `json:"window"`
		Tokens int `json:"tokens"`
		Parts  []struct {
			Kind   string `json:"kind"`
			Tokens int    `json:"tokens"`
			Items  []struct {
				Name   string `json:"name"`
				Tokens int    `json:"tokens"`
			} `json:"items"`
		} `json:"parts"`
	} `json:"prompt"`
}
type traceRest struct {
	Why    string `json:"why"`
	Status int    `json:"status"`
	Until  string `json:"until"`
	Key    string `json:"key"`
	Link   string `json:"link"`
}
type traceCandidate struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Who      string     `json:"who"`
	Provider string     `json:"provider"`
	Model    string     `json:"model"`
	Plan     string     `json:"plan"`
	Known    bool       `json:"known"`
	Used     float64    `json:"used"`
	Rest     *traceRest `json:"rest"`
	UseFirst bool       `json:"useFirst"`
	Barred   bool       `json:"barred"`
	Held     bool       `json:"held"`
	Capped   int        `json:"capped"`
	Busy     float64    `json:"busy"`
	Window   int        `json:"window"`
	Free     bool       `json:"free"`
}

func (t *nativeTrace) evidenceView(c *ui.Context, p *page, r *traceRoute) {
	var evidence traceEvidence
	if json.Unmarshal(r.Raw, &evidence) != nil {
		return
	}
	for _, section := range []struct {
		label string
		rows  []traceCandidate
	}{{"candidateOrder", evidence.Order}, {"excludedCandidates", evidence.Left}} {
		if len(section.rows) == 0 {
			continue
		}
		kit.Strong(c, nativeT(section.label))
		for index, row := range section.rows {
			ui.Column(c.Key(section.label+row.ID)).FillWidth().Gap(5).Padding(8, 0).Children(func() {
				kit.Strong(c, fmt.Sprintf("%d. %s · %s · %s", index+1, row.Name, row.Who, row.Model)).Selectable()
				var facts []string
				if row.Known {
					facts = append(facts, nativeT("quotaUsed")+fmt.Sprintf(" %.1f%%", row.Used*100))
				}
				if row.UseFirst {
					facts = append(facts, nativeT("useFirst"))
				}
				if row.Free {
					facts = append(facts, nativeT("free"))
				}
				if row.Barred {
					facts = append(facts, nativeT("modelExcluded"))
				}
				if row.Held {
					facts = append(facts, nativeT("accountRestricted"))
				}
				if row.Capped > 0 {
					facts = append(facts, fmt.Sprintf("%s %d%%", nativeT("quotaCap"), row.Capped))
				}
				if row.Busy > 0 {
					facts = append(facts, fmt.Sprintf("%s %.0f%%", nativeT("busyLoad"), row.Busy))
				}
				if row.Window > 0 {
					facts = append(facts, fmt.Sprintf("%s %d", nativeT("context"), row.Window))
				}
				if len(facts) > 0 {
					kit.Help(c, strings.Join(facts, " · "))
				}
				t.restView(c, p, row.Rest)
			})
		}
	}
	if len(evidence.Tries) > 0 {
		kit.Strong(c, nativeT("attempts"))
		for i, attempt := range evidence.Tries {
			ui.Column(c.Key(i)).FillWidth().Gap(4).Padding(8, 0).Children(func() {
				kit.Strong(c, fmt.Sprintf("%d. %s · %s", i+1, attempt.ID, attempt.Model))
				kit.Help(c, fmt.Sprintf("HTTP %d · %d ms · %s", attempt.Status, attempt.Millis, attempt.Effort))
				if attempt.Error != "" {
					kit.Help(c, attempt.Error).Selectable()
				}
				if attempt.Fail != "" {
					kit.Help(c, attempt.Fail).Selectable()
				}
				t.restView(c, p, attempt.Rest)
			})
		}
	}
	for _, part := range []struct {
		key   string
		value json.RawMessage
	}{{"groupDecision", evidence.Group}, {"matchedRule", evidence.Rule}, {"nestedRules", evidence.Nested}, {"affinityDecision", evidence.Affinity}} {
		if len(part.value) == 0 || string(part.value) == "null" {
			continue
		}
		open := ui.Local(ui.Column(c.Key(part.key)), "open", func() bool { return false })
		ui.Collapsible(c, nativeT(part.key), open, func() { nativeJSON(c, part.value) })
	}
	if len(evidence.Prompt.Parts) > 0 {
		kit.Strong(c, nativeT("contextText"))
		kit.Help(c, fmt.Sprintf("%d / %d tokens", evidence.Prompt.Tokens, evidence.Prompt.Window))
		for _, part := range evidence.Prompt.Parts {
			label := i18n.T("usage.part" + part.Kind)
			ui.Column(c.Key(part.Kind)).FillWidth().Gap(4).Children(func() {
				kit.Strong(c, label+" · "+kit.Itoa(part.Tokens))
				if kit.Button(c, nativeT("showContext"), icons.FileText, false).Disabled(t.detailAPI.busy).Clicked() {
					t.contextRead(p, r, part.Kind, -1)
				}
				for index, item := range part.Items {
					if kit.GhostButton(c.Key(index), item.Name+" · "+kit.Itoa(item.Tokens), icons.FileText, false).FillWidth().Disabled(t.detailAPI.busy).Clicked() {
						t.contextRead(p, r, part.Kind, index)
					}
				}
			})
		}
	}
	open := ui.Local(ui.Column(c), "all-evidence", func() bool { return false })
	ui.Collapsible(c, nativeT("completeEvidence"), open, func() { nativeJSON(c, r.Raw) })
}
func (t *nativeTrace) contextRead(p *page, r *traceRoute, kind string, item int) {
	q := url.Values{"id": {strconv.FormatInt(r.ID, 10)}, "kind": {kind}, "day": {strings.Split(r.Time, "T")[0]}}
	if item >= 0 {
		q.Set("item", kit.Itoa(item))
	}
	t.detailRead(p, "context", "/api/context/text?"+q.Encode())
}
func (t *nativeTrace) restView(c *ui.Context, p *page, rest *traceRest) {
	if rest == nil {
		return
	}
	kit.Help(c, nativeT("resting")+" · "+rest.Why+" · "+rest.Until)
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if rest.Key != "" && kit.Button(c, nativeT("unrest"), icons.RefreshCw, false).Disabled(t.detailAPI.busy).Clicked() {
			t.detailAPI.request(p, "POST", "/api/gateway/unrest", map[string]string{"key": rest.Key}, func(raw json.RawMessage) { t.detail = raw; t.load(p) })
		}
		if parsed, err := url.Parse(rest.Link); err == nil && parsed.Scheme == "https" && parsed.Host != "" {
			ui.Link(c, nativeT("verifyAccount"), rest.Link)
		}
	})
}

package pluginhost

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"golang.org/x/net/html"
	"maps"
	"reflect"
	"strings"
	"winone/desktopmygo/chat/workspace"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
)

type cardFormField struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	Multiline bool   `json:"multiline"`
	Row       string `json:"row"`
	Collapsed bool   `json:"collapsed"`
	Kind      string `json:"kind"`
}

// Same three-way merge as Cindy mergeGhostCardFormValues: adopt remote updates
// only where the owner has no newer local edit. A broadcast never erases typing.
func mergeCardFields(item *card) {
	if item.Mail != nil {
		item.Fields = item.Mail.Fields
	}
	next, local := map[string]string{}, map[string]string{}
	for _, field := range item.Fields {
		next[field.Name] = field.Value
		previous, has := item.values[field.Name]
		if !has || previous == item.baseValues[field.Name] {
			local[field.Name] = field.Value
		} else {
			local[field.Name] = previous
		}
	}
	item.baseValues, item.values = next, local
}
func hasAttr(n *html.Node, name string) bool {
	for _, value := range n.Attr {
		if value.Key == name {
			return true
		}
	}
	return false
}
func (h *Host) cardFields(c *ui.Context, item *card) {
	if item.Mail != nil || isLegacyMail(item) {
		h.mailFields(c, item, false)
		return
	}
	for _, field := range item.Fields {
		if field.Kind == "hidden" {
			continue
		}
		value := item.values[field.Name]
		if field.Collapsed && value == "" {
			if item.expanded == nil {
				item.expanded = map[string]bool{}
			}
			if !item.expanded[field.Name] {
				if kit.GhostButton(c.Key(field.Name), field.Label, nil, false).Clicked() {
					item.expanded[field.Name] = true
				}
				continue
			}
		}
		ui.Field(c.Key(field.Name), field.Label, func() {
			changed := false
			if field.Multiline || field.Kind == "mail-html" {
				changed = ui.TextArea(c, &value).FillWidth().Height(120).Label(field.Label).Disabled(item.busy).Changed()
			} else {
				changed = ui.TextInput(c, &value).FillWidth().Label(field.Label).Disabled(item.busy).Changed()
			}
			if changed {
				item.values[field.Name] = value
				item.formDirty = true
			}
		})
	}
	if item.formDirty && !item.formSending && item.FormChange != "" && item.err == "" {
		h.flushCardForm(item)
	}
	kit.InlineError(c, item.err)
	if item.err != "" && item.FormChange != "" && kit.GhostButton(c, text("retry"), nil, false).Clicked() {
		item.err = ""
		item.formDirty = true
	}
}
func validCardValues(values map[string]string) bool {
	for name, value := range values {
		limit := 16000
		if name == "bodyHtml" || name == "quoteHtml" {
			limit = 64000
		}
		if cardValueLength(value) > limit {
			return false
		}
	}
	return true
}

// JavaScript's field contract counts UTF-16 units, not Unicode code points.
func cardValueLength(value string) int {
	size := 0
	for _, r := range value {
		size++
		if r > 0xffff {
			size++
		}
	}
	return size
}
func (h *Host) flushCardForm(item *card) {
	if h.closed || item.formSending || item.FormChange == "" || !mailCanEdit(item) {
		return
	}
	if !validCardValues(item.values) {
		item.err = text("formTooLong")
		settleCardForm(item, false)
		return
	}
	sent := maps.Clone(item.values)
	generation := item.formGeneration
	action := item.FormChange
	item.formDirty = false
	item.formSending = true
	h.call("card-action", map[string]any{"callId": item.CallID, "action": action, "formValues": sent, "change": true}, func(raw json.RawMessage, err error) {
		if h.closed || generation != item.formGeneration {
			return
		}
		item.formSending = false
		item.err = cardActionError(raw, err)
		if item.err != "" && mailCanEdit(item) {
			item.formDirty = true
		}
		if item.err == "" && !reflect.DeepEqual(sent, item.values) && mailCanEdit(item) {
			item.formDirty = true
		}
		if item.err == "" && item.formDirty && item.FormChange != "" {
			h.flushCardForm(item)
			return
		}
		settleCardForm(item, item.err == "" && !item.formDirty)
	})
}
func settleCardForm(item *card, ok bool) {
	waiters := item.formWaiters
	item.formWaiters = nil
	for _, done := range waiters {
		done(ok)
	}
}

// A new logical form must not inherit old typing or late delivery callbacks.
func retireCardForm(item *card) {
	item.formGeneration++
	item.values = nil
	item.baseValues = nil
	item.formDirty = false
	item.formSending = false
	item.busy = false
	item.err = ""
	item.expanded = nil
	settleCardForm(item, false)
}
func cardActionError(raw json.RawMessage, err error) string {
	if err != nil {
		return err.Error()
	}
	var result struct {
		OK     bool
		Reason string
	}
	if json.Unmarshal(raw, &result) != nil {
		return text("actionFailed")
	}
	if !result.OK && result.Reason != "cancelled" {
		return text("actionFailed") + ": " + result.Reason
	}
	return ""
}
func (h *Host) cardDispatch(item *card, method string, params map[string]any) {
	if item.Mail != nil || isLegacyMail(item) {
		h.mailDispatch(item, method, params)
		return
	}
	h.deliverCardAction(item, method, params)
}
func (h *Host) deliverCardAction(item *card, method string, params map[string]any) {
	if h.closed {
		return
	}
	if !validCardValues(item.values) {
		item.err = text("formTooLong")
		return
	}
	params["callId"] = item.CallID
	if len(item.Fields) > 0 && params["formValues"] == nil {
		params["formValues"] = maps.Clone(item.values)
	}
	generation := item.formGeneration
	item.busy = true
	item.err = ""
	h.call(method, params, func(raw json.RawMessage, err error) {
		if h.closed || generation != item.formGeneration {
			return
		}
		item.busy = false
		item.err = cardActionError(raw, err)
	})
}
func (h *Host) cardPrompt(c *ui.Context, item *card) {
	if item.promptAction == "" {
		return
	}
	open := true
	if kit.Dialog(c.Key("prompt:"+item.CallID), &open, kit.DialogOptions{Title: text("actionPrompt"), Confirm: text("allow"), Cancel: text("cancel"), ConfirmDisabled: len([]rune(item.promptValue)) > 2000, Body: func() {
		ui.TextArea(c, &item.promptValue).Height(140).FillWidth().Label(first(item.promptHint, text("actionPrompt"))).Placeholder(item.promptHint)
	}}) {
		action, value := item.promptAction, item.promptValue
		item.promptAction = ""
		h.cardDispatch(item, "card-action", map[string]any{"action": action, "prompt": value})
	} else if !open {
		item.promptAction = ""
	}
}
func (h *Host) cardMedia(c *ui.Context, key, source, kind string) {
	if source == "" {
		return
	}
	s := h.cards
	s.mediaVisible[source] = true
	pane := s.media[source]
	if pane == nil && !s.asked[source] {
		s.asked[source] = true
		core.Go(h.env, func() (workspace.FilePreview, error) {
			var value workspace.FilePreview
			err := h.env.Host.CallInto("chat.ghosts.media-preview", map[string]string{"url": source}, &value)
			return value, err
		}, func(value workspace.FilePreview, err error) {
			if h.closed {
				return
			}
			if err == nil {
				s.media[source], err = workspace.NewMediaPane(h.env, value)
			}
			if err != nil && h.env.Toast != nil {
				h.env.Toast(fmt.Sprintf("%s: %s", text("mediaFailed"), err))
			}
		})
	}
	if pane != nil {
		height := float32(110)
		if kind == "video" {
			height = 320
		}
		ui.Column(c.Key(key)).FillWidth().Height(height).Children(func() { pane.View(c) })
	} else {
		kit.Caption(c, strings.TrimSpace(text("loadingMedia")))
	}
}

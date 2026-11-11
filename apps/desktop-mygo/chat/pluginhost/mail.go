package pluginhost

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/workspace"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// MailCardData is Cindy's Host-created marker, not a plugin's ordinary HTML.
// Provider/account/draft identify the entity; key identifies the current editor.
// The provider SDK remains the authority for synchronization, grace and receipts.
type mailCardData struct {
	AccountID     string            `json:"accountId"`
	DraftID       string            `json:"draftId"`
	Key           string            `json:"key"`
	From          string            `json:"from"`
	Fields        []cardFormField   `json:"fields"`
	BaseValues    map[string]string `json:"baseValues"`
	Attachments   []string          `json:"attachments"`
	Images        []mailInlineImage `json:"images"`
	State         string            `json:"state"`
	Status        string            `json:"status"`
	SendLabel     string            `json:"sendLabel"`
	FromLabel     string            `json:"fromLabel"`
	ContinueLabel string            `json:"continueLabel"`
	DeleteLabel   string            `json:"deleteLabel"`
	SupersededBy  string            `json:"supersededBy"`
	PendingSend   *mailPendingSend  `json:"pendingSend"`
}
type mailInlineImage struct {
	CID string `json:"cid"`
	URL string `json:"url"`
}
type mailPendingSend struct {
	Deadline     int64  `json:"deadline"`
	Label        string `json:"label"`
	CancelLabel  string `json:"cancelLabel"`
	SendNowLabel string `json:"sendNowLabel"`
}
type mailAttachment struct {
	Key        string `json:"key"`
	Filename   string `json:"filename"`
	Bytes      *int64 `json:"bytes"`
	ContentID  string `json:"contentId"`
	MIME       string `json:"mimeType"`
	PreviewURL string `json:"previewUrl"`
}
type mailRecipient struct {
	Address string `json:"address"`
	Label   string `json:"label"`
}
type mailFormModel struct {
	Attachments []mailAttachment           `json:"attachments"`
	Images      []mailInlineImage          `json:"images"`
	Insertable  []string                   `json:"insertable"`
	Recipients  map[string][]mailRecipient `json:"recipients"`
	source      string
	loading     bool
	err         string
}
type mailController struct {
	models           map[string]*mailFormModel
	documents        map[string]*mailHTMLDocument
	enlarged         string
	enlargedSnapshot bool
	enlargedErr      string
	reader           *mailReaderView
	preview          *workspace.DocumentPane
	previewSource    string
	browserOpener    func(session, address string)
	resolving        bool
	closed           bool
}

func newMailController() *mailController {
	return &mailController{models: map[string]*mailFormModel{}, documents: map[string]*mailHTMLDocument{}}
}
func mailIdentity(mail *mailCardData) string {
	if mail == nil {
		return ""
	}
	return strings.Join([]string{mail.AccountID, mail.DraftID, mail.Key}, "\x00")
}
func isLegacyMail(item *card) bool {
	return item.Mail == nil && strings.HasPrefix(item.FormChange, "mail:change:")
}
func mailCanEdit(item *card) bool {
	if item.Mail == nil {
		return !isLegacyMail(item)
	}
	return item.Mail.SupersededBy == "" && slices.Contains([]string{"draft", "syncing", "error"}, item.Mail.State)
}
func mailStatus(mail *mailCardData, now time.Time) string {
	if mail.PendingSend == nil {
		return mail.Status
	}
	seconds := max(0, int(math.Ceil(float64(mail.PendingSend.Deadline-now.UnixMilli())/1000)))
	return strings.ReplaceAll(mail.PendingSend.Label, "{seconds}", fmt.Sprint(seconds))
}
func mailSendProgress(mail *mailCardData, now time.Time) float64 {
	if mail.PendingSend == nil {
		return 0
	}
	return 1 - math.Max(0, math.Min(1, float64(mail.PendingSend.Deadline-now.UnixMilli())/10000))
}

// These chrome labels adapt the pinned renderer to the native window. Mail
// status and operation labels themselves always come from the original SDK.
func mailText(key string) string {
	copy := map[string][2]string{
		"preview": {"邮件预览", "Mail preview"}, "enlarge": {"放大邮件", "Enlarge mail"}, "attach": {"添加附件", "Add attachment"}, "inline": {"添加内嵌图片", "Add inline image"},
		"insertImage": {"插入图片", "Insert image"}, "remove": {"移除附件", "Remove attachment"}, "expand": {"展开字段", "Expand fields"}, "collapse": {"收起字段", "Collapse fields"},
		"updated": {"草稿已更新（只读历史）", "Updated draft (read-only history)"}, "legacy": {"旧草稿卡片（只读）", "Earlier draft card (read only)"},
		"failed": {"未能读取邮件，请重试。", "Could not read the mail. Please retry."}, "unavailable": {"此邮件预览已不可用。", "This mail preview is no longer available."},
		"editorUnavailable": {"邮件编辑器暂不可用，请重试。", "The mail editor is unavailable. Please retry."}, "syncFailed": {"草稿编辑未能同步，请重试后再操作。", "Draft edits were not synchronized. Retry before continuing."},
		"externalImages": {"显示外部图片", "Show external images"}, "markReadFailed": {"未能标记为已读。", "Could not mark this message as read."},
		"conversationFailed": {"未能读取完整邮件会话。", "Could not load the whole mail conversation."}, "providerChoice": {"选择邮件插件", "Choose a mail reader"},
		"to": {"收件人", "To"}, "cc": {"抄送", "Cc"}, "bcc": {"密送", "Bcc"}, "from": {"发件人", "From"}, "subject": {"主题", "Subject"},
		"showMessages": {"显示其余邮件", "Show more messages"}, "currentMessage": {"当前邮件", "Current message"}, "openMailbox": {"在邮箱中打开", "Open in mailbox"},
	}
	index := 0
	if i18n.Locale() == i18n.English {
		index = 1
	}
	return copy[key][index]
}

func (h *Host) mailCard(c *ui.Context, item *card, expanded bool) {
	if h.env.Activity != nil {
		defer func() {
			editable := mailCanEdit(item)
			h.env.Activity.Report("plugin-mail:"+item.CallID, editable && item.formDirty, item.formSending || item.busy, func() {
				item.values = maps.Clone(item.baseValues)
				item.formDirty = false
				item.err = ""
			})
		}()
	}
	if item.Mail == nil || item.Mail.SupersededBy != "" {
		if item.expanded == nil {
			item.expanded = map[string]bool{}
		}
		open := item.expanded["mail-history"]
		label := mailText("updated")
		if item.Mail == nil {
			label = mailText("legacy")
		}
		ui.Collapsible(c, label, &open, func() {
			ui.Column(c).FillWidth().MinWidth(0).Gap(8).Padding(8).Children(func() { h.mailFields(c, item, expanded) })
		})
		item.expanded["mail-history"] = open
		return
	}
	ui.Column(c).FillWidth().MinWidth(0).Gap(8).Padding(10).Radius(8).Border(1, c.Theme().Border).Children(func() {
		h.mailFields(c, item, expanded)
		h.mailFooter(c, item)
	})
}

func (h *Host) mailModel(item *card) *mailFormModel {
	s := h.cards.mail
	images := []mailInlineImage{}
	if item.Mail != nil {
		images = item.Mail.Images
	}
	signature, _ := json.Marshal([]any{item.values, images})
	model := s.models[item.CallID]
	if model != nil && model.source == string(signature) {
		return model
	}
	model = &mailFormModel{source: string(signature), loading: true}
	s.models[item.CallID] = model
	generation := item.formGeneration
	h.call("mail-ui", map[string]any{"op": "model", "values": maps.Clone(item.values), "images": images, "reading": item.readerMail}, func(raw json.RawMessage, err error) {
		if h.closed || s.closed || generation != item.formGeneration || s.models[item.CallID] != model {
			return
		}
		model.loading = false
		if err != nil {
			model.err = err.Error()
			return
		}
		if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, model) != nil {
			model.err = mailText("editorUnavailable")
		}
	})
	return model
}
func (h *Host) mailFields(c *ui.Context, item *card, expanded bool) {
	readOnly := !mailCanEdit(item)
	if item.expanded == nil {
		item.expanded = map[string]bool{}
	}
	model := h.mailModel(item)
	var recipients []cardFormField
	for _, field := range item.Fields {
		if field.Kind == "recipients" {
			recipients = append(recipients, field)
		}
	}
	if len(recipients) > 0 {
		h.mailRecipients(c, item, recipients, model, readOnly)
	}
	for _, field := range item.Fields {
		if field.Kind == "hidden" || field.Kind == "recipients" {
			continue
		}
		switch field.Kind {
		case "mail-html":
			h.mailHTML(c, item, field, expanded, !readOnly)
		case "mail-attachments":
			h.mailAttachments(c, item, field, model, readOnly)
		default:
			draw := func() {
				if field.Name == "body" && item.Mail != nil && !expanded {
					ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
						if kit.IconButton(c, icons.Maximize, mailText("enlarge")).Clicked() {
							h.enlargeMail(item)
						}
					})
				}
				value := item.values[field.Name]
				if readOnly {
					ui.Text(c.Key(field.Name), value).Selectable().MinWidth(0)
					return
				}
				if field.Multiline {
					maximum := 22
					if expanded {
						maximum = 100000
					}
					input := ui.TextArea(c.Key(field.Name), &value).FillWidth().Lines(3, maximum).MaxLength(16000).Label(field.Label).Disabled(item.busy)
					if !expanded {
						input = input.MaxHeight(440)
					}
					if input.Changed() {
						h.mailChange(item, field.Name, value)
					}
				} else {
					label := ui.Text(c, field.Label).TextColor(c.Theme().TextMuted)
					input := kit.TextInput(c.Key(field.Name), &value).FillWidth().MaxLength(16000).LabelledBy(label).Label(field.Label).Disabled(item.busy)
					if input.Changed() {
						h.mailChange(item, field.Name, value)
					}
				}
			}
			if field.Collapsed {
				open := item.expanded[field.Name]
				ui.Collapsible(c.Key(field.Name), field.Label, &open, draw)
				item.expanded[field.Name] = open
			} else {
				ui.Column(c.Key(field.Name)).FillWidth().MinWidth(0).Gap(4).Children(draw)
			}
		}
	}
	if model.err != "" {
		kit.InlineError(c, model.err)
		if kit.Button(c, text("retry"), nil, false).Clicked() {
			delete(h.cards.mail.models, item.CallID)
		}
	}
	if item.formDirty && !item.formSending && item.err == "" && mailCanEdit(item) && item.FormChange != "" {
		h.flushCardForm(item)
	}
	kit.InlineError(c, item.err)
	if item.err != "" && mailCanEdit(item) && kit.Button(c, text("retry"), nil, false).Clicked() {
		item.err = ""
		item.formDirty = true
		h.flushCardForm(item)
	}
}
func (h *Host) mailChange(item *card, name, value string) {
	if !mailCanEdit(item) {
		return
	}
	item.values[name] = value
	item.formDirty = true
	if !validCardValues(item.values) {
		item.err = text("formTooLong")
	}
}
func (h *Host) mailRecipients(c *ui.Context, item *card, fields []cardFormField, model *mailFormModel, readOnly bool) {
	if item.readerMail {
		// The Reader always shows every nonempty header, including Bcc. Its
		// compose summary/expansion rules must not hide a read-only recipient.
		for _, field := range fields {
			if strings.TrimSpace(item.values[field.Name]) == "" {
				continue
			}
			ui.Row(c.Key(field.Name)).FillWidth().MinWidth(0).Gap(6).AlignItems(ui.Start).Children(func() {
				ui.Text(c, field.Label).TextColor(c.Theme().TextMuted).Shrink(0)
				ui.Text(c, item.values[field.Name]).Selectable().Grow(1).MinWidth(0)
			})
		}
		return
	}
	expanded := item.expanded["recipients"]
	ui.Column(c.Key("recipients")).FillWidth().MinWidth(0).Gap(5).Children(func() {
		if expanded {
			for i, field := range fields {
				ui.Row(c.Key(field.Name)).FillWidth().MinWidth(0).Gap(6).AlignItems(ui.Start).Children(func() {
					label := ui.Text(c, field.Label).TextColor(c.Theme().TextMuted).Shrink(0)
					value := item.values[field.Name]
					if readOnly {
						ui.Text(c, value).Selectable().Grow(1).MinWidth(0)
					} else {
						input := ui.TextArea(c, &value).FillWidth().Lines(1, 16).MaxLength(16000).LabelledBy(label).Label(field.Label).Disabled(item.busy)
						if input.Changed() {
							h.mailChange(item, field.Name, value)
						}
					}
					if i == 0 && kit.GhostButton(c, mailText("collapse"), nil, false).Clicked() {
						item.expanded["recipients"] = false
					}
				})
			}
			return
		}
		ui.Row(c).FillWidth().MinWidth(0).Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
			for _, field := range fields {
				if field.Name == "bcc" || field.Collapsed {
					continue
				}
				ui.Text(c, field.Label).TextColor(c.Theme().TextMuted)
				entries := model.Recipients[field.Name]
				for _, entry := range entries {
					if kit.GhostButton(c, entry.Label, nil, false).Tooltip(entry.Address).Label(field.Label + ": " + entry.Address).MaxWidthPercent(100).Clicked() {
						item.expanded["recipients"] = true
					}
				}
				if len(entries) == 0 && item.values[field.Name] != "" {
					ui.Text(c, item.values[field.Name]).Selectable().MinWidth(0)
				}
			}
			if kit.GhostButton(c, mailText("expand"), nil, false).Expanded(false).Clicked() {
				item.expanded["recipients"] = true
			}
		})
	})
}
func (h *Host) mailAttachments(c *ui.Context, item *card, field cardFormField, model *mailFormModel, readOnly bool) {
	ui.Column(c.Key(field.Name)).FillWidth().MinWidth(0).Gap(5).Children(func() {
		if !readOnly {
			ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
				if kit.GhostButton(c, mailText("attach"), icons.Paperclip, false).Disabled(item.busy || item.formSending).Clicked() {
					h.cardDispatch(item, "card-action", map[string]any{"action": "mail:attach:" + item.Mail.Key})
				}
				if _, ok := item.values["bodyHtml"]; ok && kit.GhostButton(c, mailText("inline"), icons.Image, false).Disabled(item.busy || item.formSending).Clicked() {
					h.cardDispatch(item, "card-action", map[string]any{"action": "mail:image:" + item.Mail.Key})
				}
			})
		}
		for _, file := range model.Attachments {
			ui.Row(c.Key(file.Key)).FillWidth().MinWidth(0).Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
				button := kit.Button(c, file.Filename, icons.File, false).Tooltip(file.Filename).Label(file.Filename).MinWidth(0).MaxWidthPercent(100).Disabled(file.PreviewURL == "")
				if button.Clicked() {
					h.openMailImage(file.PreviewURL)
				}
				if file.Bytes != nil {
					kit.Caption(c, mailBytes(*file.Bytes))
				}
				if !readOnly && slices.Contains(model.Insertable, file.Key) && kit.GhostButton(c, mailText("insertImage"), nil, false).Disabled(item.busy || item.formSending || model.loading).Clicked() {
					h.mailMutate(item, "insert", field.Name, file.Key)
				}
				if !readOnly && kit.IconButton(c, icons.X, mailText("remove")+": "+file.Filename).Disabled(item.busy || item.formSending || model.loading).Clicked() {
					h.mailMutate(item, "remove", field.Name, file.Key)
				}
			})
		}
	})
}
func mailBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(bytes)/(1024*1024))
}

func (h *Host) mailFooter(c *ui.Context, item *card) {
	mail := item.Mail
	if mail == nil || mail.SupersededBy != "" {
		return
	}
	ui.Column(c.Key("mail-footer")).FillWidth().MinWidth(0).Gap(5).Children(func() {
		kit.Caption(c, mail.FromLabel+": "+mail.From).Selectable().MinWidth(0)
		if len(mail.Attachments) > 0 && !slices.ContainsFunc(mail.Fields, func(field cardFormField) bool { return field.Kind == "mail-attachments" }) {
			kit.Caption(c, strings.Join(mail.Attachments, " · ")).Selectable().MinWidth(0)
		}
		status := mailStatus(mail, time.Now())
		role := ui.RoleStatus
		if mail.PendingSend != nil {
			role = ui.RoleTimer
			c.After(250 * time.Millisecond)
		}
		ui.Text(c, status).TextColor(c.Theme().TextMuted).FontSize(kit.HelpSize(c)).Role(role).MinWidth(0).Selectable()
		ui.Row(c).FillWidth().MinWidth(0).Wrap().Gap(6).Justify(ui.End).Children(func() {
			if mailCanEdit(item) {
				if mail.DeleteLabel != "" && kit.Button(c, mail.DeleteLabel, nil, false).Disabled(item.busy).Clicked() {
					h.cardDispatch(item, "card-action", map[string]any{"action": "mail:delete:" + mail.Key})
				}
				if kit.PrimaryButton(c, mail.SendLabel, nil, item.busy).Disabled(item.busy || item.err != "").Clicked() {
					h.cardDispatch(item, "card-action", map[string]any{"action": "mail:send:" + mail.Key})
				}
			}
			if mail.PendingSend != nil {
				if kit.Button(c, mail.PendingSend.CancelLabel, nil, false).Disabled(item.busy).Clicked() {
					h.cardDispatch(item, "card-action", map[string]any{"action": "mail:cancel:" + mail.Key})
				}
				if kit.Button(c, mail.PendingSend.SendNowLabel, nil, false).Label(mail.PendingSend.SendNowLabel + ", " + status).Disabled(item.busy).Clicked() {
					h.cardDispatch(item, "card-action", map[string]any{"action": "mail:now:" + mail.Key})
				}
			}
			if mail.State == "sent" && mail.ContinueLabel != "" && kit.Button(c, mail.ContinueLabel, nil, item.busy).Disabled(item.busy).Clicked() {
				h.cardDispatch(item, "card-action", map[string]any{"action": "mail:continue:" + mail.Key})
			}
		})
		if mail.PendingSend != nil {
			ui.Progress(c, mailSendProgress(mail, time.Now())).FillWidth().Label(status)
		}
	})
}

// Native action delivery is not send completion. Captured HTML and every prior
// form delivery must settle before Send/attachment selection, as in Cindy.
func (h *Host) mailDispatch(item *card, method string, params map[string]any) {
	if h.closed || item.Mail == nil || item.busy || method != "card-action" {
		return
	}
	action, _ := params["action"].(string)
	prefix := "mail:"
	suffix := ":" + item.Mail.Key
	if !strings.HasPrefix(action, prefix) || !strings.HasSuffix(action, suffix) {
		return
	}
	operation := strings.TrimSuffix(strings.TrimPrefix(action, prefix), suffix)
	needsForm := slices.Contains([]string{"send", "attach", "image"}, operation)
	allowed := needsForm && mailCanEdit(item) || operation == "delete" && mailCanEdit(item) && item.Mail.DeleteLabel != "" || slices.Contains([]string{"cancel", "now"}, operation) && item.Mail.PendingSend != nil || operation == "continue" && item.Mail.State == "sent" && item.Mail.ContinueLabel != ""
	if !allowed || item.Mail.SupersededBy != "" {
		return
	}
	if !needsForm {
		// Delete discards unsaved edits rather than writing them back first.
		if operation == "delete" {
			item.formDirty = false
		}
		h.deliverCardAction(item, method, params)
		return
	}
	generation := item.formGeneration
	item.busy = true
	h.captureMailSession(item.SessionID, func(ok bool) {
		if h.closed || generation != item.formGeneration {
			return
		}
		item.busy = false
		if !ok || !mailCanEdit(item) {
			item.err = mailText("syncFailed")
			return
		}
		params["formValues"] = maps.Clone(item.values)
		h.deliverCardAction(item, method, params)
	})
}
func (h *Host) waitMailDeliveries(session string, done func(bool)) {
	var items []*card
	for _, item := range h.cards.byCall {
		if item.SessionID == session && (item.formDirty || item.formSending || item.err != "") && mailCanEdit(item) {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		done(true)
		return
	}
	remaining, ok := len(items), true
	for _, item := range items {
		finish := func(success bool) {
			ok = ok && success
			remaining--
			if remaining == 0 {
				done(ok)
			}
		}
		if item.err != "" {
			finish(false)
			continue
		}
		item.formWaiters = append(item.formWaiters, finish)
		if !item.formSending {
			if item.FormChange == "" {
				settleCardForm(item, false)
			} else {
				h.flushCardForm(item)
			}
		}
	}
}
func (h *Host) mailMutate(item *card, operation, field, key string) {
	if item.busy || !mailCanEdit(item) {
		return
	}
	generation := item.formGeneration
	item.busy = true
	h.captureMailEditors(item, func(ok bool) {
		if h.closed || generation != item.formGeneration {
			return
		}
		if !ok {
			item.busy = false
			item.err = mailText("syncFailed")
			return
		}
		base := maps.Clone(item.values)
		h.call("mail-ui", map[string]any{"op": operation, "values": base, "field": field, "key": key}, func(raw json.RawMessage, err error) {
			if h.closed || generation != item.formGeneration {
				return
			}
			item.busy = false
			var result struct {
				Values map[string]string `json:"values"`
			}
			if err != nil {
				item.err = err.Error()
				return
			}
			if json.Unmarshal(raw, &result) != nil || len(result.Values) == 0 || !validCardValues(result.Values) {
				item.err = mailText("editorUnavailable")
				return
			}
			for name, value := range result.Values {
				if item.values[name] == base[name] {
					item.values[name] = value
				}
			}
			if !reflect.DeepEqual(base, item.values) {
				item.formDirty = true
				h.flushCardForm(item)
			}
		})
	})
}
func (h *Host) enlargeMail(item *card) {
	h.captureMailEditors(item, func(ok bool) {
		if !ok || h.closed {
			return
		}
		h.waitMailDeliveries(item.SessionID, func(ok bool) {
			if ok && !h.closed {
				h.cards.mail.enlarged = item.CallID
				h.cards.mail.enlargedErr = ""
				h.cards.mail.enlargedSnapshot = item.Mail != nil && item.Mail.SupersededBy != ""
			}
		})
	})
}

func (h *Host) openMailImage(source string) {
	s := h.cards.mail
	if s.preview != nil {
		s.preview.Close()
	}
	s.previewSource = source
	s.preview = workspace.NewDocumentPane(h.env, workspace.DocumentOptions{Path: source, Preview: func() (workspace.FilePreview, error) {
		var preview workspace.FilePreview
		err := h.env.Host.CallInto("chat.ghosts.media-preview", map[string]string{"url": source}, &preview)
		return preview, err
	}})
}

// MailView and MailClose are the root lifecycle seams. Hidden/unmounted mail
// documents lose their native contexts; provider saves/receipts remain owned by
// the backend. Closing a preview is not cancelling a send or deleting a draft.
func (h *Host) MailView(c *ui.Context) {
	s := h.cards.mail
	if h.closed || s.closed {
		return
	}
	if s.enlarged != "" {
		if item := h.cards.byCall[s.enlarged]; item != nil && item.Mail != nil && item.Mail.SupersededBy != "" && !s.enlargedSnapshot && !s.resolving && s.enlargedErr == "" {
			previous := item.CallID
			s.resolving = true
			h.resolveMailCard(item.SessionID, previous, item.Mail.AccountID, item.Mail.DraftID, item.GhostID, func(next *card, err error) {
				s.resolving = false
				if h.closed || s.enlarged != previous {
					return
				}
				if err != nil {
					s.enlargedErr = err.Error()
				} else {
					s.enlarged = next.CallID
				}
			})
		}
		open := true
		kit.Dialog(c.Key("mail-editor-expanded"), &open, kit.DialogOptions{Title: mailText("preview"), Width: 960, Cancel: i18n.T("common.close"), Error: s.enlargedErr, Actions: func() {
			if s.enlargedErr != "" && kit.Button(c, text("retry"), nil, false).Clicked() {
				s.enlargedErr = ""
			}
		}, Body: func() {
			item := h.cards.byCall[s.enlarged]
			if item == nil {
				kit.InlineError(c, mailText("unavailable"))
			} else {
				h.mailCard(c, item, true)
			}
		}})
		if !open {
			s.enlarged = ""
			s.enlargedSnapshot = false
			s.enlargedErr = ""
		}
	}
	h.mailReaderDialog(c)
	if s.preview != nil {
		open := true
		kit.Dialog(c.Key("mail-image-preview"), &open, kit.DialogOptions{Title: mailText("preview"), Width: 920, Cancel: i18n.T("common.close"), Body: func() { ui.Column(c).FillWidth().Height(520).MinWidth(0).Children(func() { s.preview.View(c) }) }})
		if !open {
			s.preview.Close()
			s.preview = nil
			s.previewSource = ""
		}
	}
	for id, document := range s.documents {
		if !document.visible {
			document.Close()
			delete(s.documents, id)
		} else {
			document.visible = false
		}
	}
}
func (h *Host) MailClose() {
	s := h.cards.mail
	if s.closed {
		return
	}
	s.closed = true
	for _, document := range s.documents {
		document.Close()
	}
	clear(s.documents)
	clear(s.models)
	if s.preview != nil {
		s.preview.Close()
		s.preview = nil
	}
	if s.reader != nil {
		s.reader.closed = true
		s.reader = nil
	}
	for _, item := range h.cards.byCall {
		settleCardForm(item, false)
	}
}

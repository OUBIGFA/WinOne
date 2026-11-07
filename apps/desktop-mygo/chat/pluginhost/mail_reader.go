package pluginhost

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// MailMessage is the original Reader's public, read-only data. Bodies and
// provider identities never come from a link, URL host or a persisted UI tab.
type mailMessage struct {
	AccountID               string                 `json:"accountId"`
	MessageID               string                 `json:"messageId"`
	From                    string                 `json:"from"`
	To                      string                 `json:"to"`
	Cc                      string                 `json:"cc"`
	Bcc                     string                 `json:"bcc"`
	Subject                 string                 `json:"subject"`
	Date                    string                 `json:"date"`
	BodyText                string                 `json:"bodyText"`
	BodyHTML                string                 `json:"bodyHtml"`
	Attachments             []mailAttachment       `json:"attachments"`
	ParticipantPhotos       []mailParticipantPhoto `json:"participantPhotos"`
	Conversation            *mailConversation      `json:"conversation"`
	ConversationUnavailable bool                   `json:"conversationUnavailable"`
	MarkReadFailed          bool                   `json:"markReadFailed"`
}
type mailParticipantPhoto struct {
	Email string `json:"email"`
	URL   string `json:"url"`
}
type mailMessageSummary struct {
	MessageID string `json:"messageId"`
	URL       string `json:"url"`
	From      string `json:"from"`
	Date      string `json:"date"`
	Subject   string `json:"subject"`
	Snippet   string `json:"snippet"`
}
type mailConversation struct {
	ThreadID string               `json:"threadId"`
	Messages []mailMessageSummary `json:"messages"`
}
type mailReaderChoice struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Namespace *string `json:"namespace"`
}
type mailPreviewResult struct {
	Supported   bool               `json:"supported"`
	Message     *mailMessage       `json:"message"`
	DraftCallID string             `json:"draftCallId"`
	Error       string             `json:"error"`
	ProviderID  string             `json:"providerId"`
	Providers   []mailReaderChoice `json:"providers"`
}
type mailReaderRow struct {
	summary                    mailMessageSummary
	message                    *mailMessage
	card                       *card
	expanded, visible, loading bool
	openAfterPending           bool
	err                        string
}
type mailReaderView struct {
	session, address, provider string
	result                     mailPreviewResult
	rows                       []*mailReaderRow
	loading, closed            bool
	err                        string
}

func safeMailLink(address string) string {
	target, err := url.Parse(address)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" || target.User != nil {
		return ""
	}
	return target.String()
}

// SetMailBrowserOpener binds reading links/open-in-mailbox to the window's
// existing native browser route. It is a presentation callback, not a plugin
// capability or a way to mint native gesture tickets.
func (h *Host) SetMailBrowserOpener(open func(session, address string)) {
	h.cards.mail.browserOpener = open
}

// OpenMailIfSupported is the normal link-navigation seam. Background probing
// never marks mail read. Only an explicit native opening calls OpenMail below.
func (h *Host) OpenMailIfSupported(session, address string, done func(bool)) {
	if h.closed || session == "" || safeMailLink(address) == "" {
		done(false)
		return
	}
	h.call("mail-preview", map[string]any{"sessionId": session, "url": address, "read": false}, func(raw json.RawMessage, err error) {
		if h.closed {
			return
		}
		var result mailPreviewResult
		if err != nil || json.Unmarshal(raw, &result) != nil {
			done(false)
			return
		}
		if !result.Supported {
			done(false)
			return
		}
		h.OpenMail(session, address)
		done(true)
	})
}
func (h *Host) OpenMail(session, address string) {
	if h.closed || session == "" || safeMailLink(address) == "" {
		return
	}
	s := h.cards.mail
	if s.reader != nil {
		s.reader.closed = true
	}
	reader := &mailReaderView{session: session, address: address, loading: true}
	s.reader = reader
	h.captureMailSession(session, func(ok bool) {
		if h.closed || reader.closed || s.reader != reader {
			return
		}
		if !ok {
			reader.loading = false
			reader.err = mailText("syncFailed")
			return
		}
		s.enlarged = ""
		s.enlargedSnapshot = false
		s.enlargedErr = ""
		h.readMail(reader, "", true)
	})
}

// FlushMailEdits is the normal Chat-send barrier. It covers browser editor
// events still crossing the native boundary and every same-task card delivery;
// the original provider SDK subsequently flushes its own autosave before tools.
func (h *Host) FlushMailEdits(session string, done func(bool)) { h.captureMailSession(session, done) }

func (h *Host) captureMailSession(session string, done func(bool)) {
	var items []*card
	for _, item := range h.cards.byCall {
		if item.SessionID == session && mailCanEdit(item) {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		h.waitMailDeliveries(session, done)
		return
	}
	remaining, ok := len(items), true
	for _, item := range items {
		h.captureMailEditors(item, func(success bool) {
			ok = ok && success
			remaining--
			if remaining == 0 {
				if !ok {
					done(false)
				} else {
					h.waitMailDeliveries(session, done)
				}
			}
		})
	}
}
func (h *Host) readMail(reader *mailReaderView, provider string, opened bool) {
	reader.loading = true
	reader.err = ""
	params := map[string]any{"sessionId": reader.session, "url": reader.address, "read": true, "opened": opened}
	if provider != "" {
		params["providerId"] = provider
	}
	h.call("mail-preview", params, func(raw json.RawMessage, err error) {
		if h.closed || reader.closed || h.cards.mail.reader != reader {
			return
		}
		reader.loading = false
		if err != nil {
			reader.err = err.Error()
			return
		}
		var result mailPreviewResult
		if json.Unmarshal(raw, &result) != nil || !result.Supported {
			reader.err = mailText("failed")
			return
		}
		if provider != "" && result.Message != nil && result.ProviderID != provider {
			reader.err = mailText("unavailable")
			return
		}
		reader.result = result
		reader.provider = result.ProviderID
		if result.Message == nil {
			reader.err = first(result.Error, mailText("failed"))
			return
		}
		if result.DraftCallID != "" {
			h.resolveMailCard(reader.session, result.DraftCallID, result.Message.AccountID, "", "", func(item *card, err error) {
				if h.closed || reader.closed || h.cards.mail.reader != reader {
					return
				}
				if err != nil {
					reader.err = err.Error()
					return
				}
				reader.closed = true
				h.cards.mail.reader = nil
				h.cards.mail.enlarged = item.CallID
				h.cards.mail.enlargedSnapshot = false
				h.cards.mail.enlargedErr = ""
			})
			return
		}
		h.mailReaderRows(reader)
	})
}
func (h *Host) resolveMailCard(session, callID, account, draft, ghost string, done func(*card, error)) {
	visited := map[string]bool{}
	var read func(string)
	read = func(id string) {
		if id == "" || visited[id] {
			done(nil, fmt.Errorf("%s", mailText("unavailable")))
			return
		}
		visited[id] = true
		h.call("card-get", map[string]string{"callId": id}, func(raw json.RawMessage, err error) {
			if h.closed {
				return
			}
			if err != nil {
				done(nil, err)
				return
			}
			var next card
			if json.Unmarshal(raw, &next) != nil || next.CallID != id || next.SessionID != session || next.Mail == nil || (account != "" && next.Mail.AccountID != account) || (draft != "" && next.Mail.DraftID != draft) || (ghost != "" && next.GhostID != ghost) {
				done(nil, fmt.Errorf("%s", mailText("unavailable")))
				return
			}
			account, draft, ghost = next.Mail.AccountID, next.Mail.DraftID, next.GhostID
			h.cards.update(raw)
			if next.Mail.SupersededBy != "" {
				read(next.Mail.SupersededBy)
				return
			}
			done(h.cards.byCall[id], nil)
		})
	}
	read(callID)
}

func (h *Host) mailReaderRows(reader *mailReaderView) {
	mail := reader.result.Message
	summaries := []mailMessageSummary{{MessageID: mail.MessageID, URL: reader.address, From: mail.From, Date: mail.Date, Subject: mail.Subject, Snippet: mail.BodyText}}
	if mail.Conversation != nil {
		summaries = mail.Conversation.Messages
	}
	selected := 0
	for i, summary := range summaries {
		if summary.MessageID == mail.MessageID {
			selected = i
			break
		}
	}
	reader.rows = nil
	for i, summary := range summaries {
		near := i >= selected-2 && i <= selected+2
		row := &mailReaderRow{summary: summary, expanded: len(summaries) <= 6 || near, visible: len(summaries) <= 6 || i == 0 || i >= len(summaries)-2 || near}
		if summary.MessageID == mail.MessageID {
			row.message = mail
			row.card = mailMessageCard(reader.session, mail)
		}
		reader.rows = append(reader.rows, row)
	}
}
func mailMessageCard(session string, message *mailMessage) *card {
	fields := []cardFormField{{Name: "to", Label: mailText("to"), Value: message.To, Kind: "recipients"}, {Name: "cc", Label: mailText("cc"), Value: message.Cc, Kind: "recipients"}, {Name: "bcc", Label: mailText("bcc"), Value: message.Bcc, Kind: "recipients"}}
	if message.BodyHTML != "" {
		fields = append(fields, cardFormField{Name: "bodyHtml", Value: message.BodyHTML, Kind: "mail-html"})
	} else {
		fields = append(fields, cardFormField{Name: "body", Value: message.BodyText, Multiline: true})
	}
	attachments, _ := json.Marshal(message.Attachments)
	fields = append(fields, cardFormField{Name: "attachmentItems", Value: string(attachments), Kind: "mail-attachments"})
	item := &card{CallID: "mail-reader:" + session + ":" + message.AccountID + ":" + message.MessageID, SessionID: session, Fields: fields, readerMail: true, Mail: &mailCardData{AccountID: message.AccountID, State: "sent", Fields: fields}, values: map[string]string{}, baseValues: map[string]string{}}
	for _, field := range fields {
		item.values[field.Name] = field.Value
		item.baseValues[field.Name] = field.Value
	}
	return item
}
func (h *Host) readMailRow(reader *mailReaderView, row *mailReaderRow, opened bool) {
	if row.loading {
		if opened {
			row.openAfterPending = true
		}
		return
	}
	if row.summary.URL == "" {
		return
	}
	row.loading = true
	row.err = ""
	params := map[string]any{"sessionId": reader.session, "url": row.summary.URL, "read": true, "opened": opened, "providerId": reader.provider}
	h.call("mail-preview", params, func(raw json.RawMessage, err error) {
		if h.closed || reader.closed || h.cards.mail.reader != reader {
			return
		}
		row.loading = false
		defer func() {
			if row.openAfterPending {
				row.openAfterPending = false
				if row.expanded {
					h.readMailRow(reader, row, true)
				}
			}
		}()
		if err != nil {
			row.err = err.Error()
			return
		}
		var result mailPreviewResult
		if json.Unmarshal(raw, &result) != nil || result.Message == nil || result.ProviderID != reader.provider || result.Message.AccountID != reader.result.Message.AccountID || result.Message.MessageID != row.summary.MessageID {
			row.err = mailText("failed")
			return
		}
		row.message = result.Message
		row.card = mailMessageCard(reader.session, result.Message)
	})
}
func (h *Host) mailReaderDialog(c *ui.Context) {
	reader := h.cards.mail.reader
	if reader == nil || reader.closed {
		return
	}
	open := true
	title := mailText("preview")
	if reader.result.Message != nil {
		title = first(reader.result.Message.Subject, title)
	}
	kit.Dialog(c.Key("mail-reader"), &open, kit.DialogOptions{Title: title, Width: 960, Cancel: i18n.T("common.close"), Body: func() {
		if reader.loading {
			kit.Loader(c)
		}
		kit.InlineError(c, reader.err)
		for _, choice := range reader.result.Providers {
			if kit.Button(c, choice.Name, nil, false).Disabled(reader.loading).Clicked() {
				h.readMail(reader, choice.ID, true)
			}
		}
		if reader.err != "" && kit.Button(c, text("retry"), nil, false).Disabled(reader.loading).Clicked() {
			h.readMail(reader, reader.provider, true)
		}
		mail := reader.result.Message
		if mail == nil {
			return
		}
		if mail.ConversationUnavailable {
			kit.Notice(c, kit.Info, "", mailText("conversationFailed"), nil)
			if kit.Button(c, text("retry"), nil, false).Clicked() {
				h.readMail(reader, reader.provider, false)
			}
		}
		ui.Column(c).FillWidth().MinWidth(0).Gap(8).Children(func() {
			for i, row := range reader.rows {
				if !row.visible {
					if i == 0 || reader.rows[i-1].visible {
						if kit.Button(c, mailText("showMessages"), nil, false).Clicked() {
							for _, hidden := range reader.rows {
								hidden.visible = true
							}
						}
					}
					continue
				}
				h.mailReaderRow(c, reader, row)
			}
		})
	}, Actions: func() {
		if kit.Button(c, mailText("openMailbox"), nil, false).Clicked() {
			h.openMailDocumentLink(reader.session, reader.address)
		}
	}})
	if !open {
		reader.closed = true
		h.cards.mail.reader = nil
	}
}
func (h *Host) mailReaderRow(c *ui.Context, reader *mailReaderView, row *mailReaderRow) {
	previous := row.expanded
	startedOpened := false
	date := row.summary.Date
	if parsed, err := time.Parse(time.RFC3339, date); err == nil {
		date = parsed.Local().Format("2006-01-02 15:04")
	}
	label := row.summary.From + " · " + date
	if row.summary.MessageID == reader.result.Message.MessageID {
		label += " · " + mailText("currentMessage")
	}
	ui.Collapsible(c.Key(row.summary.MessageID), label, &row.expanded, func() {
		if row.message == nil && !row.loading && row.err == "" {
			startedOpened = !previous
			h.readMailRow(reader, row, startedOpened)
		}
		if row.loading {
			kit.Loader(c)
		}
		kit.InlineError(c, row.err)
		if row.err != "" && kit.Button(c, text("retry"), nil, false).Clicked() {
			h.readMailRow(reader, row, false)
		}
		if row.message == nil {
			return
		}
		if row.message.MarkReadFailed {
			kit.Notice(c, kit.Info, "", mailText("markReadFailed"), nil)
		}
		h.mailFields(c, row.card, true)
	})
	if !previous && row.expanded && !startedOpened {
		h.readMailRow(reader, row, true)
	}
	if !row.expanded {
		ui.Text(c, row.summary.Snippet).TextColor(c.Theme().TextMuted).MinWidth(0)
	}
}

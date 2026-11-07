package onebot

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"winone/desktopmygo/chat/mathview"
	"winone/desktopmygo/pages/chat"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/chat/workspace"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/webui"
	"winone/desktopmygo/widgets"
)

type GroupMember struct{ BotID, Name, Avatar, Status string }
type GroupSpeaker struct{ BotID, SessionID, Activity string }
type GroupActivity struct {
	Phase string `json:"phase"`
}
type GroupRound struct {
	Status      string
	Speakers    []GroupSpeaker
	CanContinue bool
}
type GroupPlanStep struct {
	Position                                      int
	BotID, BotName, Task, Status, ResultMessageID string
}
type GroupPlan struct {
	ID, Status, OrganizerBotID, OrganizerName, WorkDir, Branch string
	Steps                                                      []GroupPlanStep
	CurrentStep                                                *int
}
type GroupSummary struct {
	ID, Name, ReplyMode, SpeakingMode string
	Members                           []GroupMember
	OrganizerBotID                    *string
	ProjectDir                        *string
	Round                             GroupRound
	Revision                          int
	ServerBacked                      bool
	AllowMentionAllBots               bool
	TeammateActivity                  *GroupActivity `json:"teammateActivity,omitempty"`
}
type GroupQuote struct {
	MessageID, AuthorName, Excerpt string
	OriginalDeleted                bool
}
type GroupAttachment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	URL      string `json:"url,omitempty"`
	Category string `json:"category"`
	MIME     string `json:"mimeType"`
	Size     int64  `json:"size"`
	State    string `json:"state,omitempty"`
}
type GroupTask struct {
	State, Title, Summary string
	Meta                  struct{ DelegationID, ParentSessionID, ChildSessionID string }
}
type GroupMessage struct {
	ID, Kind, AuthorKind, AuthorBotID, AuthorName, Content, NoticeCode, RuntimeFailureCode, CapabilityNoticeCode, PlanID string
	Sequence                                                                                                             int
	CreatedAt                                                                                                            int64
	Attachments                                                                                                          []GroupAttachment
	Files                                                                                                                []string
	Quote                                                                                                                *GroupQuote
	Task                                                                                                                 *GroupTask
}
type GroupDetail struct {
	GroupSummary
	Messages        []GroupMessage
	MessageUpdates  []GroupMessage
	Plans           []GroupPlan
	HasMoreBefore   bool
	HistoryRevision string
}
type groupDraft struct {
	Text        string            `json:"text"`
	Attachments []GroupAttachment `json:"attachments"`
	Mentioned   []string          `json:"mentioned"`
	All         bool              `json:"all"`
	ClientID    string            `json:"clientId,omitempty"`
	// Submission fingerprints the complete send intent, including recipients,
	// quote and division. A changed intent must not borrow a lost receipt.
	Submission string `json:"submission,omitempty"`
}
type groupEditor struct {
	groupID, name, replyMode, speakingMode, organizer, directory string
	members                                                      []string
	baseline                                                     []byte
	busy                                                         bool
	error                                                        string
}
type groupPane struct {
	parent *page
	// prompts follow speakers waiting on a permission or question.
	prompts                      map[string]*chat.PromptPanel
	id                           string
	data                         GroupDetail
	draft                        groupDraft
	loading, busy, closed, again bool
	error                        string
	generation                   uint64
	readEarlier                  bool
	scroll                       ui.ScrollState
	follow                       bool
	lastY                        float32
	settings                     *groupEditor
	deleteOpen                   bool
	planEdit                     *struct {
		planID        string
		position      int
		botID, action string
	}
	preview     *workspace.DocumentPane
	previewOpen bool
	quote       *GroupMessage
	failed      *groupDraft
}

func (p *page) openGroup(id string) {
	p.env.RequestChange(func() {
		if p.settings != nil {
			p.settings.Close()
			p.settings = nil
		}
		p.closeContent()
		p.section = "group"
		p.group = newGroupPane(p, id)
	})
}
func newGroupPane(parent *page, id string) *groupPane {
	p := &groupPane{parent: parent, id: id, follow: true}
	p.loadDraft()
	p.Refresh(false)
	return p
}
func (p *groupPane) Dirty() bool { return p.settings != nil && p.settings.Dirty() }
func (p *groupPane) Busy() bool  { return p.busy || p.settings != nil && p.settings.busy }
func (p *groupPane) Discard() {
	p.settings = nil
	p.parent.env.Activity.Report("onebot-group-settings", false, false, nil)
}
func (p *groupPane) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.generation++
	for _, panel := range p.prompts {
		panel.Close()
	}
	p.saveDraft()
	p.parent.env.Activity.Report("onebot-group", false, false, nil)
	p.parent.env.Activity.Report("onebot-group-settings", false, false, nil)
	if p.preview != nil {
		p.preview.Close()
	}
}
func (p *groupPane) Refresh(earlier bool) {
	if p.closed {
		return
	}
	if p.loading {
		p.again = true
		return
	}
	p.loading = true
	p.generation++
	generation := p.generation
	options := map[string]any{"limit": 100}
	if earlier && len(p.data.Messages) > 0 {
		options["beforeSequence"] = p.data.Messages[0].Sequence
	}
	oneBotCall(p.parent, "groups.get", map[string]any{"groupId": p.id, "options": options}, func(value struct {
		Group GroupDetail `json:"group"`
	}, err error) {
		if p.closed || generation != p.generation {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		old := p.data.Messages
		oldRevision := p.data.HistoryRevision
		p.data = value.Group
		if oldRevision != "" && oldRevision != value.Group.HistoryRevision {
			old = nil
		}
		if len(old) > 0 {
			byID := map[string]GroupMessage{}
			for _, message := range old {
				byID[message.ID] = message
			}
			for _, message := range p.data.Messages {
				byID[message.ID] = message
			}
			for _, message := range value.Group.MessageUpdates {
				byID[message.ID] = message
			}
			p.data.Messages = nil
			for _, message := range byID {
				p.data.Messages = append(p.data.Messages, message)
			}
			sort.Slice(p.data.Messages, func(i, j int) bool { return p.data.Messages[i].Sequence < p.data.Messages[j].Sequence })
		}
		p.error = ""
		if earlier {
			p.follow = false
		}
		if p.again {
			p.again = false
			p.Refresh(false)
		}
	})
}
func (p *groupPane) action(method string, input map[string]any, done func()) {
	if p.busy || p.closed {
		return
	}
	p.busy = true
	oneBotCall(p.parent, "groups."+method, input, func(_ json.RawMessage, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.error = ""
		if done != nil {
			done()
		}
		p.Refresh(false)
	})
}
func (p *groupPane) send(division bool) {
	if p.busy || strings.TrimSpace(p.draft.Text) == "" && len(p.draft.Attachments) == 0 {
		return
	}
	capture := copyValue(p.draft)
	quote := p.quote
	quoteID := ""
	if quote != nil {
		quoteID = quote.ID
	}
	mentions := append([]string(nil), capture.Mentioned...)
	all := capture.All
	for _, member := range p.data.Members {
		if strings.Contains(capture.Text, "@"+member.Name) && !slices.Contains(mentions, member.BotID) {
			mentions = append(mentions, member.BotID)
		}
	}
	if strings.Contains(strings.ToLower(capture.Text), "@all") {
		all = true
	}
	identity, _ := json.Marshal([]any{capture.Text, capture.Attachments, division, mentions, all, quoteID})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(identity))
	if p.failed != nil && p.failed.Submission == fingerprint {
		capture.ClientID = p.failed.ClientID
	} else if capture.ClientID == "" || capture.Submission != "" && capture.Submission != fingerprint {
		capture.ClientID = rand.Text()
	}
	capture.Submission = fingerprint
	input := map[string]any{"groupId": p.id, "text": capture.Text, "clientId": capture.ClientID, "division": division, "mentions": map[string]any{"all": all, "botIds": mentions}, "attachments": capture.Attachments}
	if quote != nil {
		input["quote"] = map[string]string{"messageId": quote.ID}
	}
	p.busy = true
	p.draft = groupDraft{}
	p.saveDraft()
	oneBotCall(p.parent, "groups.send", map[string]any{"input": input}, func(_ struct {
		MessageID string `json:"messageId"`
	}, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			p.failed = &capture
			newer := p.draft
			p.draft = capture
			if newer.Text != "" || len(newer.Attachments) > 0 {
				// The composer remains editable while the real receipt is pending.
				// A failure restores the submitted draft before newer text/files,
				// rather than keeping only the newer draft and losing the failed one.
				if newer.Text != "" {
					p.draft.Text += "\n\n" + newer.Text
				}
				seen := map[string]bool{}
				for _, file := range p.draft.Attachments {
					seen[file.Path] = true
				}
				for _, file := range newer.Attachments {
					if !seen[file.Path] {
						p.draft.Attachments = append(p.draft.Attachments, file)
						seen[file.Path] = true
					}
				}
				for _, id := range newer.Mentioned {
					if !slices.Contains(p.draft.Mentioned, id) {
						p.draft.Mentioned = append(p.draft.Mentioned, id)
					}
				}
				p.draft.All = p.draft.All || newer.All
				p.draft.ClientID, p.draft.Submission = "", ""
			}
			p.saveDraft()
			return
		}
		p.failed = nil
		if p.quote == quote {
			p.quote = nil
		}
		p.error = ""
		p.follow = true
		p.Refresh(false)
	})
}
func (p *groupPane) stagePaths(paths []string) {
	if p.busy || len(paths) == 0 {
		return
	}
	p.busy = true
	available := 20 - len(p.draft.Attachments)
	paths = append([]string(nil), paths...)
	core.Go(p.parent.env, func() ([]GroupAttachment, error) {
		var files []GroupAttachment
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil {
				return files, err
			}
			if info.IsDir() {
				continue
			}
			if len(files) >= available {
				return files, fmt.Errorf("%s", nt("groupAttachmentLimit"))
			}
			file, err := os.Open(path)
			if err != nil {
				return files, err
			}
			var upload struct {
				Token string `json:"token"`
			}
			if err = p.parent.call("groups.files.begin", map[string]any{"groupId": p.id, "name": filepath.Base(path), "size": info.Size()}, &upload); err != nil {
				file.Close()
				return files, err
			}
			buffer := make([]byte, 256<<10)
			offset := int64(0)
			for {
				n, readErr := file.Read(buffer)
				if n > 0 {
					if err = p.parent.call("groups.files.chunk", map[string]any{"token": upload.Token, "offset": offset, "base64": base64.StdEncoding.EncodeToString(buffer[:n])}, nil); err != nil {
						file.Close()
						return files, err
					}
					offset += int64(n)
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					file.Close()
					return files, readErr
				}
			}
			if err = file.Close(); err != nil {
				return files, err
			}
			var saved struct {
				Path, Name, Kind, URL string
				MIME                  string `json:"mimeType"`
				Size                  int64
			}
			image := slices.Contains([]string{".png", ".jpg", ".jpeg", ".gif", ".webp"}, strings.ToLower(filepath.Ext(path)))
			if err = p.parent.call("groups.files.finish", map[string]any{"token": upload.Token, "image": image}, &saved); err != nil {
				return files, err
			}
			category := "file"
			mimeType := saved.MIME
			switch strings.ToLower(filepath.Ext(path)) {
			case ".png", ".jpg", ".jpeg", ".gif", ".webp":
				category = "image"
			case ".pdf":
				category, mimeType = "pdf", "application/pdf"
			case ".txt", ".md", ".json", ".go", ".ts":
				category, mimeType = "text", "text/plain"
			case ".docx", ".xlsx", ".pptx":
				category = "office"
			}
			files = append(files, GroupAttachment{ID: rand.Text(), Path: saved.Path, Name: saved.Name, Category: category, MIME: mimeType, URL: saved.URL, Size: saved.Size})
		}
		return files, nil
	}, func(files []GroupAttachment, err error) {
		if p.closed {
			return
		}
		p.busy = false
		p.draft.Attachments = append(p.draft.Attachments, files...)
		if err != nil {
			p.error = err.Error()
		}
		for _, path := range paths {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				p.draft.Text = strings.TrimSpace(p.draft.Text + "\n@" + path)
			}
		}
		p.saveDraft()
	})
}
func (p *groupPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	p.parent.env.Activity.Report("onebot-group", p.Dirty(), p.Busy(), p.Discard)
	committed := max(0, min(p.scroll.Y, p.scroll.MaxY))
	if committed < min(p.lastY, p.scroll.MaxY)-.01 {
		p.follow = false
	} else if committed > p.lastY && p.scroll.MaxY-committed <= 80 {
		p.follow = true
	}
	ui.Column(c).Fill().MinWidth(0).Gap(8).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Padding(10).AlignItems(ui.Center).Children(func() {
			kit.Strong(c, p.data.Name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if kit.IconButton(c, icons.Settings2, nt("groupSettings")).Clicked() {
				p.settings = groupSettings(p.data.GroupSummary)
			}
			if kit.IconButton(c, icons.RefreshCw, nt("refresh")).Clicked() {
				p.Refresh(false)
			}
		})
		kit.InlineError(c, p.error)
		if p.loading && len(p.data.Messages) == 0 {
			kit.Loader(c)
		}
		ui.Scroll(c).TrackScroll(&p.scroll).HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputScroll && ev.DY < 0 {
				p.follow = false
			}
			return false
		}).FillWidth().Grow(1).MinHeight(0).Padding(16).Gap(12).Children(func() {
			if p.data.HasMoreBefore && kit.Button(c, nt("loadEarlier"), icons.ChevronsUp, false).Disabled(p.loading).Clicked() {
				p.Refresh(true)
			}
			for _, message := range p.data.Messages {
				p.messageView(c, message)
			}
		})
		if p.follow && !p.scroll.Scrolling() {
			p.scroll.Y = 1e20
		}
		p.lastY = committed
		for _, speaker := range p.data.Round.Speakers {
			panel := p.prompts[speaker.SessionID]
			pending := panel != nil && panel.Pending()
			ui.Row(c.Key(speaker.BotID)).FillWidth().Gap(8).Padding(0, 12).Children(func() {
				name := speaker.BotID
				if bot := p.parent.bot(speaker.BotID); bot != nil {
					name = bot.Name
				}
				if pending {
					kit.Status(c, bt("bots.groupChat.waitingConfirm", "name", name))
				} else {
					kit.Status(c, nt("groupSpeaking", "name", name))
				}
				if speaker.SessionID != "" && kit.Button(c, nt("openTask"), icons.ArrowUpRight, false).Clicked() {
					p.parent.openTask(speaker.SessionID)
				}
			})
			// The speaker's permission or question, answered here as Cindy's group does.
			if (p.groupNeedsInput() || pending) && speaker.SessionID != "" {
				p.waitingPrompt(c, speaker.BotID, speaker.SessionID)
			}
		}
		p.closeIdlePrompts()
		p.composer(c)
	})
	// The destructive decision replaces (rather than stacks over) the editor,
	// whose draft remains available when the user cancels or a delete fails.
	if p.settings != nil && !p.deleteOpen {
		p.groupSettingsView(c)
	}
	p.deleteView(c)
	p.planEditView(c)
	if p.previewOpen && p.preview != nil {
		open := true
		kit.Dialog(c, &open, kit.DialogOptions{Title: nt("filePreview"), Width: 900, Cancel: nt("close"), Body: func() { ui.Column(c).Height(520).FillWidth().Children(func() { p.preview.View(c) }) }})
		if !open {
			p.preview.Close()
			p.preview = nil
			p.previewOpen = false
		}
	}
}
func (p *groupPane) groupNeedsInput() bool {
	return p.data.TeammateActivity != nil && p.data.TeammateActivity.Phase == "needs-interaction"
}

func (p *groupPane) waitingPrompt(c *ui.Context, botID, sessionID string) {
	if p.prompts == nil {
		p.prompts = map[string]*chat.PromptPanel{}
	}
	panel := p.prompts[sessionID]
	if panel == nil {
		panel = chat.NewPromptPanel(p.parent.env, sessionID, p.parent.botBinding(botID, sessionID, false))
		p.prompts[sessionID] = panel
	}
	ui.Column(c.Key("prompt:"+sessionID)).FillWidth().Padding(0, 12).Children(func() { panel.View(c) })
}

// closeIdlePrompts releases the tasks no speaker is waiting in any more.
func (p *groupPane) closeIdlePrompts() {
	for id, panel := range p.prompts {
		waiting := false
		for _, speaker := range p.data.Round.Speakers {
			waiting = waiting || speaker.SessionID == id && (p.groupNeedsInput() || panel.Pending())
		}
		if !waiting {
			panel.Close()
			delete(p.prompts, id)
		}
	}
}

func (p *groupPane) messageView(c *ui.Context, m GroupMessage) {
	if m.Kind == "plan" {
		for _, plan := range p.data.Plans {
			if plan.ID == m.PlanID {
				p.planView(c, plan)
			}
		}
		return
	}
	ui.Column(c.Key(m.ID)).FillWidth().Gap(5).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			kit.Caption(c, m.AuthorName+" · "+time.UnixMilli(m.CreatedAt).Format("01-02 15:04"))
			ui.Spacer(c)
			if kit.IconButton(c, icons.Copy, nt("copy")).Clicked() {
				c.Services().WriteClipboard(m.Content)
			}
			if p.data.ServerBacked && m.AuthorKind != "system" && kit.IconButton(c, icons.Reply, nt("quote")).Clicked() {
				capture := m
				p.quote = &capture
			}
		})
		if m.AuthorKind == "system" {
			label := m.Content
			if m.NoticeCode != "" {
				label = bt("bots.groupChat.notices." + m.NoticeCode)
			}
			kit.Status(c, label)
		} else {
			options := markdown.Options{OnLink: p.openLink}
			markdown.View(c, m.Content, mathview.For(p.parent.env).Apply(&options)).FillWidth()
		}
		if m.RuntimeFailureCode != "" || m.CapabilityNoticeCode != "" {
			kit.InlineError(c, nt("groupExecutionError", "code", firstNonempty(m.RuntimeFailureCode, m.CapabilityNoticeCode)))
		}
		if m.Quote != nil {
			kit.Help(c, nt("quotedMessage", "name", m.Quote.AuthorName, "text", m.Quote.Excerpt))
		}
		for _, attachment := range m.Attachments {
			p.attachmentChip(c, attachment, false, -1)
		}
		for _, file := range m.Files {
			path := file
			for _, plan := range p.data.Plans {
				if plan.ID == m.PlanID && plan.WorkDir != "" {
					path = filepath.Join(plan.WorkDir, filepath.FromSlash(file))
				}
			}
			if kit.Button(c, file, icons.File, false).Clicked() {
				p.openDocument(path)
			}
		}
		if m.Task != nil {
			kit.Strong(c, m.Task.Title)
			kit.Help(c, m.Task.Summary)
			kit.Caption(c, nt("taskStatus."+m.Task.State))
			if m.Task.Meta.ChildSessionID != "" && kit.Button(c, nt("openTask"), icons.ArrowUpRight, false).Clicked() {
				p.parent.openTask(m.Task.Meta.ChildSessionID)
			}
		}
	})
}
func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func (p *groupPane) composer(c *ui.Context) {
	ui.Column(c).FillWidth().Padding(12).Gap(8).BorderWidth(1, 0, 0, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
		if p.quote != nil {
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				kit.Help(c, p.quote.AuthorName+": "+p.quote.Content).Grow(1)
				if kit.IconButton(c, icons.X, nt("removeQuote")).Clicked() {
					p.quote = nil
				}
			})
		}
		for i, attachment := range p.draft.Attachments {
			p.attachmentChip(c, attachment, true, i)
		}
		field := ui.TextArea(c, &p.draft.Text).FillWidth().Height(92).MinWidth(0).Label(nt("groupMessage")).Placeholder(nt("groupMessage"))
		if field.Changed() {
			p.draft.ClientID = ""
			p.saveDraft()
		}
		widgets.DropArea(c, p.parent.env, field, widgets.DropAny, true, p.busy, func(paths []string, err error) {
			if err != nil {
				p.error = err.Error()
			} else {
				p.stagePaths(paths)
			}
		})
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if kit.IconButton(c, icons.Paperclip, nt("addAttachments")).Disabled(p.busy).Clicked() {
				core.Go(p.parent.env, func() ([]string, error) { return p.parent.env.Dialogs.PickFiles(nt("addAttachments"), core.Filter{}) }, func(paths []string, err error) {
					if p.closed {
						return
					}
					if err != nil {
						p.error = err.Error()
					} else {
						p.stagePaths(paths)
					}
				})
			}
			kit.IconButton(c, icons.AtSign, nt("mentions")).Menu(func(m *ui.Menu) {
				for _, member := range p.data.Members {
					checked := slices.Contains(p.draft.Mentioned, member.BotID)
					label := member.Name
					if checked {
						label = "✓ " + label
					}
					if m.Item(label).Chosen() {
						if checked {
							p.draft.Mentioned = slices.DeleteFunc(p.draft.Mentioned, func(id string) bool { return id == member.BotID })
						} else {
							p.draft.Mentioned = append(p.draft.Mentioned, member.BotID)
						}
						p.saveDraft()
					}
				}
				if m.Item(nt("mentionAll")).Chosen() {
					p.draft.All = !p.draft.All
					p.saveDraft()
				}
			})
			if kit.Button(c, nt("division"), icons.ListTodo, false).Disabled(p.busy).Clicked() {
				p.send(true)
			}
			if p.data.Round.CanContinue && kit.Button(c, nt("continueRound"), icons.Play, false).Disabled(p.busy).Clicked() {
				p.action("continue", map[string]any{"groupId": p.id}, nil)
			}
			if p.data.Round.Status == "running" && kit.Button(c, nt("stop"), icons.Square, false).Disabled(p.busy).Clicked() {
				p.action("stop", map[string]any{"groupId": p.id}, nil)
			}
			ui.Spacer(c)
			if kit.PrimaryButton(c, nt("send"), icons.SendHorizontal, p.busy).Disabled(p.busy).Clicked() {
				p.send(false)
			}
		})
		if field.Shortcut(ui.Ctrl, ui.KeyEnter) {
			p.send(false)
		}
	})
}
func (p *groupPane) attachmentChip(c *ui.Context, a GroupAttachment, remove bool, index int) {
	ui.Row(c.Key(a.ID)).FillWidth().Gap(5).Children(func() {
		if kit.GhostButton(c, a.Name, icons.File, false).Grow(1).MinWidth(0).Justify(ui.Start).Tooltip(a.Name).Clicked() {
			p.openDocument(firstNonempty(a.Path, a.URL))
		}
		if a.State != "" {
			kit.Caption(c, nt("attachmentState."+a.State))
		}
		if remove && kit.IconButton(c, icons.X, nt("removeAttachment")).Clicked() {
			if index >= 0 && index < len(p.draft.Attachments) {
				p.draft.Attachments = slices.Delete(p.draft.Attachments, index, index+1)
				p.saveDraft()
			}
		}
	})
}
func (p *groupPane) openDocument(path string) {
	if p.preview != nil {
		p.preview.Close()
	}
	preview := func() (workspace.FilePreview, error) {
		var value workspace.FilePreview
		err := p.parent.call("groups.files.preview", map[string]string{"path": path}, &value)
		return value, err
	}
	p.preview = workspace.NewDocumentPane(p.parent.env, workspace.DocumentOptions{Path: path, Preview: preview, FileAction: func(method string) error {
		return p.parent.call("groups.files."+strings.TrimPrefix(method, "file."), map[string]string{"path": path}, nil)
	}, OnLink: p.openLink, Media: func(preview workspace.FilePreview) (workspace.DocumentBody, error) {
		return workspace.NewMediaPane(p.parent.env, preview)
	}, HTML: func(preview workspace.FilePreview) (workspace.DocumentBody, error) {
		return webui.NewHTMLPreview(p.parent.env, preview.URL, preview.Name)
	}})
	p.previewOpen = true
}
func (p *groupPane) openLink(href string) {
	if strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://") {
		p.parent.openExternal(href)
		return
	}
	directory := ""
	if p.data.ProjectDir != nil {
		directory = *p.data.ProjectDir
	}
	oneBotCall(p.parent, "groups.files.resolvePath", map[string]string{"href": href, "workingDir": directory}, func(value struct {
		Status     string
		Candidates []string
	}, err error) {
		if p.closed {
			return
		}
		if err != nil {
			p.error = err.Error()
			return
		}
		if value.Status == "unique" && len(value.Candidates) == 1 {
			p.openDocument(value.Candidates[0])
		} else {
			p.error = nt("fileLinkUnavailable")
		}
	})
}
func (p *groupPane) planView(c *ui.Context, plan GroupPlan) {
	ui.Column(c.Key(plan.ID)).FillWidth().Gap(6).Padding(10).Border(1, kit.Colors(c).Hairline).Children(func() {
		kit.Strong(c, nt("divisionPlan", "name", plan.OrganizerName))
		kit.Caption(c, nt("planStatus."+plan.Status))
		for _, step := range plan.Steps {
			ui.Row(c.Key(step.Position)).FillWidth().Gap(8).Wrap().Children(func() {
				kit.Help(c, fmt.Sprintf("%d. %s — %s", step.Position+1, step.BotName, step.Task)).Grow(1).MinWidth(0)
				kit.Caption(c, nt("planStep."+step.Status))
				if step.Status == "pending" && kit.IconButton(c, icons.Ellipsis, nt("editStep")).Clicked() {
					p.planEdit = &struct {
						planID        string
						position      int
						botID, action string
					}{plan.ID, step.Position, step.BotID, "reassign"}
				}
			})
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			for _, action := range []string{"start", "dismiss", "continue", "retry"} {
				eligible := plan.Status == "proposed" && (action == "start" || action == "dismiss") || plan.Status == "waiting" && (action == "continue" || action == "retry")
				if eligible && kit.Button(c, nt("planAction."+action), icons.Play, false).Disabled(p.busy).Clicked() {
					p.action("plan."+action, map[string]any{"input": map[string]any{"groupId": p.id, "planId": plan.ID}}, nil)
				}
			}
		})
	})
}
func (p *groupPane) planEditView(c *ui.Context) {
	d := p.planEdit
	if d == nil {
		return
	}
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{
		Title: nt("editStep"), Confirm: nt("save"), Cancel: nt("cancel"), Busy: p.busy, Error: p.error,
		Body: func() {
			options := []kit.SelectOption{}
			for _, member := range p.data.Members {
				options = append(options, kit.SelectOption{Value: member.BotID, Label: member.Name})
			}
			kit.Select(c, &d.botID, options)
			kit.Select(c, &d.action, []kit.SelectOption{{Value: "reassign", Label: nt("reassign")}, {Value: "remove", Label: nt("removeStep")}})
		},
	}) {
		p.action("plan.edit", map[string]any{"input": map[string]any{"groupId": p.id, "planId": d.planID, "position": d.position, "action": d.action, "botId": d.botID}}, func() { p.planEdit = nil })
	}
	if !open {
		p.planEdit = nil
	}
}
func (p *groupPane) draftPath() string {
	return filepath.Join(p.parent.env.Directory, "desktop-group-draft-"+p.id+".json")
}
func (p *groupPane) loadDraft() {
	if p.parent.env.Directory == "" {
		return
	}
	raw, err := os.ReadFile(p.draftPath())
	if err == nil && len(raw) < 128<<10 {
		_ = json.Unmarshal(raw, &p.draft)
	}
}
func (p *groupPane) saveDraft() {
	if p.parent.env.Directory == "" {
		return
	}
	raw, err := json.Marshal(p.draft)
	if err != nil {
		return
	}
	file, err := os.CreateTemp(p.parent.env.Directory, ".group-draft-*.tmp")
	if err != nil {
		p.error = err.Error()
		return
	}
	name := file.Name()
	defer os.Remove(name)
	_, err = file.Write(raw)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, p.draftPath())
	}
	if err != nil {
		p.error = err.Error()
	}
}

func (d *groupEditor) Dirty() bool {
	raw, _ := json.Marshal([]any{d.name, d.replyMode, d.speakingMode, d.organizer, d.directory, d.members})
	return string(raw) != string(d.baseline)
}
func groupSettings(group GroupSummary) *groupEditor {
	d := &groupEditor{groupID: group.ID, name: group.Name, replyMode: group.ReplyMode, speakingMode: group.SpeakingMode}
	if group.OrganizerBotID != nil {
		d.organizer = *group.OrganizerBotID
	}
	if group.ProjectDir != nil {
		d.directory = *group.ProjectDir
	}
	for _, member := range group.Members {
		d.members = append(d.members, member.BotID)
	}
	d.baseline, _ = json.Marshal([]any{d.name, d.replyMode, d.speakingMode, d.organizer, d.directory, d.members})
	return d
}
func (p *groupPane) groupSettingsView(c *ui.Context) {
	p.parent.groupEditorView(c, p.settings, func() { p.settings = nil; p.Refresh(false) })
}

// Deletion stays bound to this group until its real receipt succeeds. A
// failure retains its settings and message draft for inspection or retry.
func (p *groupPane) deleteView(c *ui.Context) {
	if !p.deleteOpen {
		return
	}
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: bt("bots.groupChat.settings.deleteConfirmTitle", "name", p.data.Name), Message: bt("bots.groupChat.settings.deleteNote"), Confirm: bt("bots.groupChat.settings.delete"), Cancel: nt("cancel"), Danger: true, Busy: p.busy, Error: p.error}) {
		p.action("delete", map[string]any{"groupId": p.id}, func() {
			p.deleteOpen = false
			p.settings = nil
			p.parent.env.Activity.Report("onebot-group-settings", false, false, nil)
			p.parent.navigate("groups")
		})
	}
	if !open {
		p.deleteOpen = false
	}
}

func (p *page) groupListView(c *ui.Context) {
	if kit.PrimaryButton(c, nt("createGroup"), icons.Plus, false).Clicked() {
		p.groupCreation = groupSettings(GroupSummary{ReplyMode: "all", SpeakingMode: "auto"})
	}
	for _, group := range p.groups {
		if kit.Button(c, group.Name, icons.MessagesSquare, false).FillWidth().Justify(ui.Start).Clicked() {
			p.openGroup(group.ID)
		}
	}
	if p.groupCreation != nil {
		p.groupEditorView(c, p.groupCreation, func() { p.groupCreation = nil; p.refresh() })
	}
}
func (p *page) groupEditorView(c *ui.Context, d *groupEditor, close func()) {
	p.env.Activity.Report("onebot-group-settings", d.Dirty(), d.busy, close)
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: nt("groupSettings"), Width: 720, Confirm: nt("save"), Cancel: nt("cancel"), Busy: d.busy, Error: d.error, ConfirmDisabled: len(d.members) < 2 || len(d.members) > 6, Actions: func() {
		if d.groupID != "" && p.group != nil && p.group.id == d.groupID && kit.Button(c, bt("bots.groupChat.settings.delete"), icons.Trash2, false).Disabled(d.busy).Clicked() {
			p.group.deleteOpen = true
		}
	}, Body: func() {
		ui.Field(c, nt("groupName"), func() { ui.TextInput(c, &d.name).FillWidth().Label(nt("groupName")) })
		for _, bot := range p.profiles {
			checked := slices.Contains(d.members, bot.ID)
			if kit.Toggle(c, &checked, bot.Name, bot.Description).Disabled(bot.Status != "active" && !checked).Changed() {
				if checked {
					d.members = append(d.members, bot.ID)
				} else {
					d.members = slices.DeleteFunc(d.members, func(id string) bool { return id == bot.ID })
				}
			}
		}
		if d.groupID != "" {
			kit.Select(c, &d.replyMode, []kit.SelectOption{{Value: "all", Label: nt("groupReplyAll")}, {Value: "mentioned", Label: nt("groupReplyMentioned")}})
			kit.Select(c, &d.speakingMode, []kit.SelectOption{{Value: "auto", Label: nt("groupSpeakAuto")}, {Value: "sequential", Label: nt("groupSpeakSequential")}})
			options := []kit.SelectOption{{Value: "", Label: nt("automaticOrganizer")}}
			for _, id := range d.members {
				label := id
				if bot := p.bot(id); bot != nil {
					label = bot.Name
				}
				options = append(options, kit.SelectOption{Value: id, Label: label})
			}
			kit.Select(c, &d.organizer, options)
			field := ui.TextInput(c, &d.directory).FillWidth().Label(nt("groupDirectory"))
			widgets.DropArea(c, p.env, field, widgets.DropFolders, false, d.busy, func(paths []string, err error) {
				if err != nil {
					d.error = err.Error()
				} else if len(paths) > 0 {
					d.directory = paths[0]
				}
			})
			if kit.Button(c, nt("chooseFolder"), icons.FolderOpen, false).Clicked() {
				core.Go(p.env, func() (string, error) { return p.env.Dialogs.PickFolder(nt("groupDirectory")) }, func(path string, err error) {
					if err != nil {
						d.error = err.Error()
					} else if path != "" {
						d.directory = path
					}
				})
			}
		}
	}}) {
		d.busy = true
		core.Go(p.env, func() (string, error) {
			if d.groupID == "" {
				var result struct {
					GroupID string `json:"groupId"`
				}
				err := p.call("groups.create", map[string]any{"input": map[string]any{"name": d.name, "botIds": d.members}}, &result)
				return result.GroupID, err
			}
			var organizer, directory any
			if d.organizer != "" {
				organizer = d.organizer
			}
			if d.directory != "" {
				directory = d.directory
			}
			if err := p.call("groups.update", map[string]any{"input": map[string]any{"groupId": d.groupID, "name": d.name, "replyMode": d.replyMode, "speakingMode": d.speakingMode, "organizerBotId": organizer, "projectDir": directory}}, nil); err != nil {
				return "", err
			}
			err := p.call("groups.setMembers", map[string]any{"input": map[string]any{"groupId": d.groupID, "botIds": d.members}}, nil)
			return d.groupID, err
		}, func(id string, err error) {
			if p.closed {
				return
			}
			d.busy = false
			if err != nil {
				d.error = err.Error()
				return
			}
			p.env.Activity.Report("onebot-group-settings", false, false, nil)
			close()
			if d.groupID == "" && id != "" {
				p.openGroup(id)
			}
		})
	}
	if !open {
		p.env.RequestChange(func() { p.env.Activity.Report("onebot-group-settings", false, false, nil); close() })
	}
}

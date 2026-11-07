package chat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"slices"
	"time"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// newID is a random request identity, as crypto.randomUUID was.
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func now() *float64 {
	ms := float64(time.Now().UnixMilli())
	return &ms
}

func paths(files []model.Attachment) []string {
	out := []string{}
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

// sameDraft reports whether the composer still holds exactly what was
// sent (Cindy isComposerSendSnapshotCurrent).
func sameDraft(text string, files []model.Attachment, currentText string, current []model.Attachment) bool {
	if text != currentText || len(files) != len(current) {
		return false
	}
	for i := range files {
		if files[i].Path != current[i].Path {
			return false
		}
	}
	return true
}

// mergeFiles joins attachments, one per path, the later winning.
func mergeFiles(lists ...[]model.Attachment) []model.Attachment {
	var out []model.Attachment
	at := map[string]int{}
	for _, list := range lists {
		for _, f := range list {
			if i, ok := at[f.Path]; ok {
				out[i] = f
				continue
			}
			at[f.Path] = len(out)
			out = append(out, f)
		}
	}
	return out
}

// send sends the composer's draft (or override) as a prompt. Cindy's
// ChatInput.dispatchSend freezes the input, clears it before dispatch,
// then lets the user type while send stays locked until the Host
// acknowledges; newer draft saves wait behind the receipt so they cannot
// overwrite what was typed meanwhile. On failure the text comes back.
func (p *Chat) send(override *queuedSend, mode model.DeliveryMode, done func(error)) {
	id := p.activeID
	if override == nil {
		// Preserve the explicit send snapshot while browser mail events are
		// crossing their barrier; newer Chat typing is not implicitly sent.
		override = &queuedSend{text: p.text, files: append([]model.Attachment(nil), p.files...)}
	}
	p.flushMailEdits(id, func(err error) {
		if err != nil {
			done(err)
			return
		}
		if p.closed() || p.activeID != id {
			done(core.Errorf(i18n.T("chat.sendCancelled")))
			return
		}
		p.sendAfterMail(override, mode, done)
	})
}

func (p *Chat) sendAfterMail(override *queuedSend, mode model.DeliveryMode, done func(error)) {
	if mode == "" {
		mode = model.DeliveryQueue
	} // Declarative answer buttons use the same queue semantics as the composer.
	selection := p.currentModel()
	if p.ai != nil && selection.Model == "" {
		done(core.Errorf(i18n.T("chat.modelUnavailable")))
		return
	}
	if conversation := p.conversation(); conversation != nil && conversation.Model != selection && !p.view.Running {
		// Configure the runtime as well as the pill, before clearing the draft
		// or opening the send receipt barrier (configure waits for draft writes).
		p.configure(&selection, "", func(err error) {
			if err != nil {
				done(err)
				return
			}
			p.send(override, mode, done)
		})
		return
	}
	p.newModel = selection
	original, files := p.text, p.files
	if override != nil {
		original, files = override.text, override.files
	}
	originalFiles := files
	content := model.AnnotatedPrompt(original, files)
	// A teammate's @mentions leave as Cindy's wire links; its pending quote leaves
	// with this message and is cleared once accepted.
	var references []botAgentReference
	var replyQuote *botReplyQuote
	shownText := original
	if p.bot != nil {
		if p.bot.Mentions != nil {
			mentions := p.bot.Mentions()
			content, references = serializeBotMentions(content, mentions)
			shownText, _ = serializeBotMentions(original, mentions)
		}
		replyQuote = p.botMode.quote
	}
	if p.view.Paused != nil && *p.view.Paused || isBlank(content) && len(files) == 0 {
		done(nil)
		return
	}
	queued := p.view.Running && mode == model.DeliveryQueue
	r := p.pendingInput
	if r == nil || r.text != content || r.deliveryMode != mode || !slices.Equal(r.attachments, paths(files)) {
		r = &receipt{id: newID(), text: content, attachments: paths(files), deliveryMode: mode}
	}
	generation := p.sendGeneration
	target := p.conversation()
	// The prompt waits for the draft saves queued before this send, not
	// for the receipt barrier it adds below.
	priorReady, onPrior := false, func() {}
	p.draftWrites.after(func() { priorReady = true; onPrior() })
	afterPrior := func(fn func()) {
		if priorReady {
			fn()
		} else {
			onPrior = fn
		}
	}
	cleared := false
	release := func() {}
	clearSubmitted := func() {
		if sameDraft(original, originalFiles, p.text, p.files) {
			p.draftRevision++
			p.text, p.files, p.draftDirty, cleared = "", nil, false, true
		}
		// Later draft saves wait for the receipt.
		p.draftWrites.run(func(finish func()) { release = finish })
		p.sendCleared = true
	}
	if target != nil {
		clearSubmitted()
	}
	optimistic := func() model.Message {
		m := model.Message{ID: r.id, RequestID: r.id, TurnID: r.id, Pending: true, Queued: queued, DeliveryMode: mode,
			Role: "user", Content: model.TextContent(shownText), Attachments: files, Timestamp: now()}
		if replyQuote != nil {
			m.ReplyQuote, _ = json.Marshal(replyQuote)
		}
		return m
	}
	shown := optimistic()
	p.submitting = &shown
	p.ui.followLatest()

	finish := func(err error) {
		release()
		p.submitting, p.sendCleared = nil, false
		if err == nil {
			done(nil)
			return
		}
		if p.awaitingInput == r.id {
			p.awaitingInput = ""
		}
		targetID := ""
		if target != nil {
			targetID = target.ID
		}
		if p.selection == targetID {
			unchanged := !cleared && sameDraft(original, originalFiles, p.text, p.files)
			restored := p.text
			if !unchanged {
				restored = joinNonEmpty("\n\n", original, p.text)
			}
			p.updateText(restored, mergeFiles(files, p.files), targetID)
		}
		done(err)
	}

	prompt := func() {
		if generation != p.sendGeneration {
			finish(core.Errorf(i18n.T("chat.sendCancelled")))
			return
		}
		r.attachments = paths(files)
		p.pendingInput, p.lastPrompt, p.awaitingInput = r, content, r.id
		params := map[string]any{"id": target.ID, "requestId": r.id, "text": content, "deliveryMode": mode}
		if len(files) > 0 {
			params["attachments"] = paths(files)
		}
		if replyQuote != nil {
			params["replyQuote"] = replyQuote
		}
		if len(references) > 0 {
			params["agentReferences"] = references
		}
		chatCall(p, "prompt", params, func(result struct {
			Accepted *bool  `json:"accepted"`
			Error    string `json:"error"`
		}, err error) {
			if err == nil && result.Accepted != nil && !*result.Accepted {
				err = core.Errorf(first(result.Error, i18n.T("chat.sendNotAccepted")))
			}
			if err != nil {
				finish(err)
				return
			}
			// An accepted queue row is not a new active run: cancelling it
			// later must not leave the page waiting for its input.
			if queued && p.awaitingInput == r.id {
				p.awaitingInput = ""
			}
			if p.pendingInput != nil && p.pendingInput.id == r.id {
				p.pendingInput = nil
			}
			if replyQuote != nil && p.botMode.quote == replyQuote {
				p.botMode.quote = nil
			}
			if p.activeID == target.ID {
				known := false
				for _, m := range p.view.Messages {
					if m.ID == r.id || m.RequestID == r.id {
						known = true
						break
					}
				}
				if !known {
					p.view.Messages = model.Upsert(p.view.Messages, optimistic())
				}
				if p.awaitingInput != "" {
					p.view.Running, p.view.Status = true, "running"
				}
			}
			if target.Title == "Chat" {
				p.generateTitle(*target, &content)
			}
			finish(nil)
		})
	}

	// Earlier draft saves first; a new chat is created and its files
	// imported before the prompt.
	afterPrior(func() {
		if target != nil {
			prompt()
			return
		}
		p.startConversation(p.newProjectID, func(created *Conversation, err error) {
			if err != nil {
				finish(err)
				return
			}
			target = created
			p.importFiles(created.ID, files, func(imported []model.Attachment, err error) {
				if err != nil {
					finish(err)
					return
				}
				files = imported
				p.openConversation(created.ID, func(_ model.Snapshot, err error) {
					if err != nil {
						finish(err)
						return
					}
					p.newDraft, p.newFiles = "", nil
					clearSubmitted()
					prompt()
				})
			})
		})
	})
}

// afterWrites runs fn once the draft saves queued so far finished, as
// "await draftWrites.current": a send's receipt barrier holds it too.
func (p *Chat) afterWrites(fn func()) { p.draftWrites.after(fn) }

func isBlank(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' && r != '\v' && r != '\f' && r != 0xa0 && r != 0xfeff && !(r >= 0x2000 && r <= 0x200a) && r != 0x3000 && r != 0x2028 && r != 0x2029 && r != 0x1680 && r != 0x202f && r != 0x205f {
			return false
		}
	}
	return true
}

func joinNonEmpty(sep string, values ...string) string {
	out := ""
	for _, v := range values {
		if v == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += v
	}
	return out
}

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// startConversation creates the new chat with its chosen options and
// opens it.
func (p *Chat) startConversation(projectID string, done func(*Conversation, error)) {
	params := map[string]any{"projectId": projectID, "directory": "", "saveDirectory": p.saveLocation, "model": p.newModel, "title": "Chat"}
	if projectID == "" {
		params["directory"] = p.location
	}
	if p.newThinking != "" {
		params["thinkingLevel"] = p.newThinking
	}
	if p.newFastMode {
		params["fastMode"] = true
	}
	chatCall(p, "conversation.new", params, func(created Conversation, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		use := func(c Conversation) {
			p.catalog.Conversations = append(p.catalog.Conversations, c)
			p.choose(c.ID, nil, func(err error) {
				if err != nil {
					done(nil, err)
					return
				}
				done(p.catalog.conversation(c.ID), nil)
			})
		}
		if projectID == "" && p.newPermission != model.PermissionAsk {
			chatCall(p, "configure", map[string]any{"id": created.ID, "permission": p.newPermission}, func(configured Conversation, err error) {
				if err != nil {
					done(nil, err)
					return
				}
				use(configured)
			})
			return
		}
		use(created)
	})
}

// importFiles copies a new chat's files into its session storage.
func (p *Chat) importFiles(id string, files []model.Attachment, done func([]model.Attachment, error)) {
	if len(files) == 0 {
		done(files, nil)
		return
	}
	var imported []model.Attachment
	var next func(i int)
	next = func(i int) {
		if i == len(files) {
			done(imported, nil)
			return
		}
		f := files[i]
		chatCall(p, "attachments.add", map[string]any{"id": id, "paths": []string{f.Path}}, func(r struct {
			Attachments []model.Attachment `json:"attachments"`
		}, err error) {
			if err != nil {
				done(nil, err)
				return
			}
			if len(r.Attachments) == 0 {
				done(nil, core.Errorf(i18n.T("chat.attachmentImportFailed")))
				return
			}
			added := r.Attachments[0]
			added.Annotation, added.ImageAnnotation = f.Annotation, f.ImageAnnotation
			imported = append(imported, added)
			next(i + 1)
		})
	}
	next(0)
}

// queueAction changes the queue of the open conversation, serialized so
// lock, edit and unlock keep their order.
func (p *Chat) queueAction(action, entry string, extra map[string]any, done func(error)) {
	id := p.activeID
	params := map[string]any{"id": id, "entry": entry, "action": action}
	for k, v := range extra {
		params[k] = v
	}
	p.queueWrites.run(func(finish func()) {
		chatCall(p, "queue", params, func(snapshot model.Snapshot, err error) {
			if err == nil && p.selection == id {
				p.view = model.Apply(p.view, model.Event{Type: "queue_changed", State: &snapshot})
			}
			finish()
			if done != nil {
				done(err)
			}
		})
	})
}

// beginQueueEdit takes a queued message into the composer, locked in the
// queue, keeping the draft it replaces.
func (p *Chat) beginQueueEdit(entry string, done func(error)) {
	if p.queueEdit != nil {
		done(nil)
		return
	}
	var message *model.Message
	for i := range p.view.Messages {
		if p.view.Messages[i].RequestID == entry {
			message = &p.view.Messages[i]
			break
		}
	}
	if message == nil {
		done(nil)
		return
	}
	text, files := message.Text(), message.Attachments
	p.afterWrites(func() {
		p.queueAction("lock", entry, map[string]any{"locked": true}, func(err error) {
			if err != nil {
				done(err)
				return
			}
			p.queueEdit = &queueEdit{id: entry, text: p.text, files: p.files}
			p.updateText(text, files, p.activeID)
			done(nil)
		})
	})
}

// finishQueueEdit saves (or discards) the edit, unlocks the entry or
// steers it into the running turn, and gives the composer back its
// draft.
func (p *Chat) finishQueueEdit(save, steer bool, done func(error)) {
	original := p.queueEdit
	if original == nil {
		done(nil)
		return
	}
	end := func(err error) {
		if err != nil {
			done(err)
			return
		}
		p.queueEdit = nil
		p.updateText(original.text, original.files, p.activeID)
		done(nil)
	}
	release := func() {
		if steer {
			p.queueAction("steer", original.id, nil, end)
		} else {
			p.queueAction("lock", original.id, map[string]any{"locked": false}, end)
		}
	}
	if !save {
		release()
		return
	}
	p.queueAction("update", original.id, map[string]any{"text": model.AnnotatedPrompt(p.text, p.files), "attachments": paths(p.files)}, func(err error) {
		if err != nil {
			done(err)
			return
		}
		release()
	})
}

// steerQueued moves a queued message into the running turn.
func (p *Chat) steerQueued(entry string) {
	p.queueWorking[entry] = true
	p.queueAction("steer", entry, nil, func(err error) {
		delete(p.queueWorking, entry)
		if err != nil {
			p.err = err.Error()
		}
	})
}

// requestSend sends the draft; while a turn runs it first asks whether to
// queue or steer, unless mode says.
func (p *Chat) requestSend(mode model.DeliveryMode) {
	if p.busy || p.view.Paused != nil && *p.view.Paused || isBlank(p.text) && len(p.files) == 0 {
		return
	}
	if p.queueEdit != nil {
		p.run(func(done func(error)) { p.finishQueueEdit(true, mode == model.DeliverySteer, done) })
		return
	}
	if p.view.Running && p.activeID != "" && mode == "" {
		p.queuedSend = &queuedSend{id: p.activeID, text: p.text, files: append([]model.Attachment(nil), p.files...)}
		return
	}
	if mode == "" {
		mode = model.DeliveryQueue
	}
	p.run(func(done func(error)) { p.send(nil, mode, done) })
}

// confirmSend sends the draft the queue confirmation was about.
func (p *Chat) confirmSend(mode model.DeliveryMode) {
	intent := p.queuedSend
	p.queuedSend = nil
	if intent != nil && intent.id == p.activeID {
		p.run(func(done func(error)) { p.send(intent, mode, done) })
	}
}

// stopCurrent pauses the queue and stops the turn without clearing the
// new draft; it also cancels an unfinished send's preflight.
func (p *Chat) stopCurrent() {
	if p.stopInFlight || p.activeID == "" {
		return
	}
	id := p.activeID
	p.stopInFlight, p.stopping = true, true
	p.sendGeneration++
	finish := func(err error) {
		p.stopInFlight, p.stopping = false, false
		if err != nil && p.selection == id {
			p.err = err.Error()
		}
	}
	p.queueAction("pause", "", nil, func(err error) {
		if err != nil {
			finish(err)
			return
		}
		chatCall(p, "stop", map[string]any{"id": id}, func(_ json.RawMessage, err error) {
			if err == nil && p.selection == id {
				p.awaitingInput = ""
			}
			finish(err)
		})
	})
}

// simple sends a method for the open conversation that returns nothing
// the page reads.
func (p *Chat) simple(name string, params map[string]any, done func(error)) {
	chatCall(p, name, params, func(_ json.RawMessage, err error) { done(err) })
}

// respondQuestion answers the agent's question, or cancels it with nil,
// keeping the composer's draft.
func (p *Chat) respondQuestion(answers []string) {
	if len(p.view.Approvals) == 0 || p.view.Approvals[0].Kind != "question" {
		return
	}
	request := p.view.Approvals[0]
	params := map[string]any{"id": p.activeID, "approvalId": request.ID}
	if answers != nil {
		params["answers"] = answers
	} else {
		params["cancel"] = true
	}
	p.run(func(done func(error)) {
		p.simple("question", params, func(err error) {
			if err == nil {
				p.dropApproval(request.ID)
			}
			done(err)
		})
	})
}

// respondApproval allows or denies the waiting tool; scope allows it for
// the rest of the conversation.
func (p *Chat) respondApproval(allow bool, scope string) {
	if len(p.view.Approvals) == 0 {
		return
	}
	id := p.view.Approvals[0].ID
	p.run(func(done func(error)) {
		p.simple("permission", map[string]any{"id": p.activeID, "approvalId": id, "allow": allow, "scope": scope}, func(err error) {
			if err == nil {
				p.dropApproval(id)
			}
			done(err)
		})
	})
}

func (p *Chat) dropApproval(id string) {
	var rest []model.Approval
	for _, a := range p.view.Approvals {
		if a.ID != id {
			rest = append(rest, a)
		}
	}
	p.view.Approvals = rest
}

// dispatchRetry starts the turn again from entry (retry), or with new text
// (edit). The ordered event stream alone carries the replacement branch:
// a second snapshot here could arrive behind its deltas.
func (p *Chat) dispatchRetry(entry string, text *string, restoreRevision string, done func(error)) {
	id := p.activeID
	p.flushMailEdits(id, func(err error) {
		if err != nil {
			done(err)
			return
		}
		if p.closed() || p.activeID != id {
			done(core.Errorf(i18n.T("chat.sendCancelled")))
			return
		}
		p.dispatchRetryAfterMail(entry, text, restoreRevision, done)
	})
}

func (p *Chat) dispatchRetryAfterMail(entry string, text *string, restoreRevision string, done func(error)) {
	key, _ := json.Marshal([]any{p.activeID, entry, text, restoreRevision})
	r := p.retryReceipt
	if r == nil || r.key != string(key) {
		r = &retryReceipt{key: string(key), id: newID()}
	}
	p.retryReceipt = r
	p.awaitingInput = r.id
	p.ui.followLatest()
	p.view.Running, p.view.Status = true, "running"
	method := "retry"
	params := map[string]any{"id": p.activeID, "entry": entry, "requestId": r.id}
	if text != nil {
		method = "edit"
		params["text"] = *text
		if restoreRevision != "" {
			params["restoreRevision"] = restoreRevision
		}
	}
	p.simple(method, params, func(err error) {
		if err != nil {
			if p.awaitingInput == r.id {
				p.awaitingInput = ""
			}
			done(err)
			return
		}
		if p.retryReceipt == r {
			p.retryReceipt = nil
		}
		done(nil)
	})
}

// retryMessage retries the latest saved user message, or entry.
func (p *Chat) retryMessage(entry string, done func(error)) {
	if entry == "" {
		for i := len(p.view.Messages) - 1; i >= 0; i-- {
			if m := p.view.Messages[i]; m.Role == "user" && !m.Pending {
				entry = m.ID
				break
			}
		}
	}
	if p.activeID == "" || entry == "" || p.view.Running || p.view.Paused != nil && *p.view.Paused {
		done(nil)
		return
	}
	p.dispatchRetry(entry, nil, "", done)
}

// editMessage sends new text for the latest user message, stopping the
// running turn first.
func (p *Chat) editMessage(entry, text, restoreRevision string, done func(error)) {
	if p.busy {
		done(core.Errorf(i18n.T("chat.browserDraftBusy")))
		return
	}
	id := p.activeID
	p.busy = true
	end := func(err error) { p.busy = false; done(err) }
	p.afterWrites(func() {
		dispatch := func() { p.dispatchRetry(entry, &text, restoreRevision, end) }
		if !p.view.Running {
			dispatch()
			return
		}
		p.simple("stop", map[string]any{"id": id}, func(err error) {
			if err != nil {
				end(err)
				return
			}
			dispatch()
		})
	})
}

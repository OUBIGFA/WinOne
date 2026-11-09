// Package studybook presents the bundled Anki collection using native controls.
package studybook

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type choice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type deck struct {
	choice
	PresetID string                                     `json:"presetId"`
	Kind     string                                     `json:"kind"`
	Counts   struct{ New, Learning, Review, Total int } `json:"counts"`
}
type note struct {
	ID       string            `json:"id"`
	NoteType choice            `json:"noteType"`
	Fields   map[string]string `json:"fields"`
	Tags     []string          `json:"tags"`
	CardIDs  []string          `json:"cardIds"`
}
type overview struct {
	Profiles      []string `json:"profiles"`
	ActiveProfile string   `json:"activeProfile"`
	Decks         []deck   `json:"decks"`
	NoteTypes     []choice `json:"noteTypes"`
	FSRS          bool     `json:"fsrs"`
	Undo, Redo    string
}
type browseResult struct {
	Notes  []note  `json:"notes"`
	Cursor *string `json:"nextCursor"`
}
type field struct {
	Name string `json:"name"`
}
type noteType struct {
	choice
	Fields    []field          `json:"fields"`
	CSS       string           `json:"css"`
	Templates []map[string]any `json:"templates"`
}
type review struct {
	Advance          advanceOptions                   `json:"autoAdvance"`
	CustomApplied    bool                             `json:"customApplied"`
	Card             struct{ ID, NoteID string }      `json:"card"`
	MutationKey      string                           `json:"mutationKey"`
	CustomScheduling string                           `json:"customScheduling"`
	AutoPlay         bool                             `json:"autoplay"`
	ReplayQuestion   bool                             `json:"replayQuestion"`
	Speech           []speechItem                     `json:"speech"`
	Typing           bool                             `json:"typing"`
	MediaDirectory   string                           `json:"mediaDirectory"`
	Active           bool                             `json:"active"`
	Side             string                           `json:"side"`
	Token            string                           `json:"stateToken"`
	Answers          []struct{ Rating, Label string } `json:"answers"`
	Rendered         struct {
		Question string `json:"questionHtml"`
		Answer   string `json:"answerHtml"`
		CSS      string `json:"css"`
	} `json:"rendered"`
}

type Book struct {
	cardNetwork                                       bool
	cardLink                                          string
	pendingAction                                     func()
	autoAdvance                                       bool
	autoTimer                                         *time.Timer
	autoEpoch                                         uint64
	recording                                         *voiceRecording
	importForm                                        toolForm
	importPath                                        string
	pendingSelection                                  []string
	reviewAction, reviewFlag                          string
	confirmDelete                                     bool
	connectSaved                                      *connectSettings
	connectBaseline                                   string
	customReady                                       bool
	speechCancel                                      func()
	speaking                                          bool
	statistics                                        *studyStatistics
	visible                                           atomic.Bool
	pollOnce                                          sync.Once
	stopPoll                                          func()
	pendingExternal                                   []json.RawMessage
	image                                             *imageDraft
	toolSection                                       int
	connect                                           *connectSettings
	connectPort, connectOrigins                       string
	fullSyncConfirmed                                 bool
	fullSyncDirection                                 string
	newProfile                                        string
	filePreview                                       any
	env                                               *core.Env
	state                                             *overview
	tab                                               int
	busy, closed                                      bool
	err, notice, query, cursor, newDeck               string
	notes                                             []note
	selected                                          map[string]bool
	edit                                              *note
	model                                             *noteType
	modelID, deckID, tags                             string
	savedEdit                                         string
	editorID                                          string
	review                                            *review
	render                                            *cardView
	pending                                           json.RawMessage
	typed                                             string
	form                                              toolForm
	bulk, bulkText                                    string
	filePath, exportKind                              string
	replaceConfirmed, includeMedia, includeScheduling bool
	username, password, endpoint                      string
}

func New(env *core.Env) *Book { b := &Book{env: env, selected: map[string]bool{}}; b.load(); return b }
func text(key string) string  { return i18n.T("study." + key) }
func (b *Book) request(method string, params, out any) error {
	return b.env.Host.CallCompletingInto("learning.call", map[string]any{"method": method, "params": params}, out)
}
func (b *Book) act(method string, params any, done func(json.RawMessage)) {
	if b.busy || b.closed {
		return
	}
	b.busy = true
	b.err = ""
	core.Go(b.env, func() (json.RawMessage, error) {
		var out json.RawMessage
		err := b.request(method, params, &out)
		return out, err
	}, func(out json.RawMessage, err error) {
		if b.closed {
			return
		}
		b.busy = false
		if err != nil {
			b.err = err.Error()
			return
		}
		if done != nil {
			done(out)
		}
		if len(b.pending) > 0 && !b.busy {
			pending := b.pending
			b.pending = nil
			b.Navigate(pending)
		}
		if b.pendingAction != nil && !b.busy {
			action := b.pendingAction
			b.pendingAction = nil
			action()
		}
	})
}
func (b *Book) load() {
	b.act("winone.overview", map[string]any{}, func(raw json.RawMessage) {
		var value overview
		if err := json.Unmarshal(raw, &value); err != nil {
			b.err = err.Error()
			return
		}
		b.state = &value
		if b.deckID == "" && len(value.Decks) > 0 {
			b.deckID = value.Decks[0].ID
		}
		if b.modelID == "" && len(value.NoteTypes) > 0 {
			b.modelID = value.NoteTypes[0].ID
		}
	})
}
func (b *Book) Dirty() bool {
	if b.importForm.dirty {
		return true
	}
	if b.connectSaved != nil && b.connectDraft() != b.connectBaseline {
		return true
	}
	if b.image != nil && b.image.dirty {
		return true
	}
	if b.form.dirty {
		return true
	}
	if b.edit == nil {
		return false
	}
	raw, _ := json.Marshal(b.edit)
	return string(raw)+b.tags != b.savedEdit
}
func (b *Book) Busy() bool { return b.busy }

// DeckNames reads the last loaded overview without starting another request.
func (b *Book) DeckNames() []string {
	var names []string
	if b.state != nil {
		for _, deck := range b.state.Decks {
			names = append(names, deck.Name)
		}
	}
	return names
}
func (b *Book) Discard() {
	if b.importForm.dirty {
		b.importForm = toolForm{}
		b.filePreview = nil
	}
	if b.connectSaved != nil {
		b.acceptConnect(*b.connectSaved)
	}
	b.image = nil
	if b.editorID != "" {
		id := b.editorID
		b.act("winone.editor.close", map[string]string{"editorId": id}, nil)
	}
	b.editorID = ""
	b.edit = nil
	b.model = nil
	b.savedEdit = ""
	if b.form.dirty {
		b.form.value = map[string]any{}
		b.form.inputs = map[string]string{}
		b.form.variants = map[string]string{}
		b.form.errors = map[string]string{}
	}
	b.form.dirty = false
}
func (b *Book) Close() {
	b.stopAutoAdvance()
	b.stopRecording()
	b.stopSpeech()
	b.visible.Store(false)
	if b.stopPoll != nil {
		b.stopPoll()
	}
	if b.editorID != "" {
		id := b.editorID
		go func() {
			_ = b.env.Host.CallInto("learning.call", map[string]any{"method": "winone.editor.close", "params": map[string]string{"editorId": id}}, nil)
		}()
	}
	b.closed = true
	if b.render != nil {
		b.render.Close()
	}
}
func (b *Book) Navigate(raw json.RawMessage) {
	if b.busy {
		b.pending = append(json.RawMessage(nil), raw...)
		return
	}
	var event struct {
		Page, Query, EditorID, Path string
		CardIDs                     []string `json:"cardIds"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return
	}
	if b.editorID != "" {
		id := b.editorID
		b.act("winone.editor.close", map[string]string{"editorId": id}, func(json.RawMessage) { b.editorID = ""; b.Discard(); b.Navigate(raw) })
		return
	}
	b.Discard()
	if event.Page == "editor" {
		b.openExternalEditor(event.EditorID)
		return
	}
	if event.Page == "import" {
		b.tab = 4
		b.toolSection = 1
		b.filePath = event.Path
		return
	}
	if event.Page == "sync" {
		b.tab = 4
		b.toolSection = 2
		return
	}
	if event.Page == "review" {
		b.tab = 2
		b.loadReview()
	} else if event.Page == "browser" {
		b.tab = 1
		b.query = event.Query
		b.pendingSelection = event.CardIDs
		b.search(false)
	} else {
		b.tab = 0
		b.load()
	}
}
func (b *Book) switchTab(tab int) {
	change := func() {
		if b.busy {
			b.pendingAction = func() { b.switchTab(tab) }
			return
		}
		b.stopRecording()
		b.stopSpeech()
		b.stopAutoAdvance()
		if b.editorID != "" {
			id := b.editorID
			b.act("winone.editor.close", map[string]string{"editorId": id}, func(json.RawMessage) { b.editorID = ""; b.Discard(); b.switchTab(tab) })
			return
		}
		b.Discard()
		b.tab = tab
		if tab == 0 {
			b.load()
		} else if tab == 1 {
			b.search(false)
		} else if tab == 2 {
			b.loadReview()
		} else if tab == 5 {
			b.loadStatistics()
		}
	}
	if b.env.RequestChange != nil {
		b.env.RequestChange(change)
	} else {
		change()
	}
}

func (b *Book) View(c *ui.Context) {
	if b.busy {
		if kit.GhostButton(c, text("cancelOperation"), icons.X, false).Clicked() {
			core.Do(b.env, func() error { return b.env.Host.CallInto("learning.cancel", nil, nil) }, func(err error) {
				if !b.closed && err != nil {
					b.err = err.Error()
				}
			})
		}
	}
	if !b.busy && !b.Dirty() && len(b.pendingExternal) > 0 {
		event := b.pendingExternal[0]
		b.pendingExternal = b.pendingExternal[1:]
		b.externalEvent(event)
	}
	if b.err != "" {
		kit.Notice(c, kit.Failure, "", b.err, nil)
	}
	if b.notice != "" {
		kit.Notice(c, kit.Info, "", b.notice, nil)
	}
	if b.state == nil {
		if b.busy {
			kit.Status(c, text("loading"))
		} else if kit.Button(c, text("retry"), icons.RefreshCw, false).Clicked() {
			b.load()
		}
		return
	}
	active := b.tab
	kit.Tabs(c, &active, []kit.Tab{{Label: text("decks")}, {Label: text("browse")}, {Label: text("review")}, {Label: text("types")}, {Label: text("tools")}, {Label: text("statistics")}}, func(apply func()) { apply(); b.switchTab(active) })
	ui.Column(c).FillWidth().Gap(16).Disabled(b.busy).Children(func() {
		if b.image != nil {
			b.imageEditor(c)
			return
		}
		if b.edit != nil && b.model != nil {
			b.editor(c)
			return
		}
		switch b.tab {
		case 0:
			b.decks(c)
		case 1:
			b.browser(c)
		case 2:
			b.reviewer(c)
		case 3:
			b.types(c)
		case 4:
			b.tools(c)
		case 5:
			b.statisticsView(c)
		}
	})
}
func (b *Book) decks(c *ui.Context) {
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		ui.TextInput(c, &b.newDeck).Placeholder(text("deckName")).Label(text("deckName")).Grow(1).Basis(240).MinWidth(0)
		if kit.Button(c, text("createDeck"), icons.Plus, false).Disabled(strings.TrimSpace(b.newDeck) == "").Clicked() {
			b.act("decks.create", map[string]any{"name": b.newDeck}, func(json.RawMessage) { b.newDeck = ""; b.load() })
		}
		if kit.GhostButton(c, text("refresh"), icons.RefreshCw, false).Clicked() {
			b.load()
		}
	})
	for _, d := range b.state.Decks {
		d := d
		ui.Column(c.Key(d.ID)).FillWidth().Gap(8).Padding(12, 0).Children(func() {
			kit.Label(c, d.Name)
			kit.Help(c, i18n.T("study.counts", "new", d.Counts.New, "learning", d.Counts.Learning, "review", d.Counts.Review, "total", d.Counts.Total))
			ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
				if kit.Button(c, text("review"), icons.Play, false).Clicked() {
					b.deckID = d.ID
					b.act("application.navigate", map[string]any{"view": "review", "deck": map[string]string{"id": d.ID}}, func(json.RawMessage) { b.tab = 2; b.loadReview() })
				}
				if kit.GhostButton(c, text("browse"), icons.BookOpen, false).Clicked() {
					b.deckID = d.ID
					b.query = `"deck:` + strings.ReplaceAll(d.Name, `"`, `\"`) + `"`
					b.tab = 1
					b.search(false)
				}
				if kit.GhostButton(c, text("deckOptions"), icons.Settings, false).Clicked() {
					b.act("deckPresets.get", map[string]any{"preset": map[string]string{"id": d.PresetID}}, func(raw json.RawMessage) {
						var preset map[string]any
						if err := json.Unmarshal(raw, &preset); err != nil {
							b.err = err.Error()
							return
						}
						b.openTool("deckPresets.update", map[string]any{"preset": map[string]string{"id": d.PresetID}, "name": preset["name"], "settings": preset["settings"]})
					})
				}
				if kit.GhostButton(c, text("rename"), icons.Pencil, false).Clicked() {
					b.openTool("decks.update", map[string]any{"deck": map[string]string{"id": d.ID}, "name": d.Name})
				}
			})
		})
	}
	kit.Help(c, text("fsrsHint"))
}
func (b *Book) search(appendRows bool) {
	params := map[string]any{"query": b.query, "limit": 50}
	if appendRows && b.cursor != "" {
		params["cursor"] = b.cursor
	}
	b.act("winone.browser", params, func(raw json.RawMessage) {
		var result browseResult
		if err := json.Unmarshal(raw, &result); err != nil {
			b.err = err.Error()
			return
		}
		if appendRows {
			b.notes = append(b.notes, result.Notes...)
		} else {
			b.notes = result.Notes
			b.selected = map[string]bool{}
		}
		if len(b.pendingSelection) > 0 {
			for _, n := range b.notes {
				for _, id := range n.CardIDs {
					for _, selected := range b.pendingSelection {
						if id == selected {
							b.selected[n.ID] = true
						}
					}
				}
			}
			b.pendingSelection = nil
		}
		b.cursor = ""
		if result.Cursor != nil {
			b.cursor = *result.Cursor
		}
	})
}
func (b *Book) browser(c *ui.Context) {
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		ui.TextInput(c, &b.query).Grow(1).Basis(240).MinWidth(0).Placeholder(text("searchHint")).Label(text("search"))
		if kit.Button(c, text("search"), icons.Search, false).Clicked() {
			b.search(false)
		}
		if kit.Button(c, text("addNote"), icons.Plus, false).Clicked() {
			b.beginEdit(nil)
		}
		if kit.Button(c, text("imageOcclusion"), icons.Image, false).Clicked() {
			b.chooseImage()
		}
	})
	b.bulkActions(c)
	if len(b.notes) == 0 {
		kit.Help(c, text("empty"))
	}
	for _, n := range b.notes {
		n := n
		ui.Row(c.Key(n.ID)).FillWidth().Gap(12).Padding(12, 0).Children(func() {
			selected := b.selected[n.ID]
			if ui.Checkbox(c, &selected, "").Label(text("selectNote")).Changed() {
				b.selected[n.ID] = selected
				var ids []string
				for id, yes := range b.selected {
					if yes {
						ids = append(ids, id)
					}
				}
				if ids == nil {
					ids = []string{}
				}
				b.act("browser.select", map[string]any{"noteIds": ids}, nil)
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				values := make([]string, 0, len(n.Fields))
				for _, key := range []string{"Word", "Front", "Meaning", "Back"} {
					if value := n.Fields[key]; value != "" {
						values = append(values, value)
					}
				}
				if len(values) == 0 {
					for _, value := range n.Fields {
						values = append(values, value)
						break
					}
				}
				ui.Text(c, strings.Join(values, " · ")).Selectable()
				kit.Help(c, n.NoteType.Name+" · "+strings.Join(n.Tags, " "))
			})
			if kit.GhostButton(c, text("edit"), icons.Pencil, false).Clicked() {
				b.beginEdit(&n)
			}
			if strings.Contains(strings.ToLower(n.NoteType.Name), "occlusion") || strings.Contains(n.NoteType.Name, "遮挡") {
				if kit.GhostButton(c, text("editMasks"), icons.Image, false).Clicked() {
					b.openImage("", n.ID)
				}
			}
		})
	}
	if b.cursor != "" && kit.Button(c, text("more"), icons.ChevronDown, false).Clicked() {
		b.search(true)
	}
}
func options(values []choice) []kit.SelectOption {
	out := make([]kit.SelectOption, len(values))
	for i, v := range values {
		out[i] = kit.SelectOption{Value: v.ID, Label: v.Name}
	}
	return out
}
func (b *Book) beginEdit(n *note) {
	if b.editorID != "" {
		id := b.editorID
		b.act("winone.editor.close", map[string]string{"editorId": id}, func(json.RawMessage) { b.editorID = ""; b.beginEdit(n) })
		return
	}
	if n != nil {
		b.modelID = n.NoteType.ID
	}
	b.act("noteTypes.get", map[string]any{"noteType": map[string]string{"id": b.modelID}}, func(raw json.RawMessage) {
		var model noteType
		if err := json.Unmarshal(raw, &model); err != nil {
			b.err = err.Error()
			return
		}
		b.model = &model
		if n == nil {
			b.edit = &note{NoteType: model.choice, Fields: map[string]string{}, Tags: []string{}}
		} else {
			copied := *n
			copied.Fields = map[string]string{}
			for k, v := range n.Fields {
				copied.Fields[k] = v
			}
			b.edit = &copied
		}
		b.tags = strings.Join(b.edit.Tags, " ")
		saved, _ := json.Marshal(b.edit)
		b.savedEdit = string(saved) + b.tags
		method := "editors.openNote"
		params := map[string]any{"noteId": b.edit.ID}
		if b.edit.ID == "" {
			method = "editors.openAdd"
			params = map[string]any{"note": map[string]any{"noteType": map[string]string{"id": b.modelID}, "deck": map[string]string{"id": b.deckID}, "fields": b.edit.Fields, "tags": []string{}}}
		}
		b.act(method, params, func(raw json.RawMessage) {
			var value struct{ ID string }
			if err := json.Unmarshal(raw, &value); err != nil {
				b.err = err.Error()
				return
			}
			b.editorID = value.ID
		})
	})
}
func (b *Book) editor(c *ui.Context) {
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		if kit.GhostButton(c, text("back"), icons.ChevronLeft, false).Clicked() {
			if b.env.RequestChange != nil {
				b.env.RequestChange(b.Discard)
			} else {
				b.Discard()
			}
		}
		kit.Heading(c, text("edit"))
	})
	if b.edit.ID == "" {
		ui.Field(c, text("noteType"), func() {
			next := b.modelID
			if changed, _ := kit.Select(c, &next, options(b.state.NoteTypes)); changed {
				change := func() { b.modelID = next; b.beginEdit(nil) }
				if b.env.RequestChange != nil {
					b.env.RequestChange(change)
				} else {
					change()
				}
			}
		}).FillWidth()
		decks := make([]choice, len(b.state.Decks))
		for i, d := range b.state.Decks {
			decks[i] = d.choice
		}
		ui.Field(c, text("deck"), func() { kit.Select(c, &b.deckID, options(decks)) }).FillWidth()
	}
	for _, f := range b.model.Fields {
		f := f
		ui.Field(c.Key(f.Name), f.Name, func() {
			value := b.edit.Fields[f.Name]
			input := ui.TextArea(c, &value).Lines(2, 12)
			if input.Changed() {
				b.edit.Fields[f.Name] = value
			}
			b.fieldTools(c, input, f.Name)
		}).FillWidth()
	}
	ui.Field(c, text("tags"), func() { ui.TextInput(c, &b.tags).FillWidth() }).FillWidth()
	b.recordControls(c)
	if kit.Button(c, text("saveNote"), icons.Save, false).Clicked() {
		b.saveNote()
	}
}
func (b *Book) saveNote() {
	patch := map[string]any{"fields": b.edit.Fields, "tags": strings.Fields(b.tags)}
	if b.edit.ID == "" {
		patch["deck"] = map[string]string{"id": b.deckID}
		patch["noteType"] = map[string]string{"id": b.modelID}
	}
	b.act("editors.updateDraft", map[string]any{"editorId": b.editorID, "patch": patch}, func(json.RawMessage) {
		b.act("winone.editor.save", map[string]string{"editorId": b.editorID}, func(json.RawMessage) { b.editorID = ""; b.Discard(); b.tab = 1; b.search(false) })
	})
}
func (b *Book) loadReview() {
	b.stopAutoAdvance()
	b.stopSpeech()
	b.act("winone.review", map[string]any{"typed": b.typed}, func(raw json.RawMessage) {
		var value review
		if err := json.Unmarshal(raw, &value); err != nil {
			b.err = err.Error()
			return
		}
		b.review = &value
		b.cardLink = ""
		b.customReady = value.CustomScheduling == "" || value.CustomApplied
		if value.AutoPlay {
			b.playSpeech()
		}
		if b.review.Side == "question" {
			b.confirmDelete = false
		}
		b.scheduleAutoAdvance()
		if b.render != nil {
			b.render.Close()
			b.render = nil
		}
	})
}
func (b *Book) reviewCommand(command string) {
	if command == "statesMutated" {
		b.customReady = true
		if b.review != nil {
			b.review.CustomApplied = true
		}
		return
	}
	if b.closed || b.busy || b.review == nil || !b.review.Active {
		return
	}
	if command == "ans" && b.review.Side != "answer" {
		b.act("reviewer.showAnswer", map[string]string{"stateToken": b.review.Token}, func(json.RawMessage) { b.loadReview() })
		return
	}
	if b.review.Side == "answer" && b.customReady {
		ratings := map[string]string{"ease1": "again", "ease2": "hard", "ease3": "good", "ease4": "easy"}
		if rating := ratings[command]; rating != "" {
			b.act("reviewer.answer", map[string]string{"stateToken": b.review.Token, "rating": rating}, func(json.RawMessage) { b.typed = ""; b.loadReview() })
		}
	}
}

func (b *Book) reviewer(c *ui.Context) {
	if b.review == nil {
		if kit.Button(c, text("refresh"), icons.RefreshCw, false).Clicked() {
			b.loadReview()
		}
		return
	}
	if !b.review.Active {
		kit.Heading(c, text("finished"))
		if kit.Button(c, text("decks"), icons.BookOpen, false).Clicked() {
			b.tab = 0
			b.load()
		}
		return
	}
	html := b.review.Rendered.Question
	if b.review.Side == "answer" {
		html = b.review.Rendered.Answer
	}
	if b.render != nil && b.render.dark != c.Theme().Dark {
		b.render.Close()
		b.render = nil
	}
	if b.render == nil {
		var err error
		b.render, err = newCardView(b.env, html, b.review, b.reviewCommand, func(link string) {
			if !b.closed {
				b.cardLink = link
			}
		}, c.Theme().Dark, b.cardNetwork)
		if err != nil {
			b.err = err.Error()
		}
	}
	if b.render != nil {
		b.render.View(c)
	}
	if b.cardLink != "" {
		ui.Text(c, b.cardLink).Selectable()
		if kit.GhostButton(c, text("openCardLink"), icons.ExternalLink, false).Clicked() {
			c.Services().OpenURL(b.cardLink)
		}
	}
	if ui.Checkbox(c, &b.cardNetwork, text("cardNetwork")).Changed() {
		if b.render != nil {
			b.render.Close()
			b.render = nil
		}
	}
	if c.Shortcut(0, ui.KeySpace) {
		b.reviewCommand("ans")
	}
	if b.review.Side == "answer" {
		for index, key := range []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4} {
			if c.Shortcut(0, key) {
				b.reviewCommand("ease" + strconv.Itoa(index+1))
			}
		}
	}
	if b.review.Typing && b.review.Side != "answer" {
		ui.Field(c, text("typeAnswer"), func() {
			input := ui.TextInput(c, &b.typed).FillWidth()
			if input.Shortcut(0, ui.KeyEnter) {
				b.reviewCommand("ans")
			}
		}).FillWidth()
	}
	if len(b.review.Speech) > 0 {
		if kit.Button(c, text("speakCard"), icons.Volume2, b.speaking).Clicked() {
			if b.speaking {
				b.stopSpeech()
			} else {
				b.playSpeech()
			}
		}
	}
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		if b.review.Side != "answer" {
			if kit.Button(c, text("showAnswer"), icons.Eye, false).Clicked() {
				b.act("reviewer.showAnswer", map[string]string{"stateToken": b.review.Token}, func(json.RawMessage) { b.loadReview() })
			}
		} else {
			for _, answer := range b.review.Answers {
				answer := answer
				if kit.Button(c, text(answer.Rating)+" · "+answer.Label, nil, false).Disabled(!b.customReady).Clicked() {
					b.act("reviewer.answer", map[string]string{"stateToken": b.review.Token, "rating": answer.Rating}, func(json.RawMessage) { b.typed = ""; b.loadReview() })
				}
			}
		}
		if kit.GhostButton(c, text("undo"), icons.Undo2, false).Clicked() {
			b.act("collection.undo", map[string]any{}, func(json.RawMessage) {
				b.act("collection.refresh", map[string]string{"target": "cachesAndWindows"}, func(json.RawMessage) { b.loadReview() })
			})
		}
	})
	b.reviewActions(c)
	b.recordControls(c)
	b.autoAdvanceControl(c)
}
func (b *Book) types(c *ui.Context) {
	for _, model := range b.state.NoteTypes {
		model := model
		ui.Column(c.Key(model.ID)).FillWidth().Gap(8).Padding(12, 0).Children(func() {
			kit.Label(c, model.Name)
			ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
				if kit.Button(c, text("addNote"), icons.Plus, false).Clicked() {
					b.modelID = model.ID
					b.beginEdit(nil)
				}
				if kit.GhostButton(c, text("editType"), icons.Settings, false).Clicked() {
					b.act("noteTypes.get", map[string]any{"noteType": map[string]string{"id": model.ID}}, func(raw json.RawMessage) {
						var value map[string]any
						if err := json.Unmarshal(raw, &value); err != nil {
							b.err = err.Error()
							return
						}
						patch := map[string]any{}
						for _, key := range []string{"name", "css", "latexHeader", "latexFooter", "latexSvg", "sortField"} {
							patch[key] = value[key]
						}
						b.openTool("noteTypes.update", map[string]any{"noteType": map[string]string{"id": model.ID}, "patch": patch})
					})
				}
				if kit.GhostButton(c, text("fieldsTemplates"), icons.Pencil, false).Clicked() {
					b.openTool("noteTypes.editStructure", map[string]any{"noteType": map[string]string{"id": model.ID}, "edits": []any{}})
				}
			})
		})
	}
	if kit.Button(c, text("createType"), icons.Plus, false).Clicked() {
		b.openTool("noteTypes.create", map[string]any{})
	}
}
func (b *Book) tools(c *ui.Context) {
	sections := []string{text("collectionTools"), text("files"), text("sync"), text("connectTitle"), text("allTools")}
	next := b.toolSection
	kit.SubTabs(c, &next, sections, func(apply func()) {
		change := func() { apply(); b.toolSection = next }
		if b.env.RequestChange != nil {
			b.env.RequestChange(change)
		} else {
			change()
		}
	})
	if b.toolSection == 1 {
		b.fileTools(c)
		return
	}
	if b.toolSection == 2 {
		b.syncTools(c)
		return
	}
	if b.toolSection == 3 {
		b.connectTools(c)
		return
	}
	if b.toolSection == 4 {
		b.advancedTools(c)
		return
	}
	b.profileTools(c)
	fsrs := b.state.FSRS
	if kit.Toggle(c, &fsrs, text("fsrs"), text("fsrsHint")).Changed() {
		b.act("winone.fsrs", map[string]bool{"enabled": fsrs}, func(json.RawMessage) { b.load() })
	}
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		for _, item := range []struct{ key, method string }{{"backup", "winone.backup"}, {"check", "collection.checkIntegrity"}, {"undo", "collection.undo"}, {"redo", "winone.redo"}} {
			item := item
			if kit.Button(c, text(item.key), nil, false).Clicked() {
				b.act(item.method, map[string]any{}, func(raw json.RawMessage) { b.notice = text("completed"); b.load() })
			}
		}
	})
}

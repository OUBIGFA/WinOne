package studybook

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"
)

// Poll only a visible study surface. The Host serves this small change queue
// without starting Anki; external dictionary actions can update an open page.
func (b *Book) SetVisible(visible bool) {
	wasVisible:=b.visible.Load()
	if !visible && b.visible.Load() {
		b.autoAdvance = false
		b.stopAutoAdvance()
		b.stopRecording()
		b.stopSpeech()
		if b.render != nil {
			b.render.Close()
			b.render = nil
		}
	}
	b.visible.Store(visible)
	if visible&&!wasVisible&&b.tab==2&&!b.busy&&!b.Dirty(){b.loadReview()}
	if !visible || b.closed {
		return
	}
	b.pollOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		b.stopPoll = cancel
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			var after uint64
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if !b.visible.Load() {
						continue
					}
					var result struct {
						Sequence uint64 `json:"sequence"`
						Events   []struct {
							Owner uint32          `json:"owner"`
							Event json.RawMessage `json:"event"`
						} `json:"events"`
					}
					if err := b.env.Host.CallInto("learning.events", map[string]any{"after": after}, &result); err != nil {
						b.env.Post(func() {
							if !b.closed && b.visible.Load() {
								b.notice = err.Error()
							}
						})
						continue
					}
					after = result.Sequence
					var events []json.RawMessage
					for _, event := range result.Events {
						if event.Owner != uint32(os.Getpid()) {
							events = append(events, event.Event)
						}
					}
					if len(events) > 0 {
						b.env.Post(func() {
							if b.closed || !b.visible.Load() {
								return
							}
							for _, event := range events {
								b.externalEvent(event)
							}
						})
					}
				}
			}
		}()
	})
}
func (b *Book) externalEvent(raw json.RawMessage) {
	var event struct{ Type, Page, EditorID string }
	if json.Unmarshal(raw, &event) != nil {
		return
	}
	if b.busy || b.Dirty() {
		// A busy editor needs the newest refresh of each kind, not an
		// unbounded queue of external review or collection notifications.
		if event.Type != "navigate" {
			for index, queued := range b.pendingExternal {
				var prior struct{ Type string }
				if json.Unmarshal(queued, &prior) == nil && prior.Type == event.Type {
					b.pendingExternal[index] = raw
					return
				}
			}
		}
		b.pendingExternal = append(b.pendingExternal, raw)
		return
	}
	if event.Type == "navigate" {
		b.Navigate(raw)
		return
	}
	if event.Type == "editor.changed" && event.EditorID == b.editorID {
		b.openExternalEditor(event.EditorID)
		return
	}
	if event.Type == "audio" && b.tab == 2 {
		b.playSpeech()
		if b.render != nil {
			b.render.Replay()
		}
		return
	}
	if event.Type == "closeView" {
		b.Discard()
		b.review = nil
		b.state = nil
		return
	}
	if event.Type == "review.changed" && b.tab == 2 {
		b.loadReview()
		return
	}
	if event.Type == "collection.changed" {
		if b.tab == 1 {
			b.search(false)
		} else if b.tab == 0 {
			b.load()
		}
	}
}
func (b *Book) openExternalEditor(id string) {
	b.act("winone.editor.get", map[string]string{"editorId": id}, func(raw json.RawMessage) {
		var record struct {
			ID, NoteID string
			NoteType   choice
			Deck       *choice
			Fields     map[string]string
			Tags       []string
		}
		if err := json.Unmarshal(raw, &record); err != nil {
			b.err = err.Error()
			return
		}
		b.editorID = record.ID
		b.modelID = record.NoteType.ID
		if record.Deck != nil {
			b.deckID = record.Deck.ID
		}
		b.edit = &note{ID: record.NoteID, NoteType: record.NoteType, Fields: record.Fields, Tags: record.Tags}
		b.tags = strings.Join(record.Tags, " ")
		saved, _ := json.Marshal(b.edit)
		b.savedEdit = string(saved) + b.tags
		b.tab = 1
		b.act("noteTypes.get", map[string]any{"noteType": map[string]string{"id": b.modelID}}, func(raw json.RawMessage) {
			var model noteType
			if err := json.Unmarshal(raw, &model); err != nil {
				b.err = err.Error()
				return
			}
			b.model = &model
		})
	})
}

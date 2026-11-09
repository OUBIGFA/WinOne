package studybook

import (
	"encoding/json"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"time"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
)

type advanceOptions struct {
	QuestionSeconds, AnswerSeconds float64
	QuestionAction, AnswerAction   int
	WaitForAudio                   bool
}

func (b *Book) stopAutoAdvance() {
	if b.autoTimer != nil {
		b.autoTimer.Stop()
		b.autoTimer = nil
	}
	b.autoEpoch++
}
func (b *Book) scheduleAutoAdvance() {
	b.stopAutoAdvance()
	if !b.autoAdvance || b.review == nil || !b.review.Active {
		return
	}
	seconds := b.review.Advance.QuestionSeconds
	if b.review.Side == "answer" {
		seconds = b.review.Advance.AnswerSeconds
	}
	if seconds <= 0 {
		return
	}
	epoch := b.autoEpoch
	b.autoTimer = time.AfterFunc(time.Duration(seconds*float64(time.Second)), func() { b.env.Post(func() { b.advanceReview(epoch) }) })
}
func (b *Book) advanceReview(epoch uint64) {
	if b.closed || !b.autoAdvance || epoch != b.autoEpoch || !b.visible.Load() || b.review == nil || !b.review.Active {
		return
	}
	if mygo.FocusedWindow() == nil {
		b.autoAdvance = false
		b.stopAutoAdvance()
		return
	}
	if b.busy || !b.customReady {
		return
	}
	if b.review.Advance.WaitForAudio && b.speaking {
		return
	}
	if b.review.Advance.WaitForAudio && b.render != nil {
		view := b.render.view
		core.Go(b.env, func() (bool, error) {
			value, err := view.Page().Eval(`return [...document.querySelectorAll('audio,video')].some(item=>!item.paused&&!item.ended);`)
			playing, _ := value.(bool)
			return playing, err
		}, func(playing bool, err error) {
			if b.closed || epoch != b.autoEpoch {
				return
			}
			if err != nil {
				b.err = err.Error()
				b.autoAdvance = false
				return
			}
			if !playing {
				b.applyAdvance()
			}
		})
		return
	}
	b.applyAdvance()
}
func (b *Book) applyAdvance() {
	if b.review.Side == "question" {
		if b.review.Advance.QuestionAction == 0 {
			b.reviewCommand("ans")
		} else {
			b.notice = text("questionElapsed")
		}
		return
	}
	switch b.review.Advance.AnswerAction {
	case 1:
		b.reviewCommand("ease1")
	case 2:
		b.reviewCommand("ease3")
	case 3:
		b.reviewCommand("ease2")
	case 4:
		b.notice = text("answerElapsed")
	default:
		b.act("winone.review.action", map[string]string{"stateToken": b.review.Token, "action": "buryCard"}, func(json.RawMessage) { b.loadReview() })
	}
}
func (b *Book) autoAdvanceControl(c *ui.Context) {
	if kit.Toggle(c, &b.autoAdvance, text("autoAdvance"), text("autoAdvanceHint")).Changed() {
		b.scheduleAutoAdvance()
	}
}

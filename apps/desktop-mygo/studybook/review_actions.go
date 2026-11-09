package studybook

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"strconv"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func (b *Book) reviewActions(c *ui.Context) {
	var options []kit.SelectOption
	for _, key := range []string{"buryCard", "buryNote", "suspendCard", "suspendNote", "mark", "flag", "reset", "deleteNote"} {
		options = append(options, kit.SelectOption{Value: key, Label: text(key)})
	}
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		if kit.GhostButton(c, text("edit"), icons.Pencil, false).Clicked() {
			b.act("notes.get", map[string]string{"id": b.review.Card.NoteID}, func(raw json.RawMessage) {
				var n note
				if err := json.Unmarshal(raw, &n); err != nil {
					b.err = err.Error()
					return
				}
				b.beginEdit(&n)
			})
		}
		if kit.GhostButton(c, text("cardInfo"), icons.Info, false).Clicked() {
			b.openTool("card_stats", map[string]any{"cid": b.review.Card.ID})
		}
		kit.Select(c, &b.reviewAction, options)
		if b.reviewAction == "flag" {
			var flags []kit.SelectOption
			for i := 0; i <= 7; i++ {
				flags = append(flags, kit.SelectOption{Value: strconv.Itoa(i), Label: strconv.Itoa(i)})
			}
			kit.Select(c, &b.reviewFlag, flags)
		}
		if b.reviewAction == "deleteNote" {
			ui.Checkbox(c, &b.confirmDelete, text("confirmDelete"))
		}
		if kit.Button(c, text("apply"), icons.Check, false).Disabled(b.reviewAction == "" || b.reviewAction == "deleteNote" && !b.confirmDelete).Clicked() {
			flag, _ := strconv.Atoi(b.reviewFlag)
			b.act("winone.review.action", map[string]any{"stateToken": b.review.Token, "action": b.reviewAction, "flag": flag}, func(json.RawMessage) { b.confirmDelete = false; b.loadReview() })
		}
	})
}

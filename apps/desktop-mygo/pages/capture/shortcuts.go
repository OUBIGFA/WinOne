package capture

import (
	"github.com/egoist/mygo/ui"
	"maps"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
	"winone/internal/captureprefs"
)

func (p *page) canvasShortcuts(c *ui.Context) {
	if p.draft == nil {
		return
	}
	if p.canvasRecorders == nil {
		p.canvasRecorders = map[string]*widgets.ShortcutRecorder{}
	}
	kit.Group(c, icons.ScanLine, captureText("截图内快捷键", "Shortcuts while capturing"),
		captureText("仅在截图中生效；输入文字时保留输入操作。", "Active only on the capture canvas; typing keeps its normal keys."), func() {
			for _, action := range captureprefs.CanvasActions {
				label := captureText(action.Zh, action.En)
				value := captureprefs.CanvasShortcut(p.draft.Value.CanvasShortcuts, action.ID)
				recorder := p.canvasRecorders[action.ID]
				if recorder == nil {
					recorder = widgets.NewShortcutRecorder(p.env)
					recorder.Local = true
					p.canvasRecorders[action.ID] = recorder
				}
				kit.Stack(c, label, "", func() {
					if recorder.View(c, label, &value, p.busy(), false) {
						// Copy before editing: saved/draft must never share a map mutation.
						next := maps.Clone(p.draft.Value.CanvasShortcuts)
						if next == nil {
							next = map[string]string{}
						}
						next[action.ID] = value
						if err := captureprefs.ValidateCanvasShortcuts(next); err != nil {
							p.err = err.Error()
						} else {
							p.draft.Value.CanvasShortcuts = next
							p.err = ""
						}
					}
				})
			}
			kit.Status(c, captureText("Enter 确认，Esc 取消，Delete 删除；方向键微调，Shift 加方向键调整尺寸；Alt 暂时关闭吸附。",
				"Enter confirms, Esc cancels, Delete removes; arrow keys nudge, Shift + arrows resize; Alt bypasses snapping."))
			if kit.Button(c, captureText("恢复截图内快捷键", "Reset canvas shortcuts"), icons.RefreshCw, false).Clicked() {
				p.draft.Value.CanvasShortcuts = nil
			}
			if kit.Button(c, captureText("保存截图内快捷键", "Save canvas shortcuts"), nil, p.saving).Disabled(!p.dirty() || p.busy()).Clicked() {
				p.save()
			}
		})
}

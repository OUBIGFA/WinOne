package capture

import (
	"fmt"
	"maps"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
	"winone/internal/captureprefs"
)

type canvasShortcutGroup struct {
	id, zh, en string
	actionIDs  []string
}

type resolvedCanvasGroup struct {
	id, zh, en string
	actions    []captureprefs.CanvasAction
}

var canvasShortcutGroups = []canvasShortcutGroup{
	{
		id:        "annotation",
		zh:        "标注工具",
		en:        "Annotation tools",
		actionIDs: []string{"rectangle", "ellipse", "arrow", "pen", "cover", "text", "number", "mosaic", "blur", "move", "region"},
	},
	{
		id:        "editing",
		zh:        "编辑与输出",
		en:        "Edit & output",
		actionIDs: []string{"undo", "redo", "copy", "save", "saveAs", "pin"},
	},
	{
		id:        "capture",
		zh:        "识别与录制",
		en:        "Recognition & recording",
		actionIDs: []string{"ocr", "collect", "translate", "scroll", "video", "gif"},
	},
}

func groupedCanvasActions() []resolvedCanvasGroup {
	byID := make(map[string]captureprefs.CanvasAction, len(captureprefs.CanvasActions))
	for _, action := range captureprefs.CanvasActions {
		byID[action.ID] = action
	}
	assigned := make(map[string]bool, len(captureprefs.CanvasActions))
	groups := make([]resolvedCanvasGroup, 0, len(canvasShortcutGroups)+1)
	for _, def := range canvasShortcutGroups {
		var actions []captureprefs.CanvasAction
		for _, id := range def.actionIDs {
			if action, ok := byID[id]; ok {
				actions = append(actions, action)
				assigned[id] = true
			}
		}
		if len(actions) > 0 {
			groups = append(groups, resolvedCanvasGroup{
				id:      def.id,
				zh:      def.zh,
				en:      def.en,
				actions: actions,
			})
		}
	}
	var fallback []captureprefs.CanvasAction
	for _, action := range captureprefs.CanvasActions {
		if !assigned[action.ID] {
			fallback = append(fallback, action)
		}
	}
	if len(fallback) > 0 {
		groups = append(groups, resolvedCanvasGroup{
			id:      "other",
			zh:      "其他",
			en:      "Other",
			actions: fallback,
		})
	}
	return groups
}

func (p *page) canvasRecording() bool {
	for _, recorder := range p.canvasRecorders {
		if recorder.Recording() {
			return true
		}
	}
	return false
}

func (p *page) hasCustomCanvasShortcuts() bool {
	if p.draft == nil {
		return false
	}
	if len(p.draft.Value.CanvasShortcuts) > 0 {
		return true
	}
	for _, action := range captureprefs.CanvasActions {
		if captureprefs.CanvasShortcut(p.draft.Value.CanvasShortcuts, action.ID) != action.Key {
			return true
		}
	}
	return false
}

func canvasGroupSummary(count, custom int) string {
	if i18n.Locale() == i18n.English {
		if custom == 0 {
			return fmt.Sprintf("%d actions", count)
		}
		return fmt.Sprintf("%d actions · %d custom", count, custom)
	}
	if custom == 0 {
		return fmt.Sprintf("%d 项", count)
	}
	return fmt.Sprintf("%d 项 · 已改 %d", count, custom)
}

func (p *page) canvasShortcuts(c *ui.Context) {
	if p.draft == nil {
		return
	}
	if p.canvasRecorders == nil {
		p.canvasRecorders = map[string]*widgets.ShortcutRecorder{}
	}
	groups := groupedCanvasActions()
	for len(p.canvasOpen) < len(groups) {
		p.canvasOpen = append(p.canvasOpen, len(p.canvasOpen) == 0)
	}

	ui.Column(c).FillWidth().Gap(kit.HeadingGap).Children(func() {
		ui.Row(c).FillWidth().GapX(16).GapY(8).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Column(c).Basis(240).Grow(1).Shrink(1).Children(func() {
				kit.SectionHeading(c, icons.ScanLine,
					captureText("截图内快捷键", "Shortcuts while capturing"),
					captureText("仅在截图中生效；输入文字时保留输入操作。", "Active only on the capture canvas; typing keeps its normal keys."))
			})
			if kit.Button(c, captureText("恢复默认", "Reset defaults"), icons.RefreshCw, false).
				Disabled(p.busy() || !p.hasCustomCanvasShortcuts()).Clicked() {
				p.draft.Value.CanvasShortcuts = nil
				p.canvasErr = ""
			}
		})
		if p.canvasErr != "" {
			kit.Notice(c, kit.Failure, "", p.canvasErr, nil)
		}
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			for i, group := range groups {
				customCount := 0
				for _, action := range group.actions {
					if captureprefs.CanvasShortcut(p.draft.Value.CanvasShortcuts, action.ID) != action.Key {
						customCount++
					}
				}
				summary := canvasGroupSummary(len(group.actions), customCount)
				kit.Disclosure(c.Key(group.id), captureText(group.zh, group.en), summary, &p.canvasOpen[i], func() {
					for _, action := range group.actions {
						p.canvasShortcutRow(c, action)
					}
				})
			}
		})
		kit.Help(c, captureText(
			"固定操作：Enter 确认，Esc 取消，Delete 删除；方向键微调，Shift + 方向键调整尺寸；Alt 暂时关闭吸附。",
			"Fixed keys: Enter confirms, Esc cancels, Delete removes; arrow keys nudge, Shift + arrows resize; Alt bypasses snapping."))
	})
}

func (p *page) canvasShortcutRow(c *ui.Context, action captureprefs.CanvasAction) {
	label := captureText(action.Zh, action.En)
	value := captureprefs.CanvasShortcut(p.draft.Value.CanvasShortcuts, action.ID)
	help := ""
	switch {
	case value == "":
		help = captureText("已停用 · 默认 "+action.Key, "Disabled · Default "+action.Key)
	case value != action.Key:
		help = captureText("已修改 · 默认 "+action.Key, "Custom · Default "+action.Key)
	}
	recorder := p.canvasRecorders[action.ID]
	if recorder == nil {
		recorder = widgets.NewShortcutRecorder(p.env)
		recorder.Local = true
		p.canvasRecorders[action.ID] = recorder
	}
	kit.Row(c.Key(action.ID), label, help, func() {
		if recorder.View(c, label, &value, p.busy(), false) {
			next := maps.Clone(p.draft.Value.CanvasShortcuts)
			if next == nil {
				next = map[string]string{}
			}
			if value == action.Key {
				delete(next, action.ID)
			} else {
				next[action.ID] = value
			}
			if len(next) == 0 {
				next = nil
			}
			if err := captureprefs.ValidateCanvasShortcuts(next); err != nil {
				p.canvasErr = err.Error()
			} else {
				p.draft.Value.CanvasShortcuts = next
				p.canvasErr = ""
			}
		}
	})
}

func (p *page) shortcutsRail(c *ui.Context) {
	if !p.env.Rail.Active() && p.draft == nil {
		return
	}
	globalDirty := p.shortcuts != nil && p.shortcuts.Dirty()
	canvasDirty := p.dirty()
	anyDirty := globalDirty || canvasDirty
	anyBusy := p.busy() || (p.shortcuts != nil && p.shortcuts.Busy())
	anyRecording := p.canvasRecording() || (p.shortcuts != nil && p.shortcuts.Recording())
	anySaving := p.saving || (p.shortcuts != nil && p.shortcuts.Saving())
	saveDisabled := !anyDirty || anyBusy || anyRecording || (canvasDirty && !p.validScroll())
	undoDisabled := !anyDirty || anyBusy

	p.env.Rail.Set(
		core.Action{
			Label:    i18n.T("shortcuts.save"),
			Disabled: saveDisabled,
			Busy:     anySaving,
			Run: func() {
				if globalDirty {
					p.shortcuts.Save(c)
				}
				if canvasDirty {
					p.save()
				}
			},
		},
		core.Action{
			Disabled: undoDisabled,
			Run: func() {
				if globalDirty {
					p.shortcuts.Reload()
				}
				if canvasDirty {
					p.discard()
				}
			},
		},
	)
}

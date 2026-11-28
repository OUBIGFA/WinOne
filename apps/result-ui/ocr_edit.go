package resultui

import "github.com/egoist/mygo/ui"

// SnapFind's editable OCR text workflow is adapted to the shared result panel.
// Reading and conversation remain available after copy; Enter in this editor
// inserts a line instead of invoking a search or sending a conversation message.
func (p *Panel) editOriginalButton(c *ui.Context) {
	if p.Source == "selection" || p.Source == "quick-assistant" {
		return
	}
	if iconButton(c, "polish", p.T("编辑识别文字", "Edit recognized text"), p.Busy).Clicked() {
		p.originalDraft = p.Original
		p.editingOriginal, p.focusOriginal = true, true
	}
}

func (p *Panel) originalEditor(c *ui.Context, t *ui.Theme) bool {
	if !p.editingOriginal {
		return false
	}
	ui.Column(c).Key("ocr-editor").FillWidth().Gap(8).Children(func() {
		input := ui.TextArea(c.Key("ocr-source"), &p.originalDraft).Label(p.T("识别文字编辑框", "Recognized text editor")).FillWidth().Height(180).
			FontSize(em(t, sizeBody)).LineHeight(1.5).Padding(10).Border(1, t.Border).Radius(radiusSM)
		if p.focusOriginal {
			input.Focus()
			p.focusOriginal = false
		}
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, p.T("取消修改", "Cancel edit")).Clicked() {
				p.editingOriginal = false
				p.originalDraft = ""
			}
			if ui.PrimaryButton(c, p.T("保存修改", "Save changes")).Disabled(p.Busy || input.Composing()).Clicked() {
				value := p.originalDraft
				p.editingOriginal = false
				p.originalDraft = ""
				if value != p.Original {
					p.Original = value
					if p.Mode == "ocr" {
						p.Result = value
					}
					if p.Actions.EditOriginal != nil {
						p.Actions.EditOriginal(value)
					}
				}
			}
		})
	})
	return true
}

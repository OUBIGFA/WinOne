package resultui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"winone/internal/learning"
	"winone/internal/learning/dictionary"
)

func (p *Panel) studyView(c *ui.Context, t *ui.Theme, s *study) {
	ui.Column(c.Key("learning")).FillWidth().Gap(12).Children(func() {
		// Source and its speech action share one quiet strip. The visual preview
		// is collapsed; speech always receives the complete original selection.
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
				preview := strings.Join(strings.Fields(s.Text), " ")
				if s.SourceOpen {
					preview = p.T("原文", "Source")
				}
				ui.Text(c, preview).Grow(1).MinWidth(0).FontSize(em(t, sizeLabel)).TextColor(t.TextMuted).MaxLines(1).Ellipsis("…")
				label := p.T("展开原文", "Expand source")
				angle := float32(0)
				if s.SourceOpen {
					label = p.T("收起原文", "Collapse source")
					angle = 180
				}
				if rotatedIconButton(c, "chevron", label, angle).Clicked() {
					s.SourceOpen = !s.SourceOpen
				}
				if iconButton(c, "speak", p.T("朗读原文", "Read source"), strings.TrimSpace(s.Text) == "").Clicked() && p.Actions.StudySpeak != nil {
					p.Actions.StudySpeak(s, s.Text, s.Options.Accent)
				}
			})
			if s.SourceOpen {
				p.selectable(c, ui.Column(c).FillWidth(), func() { ui.Text(c, s.Text).FillWidth().FontSize(em(t, sizeBody)).LineHeight(1.6) })
			}
			p.speechFeedback(c, t, "learning-"+s.Text+"-"+s.Options.Accent)
		})
		ui.Divider(c)
		if s.Loading {
			ui.Spinner(c).Size(16, 16).Label(p.T("查询离线词典", "Looking up local dictionary"))
		}
		// Only one compact page is laid out at a time; every row remains available
		// to navigation and Add all, including rows added by the AI vocabulary table.
		compact := s.Kind == learning.Passage || s.Kind == learning.Sentence
		start, end := 0, len(s.Rows)
		if compact && len(s.Rows) > 12 {
			pages := (len(s.Rows) + 11) / 12
			if s.Page >= pages {
				s.Page = pages - 1
			}
			ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
				if ui.Button(c, p.T("上一页", "Previous")).Disabled(s.Page == 0).Clicked() {
					s.Page--
				}
				ui.Text(c, fmt.Sprintf(p.T("第 %d/%d 页 · 共 %d 词条", "Page %d/%d · %d entries"), s.Page+1, pages, len(s.Rows)))
				if ui.Button(c, p.T("下一页", "Next")).Disabled(s.Page+1 >= pages).Clicked() {
					s.Page++
				}
			})
			start = s.Page * 12
			end = min(start+12, len(s.Rows))
		}
		for index := start; index < end; index++ {
			row := &s.Rows[index]
			detail := s.Expanded == index
			ui.Column(c.Key(fmt.Sprintf("word-%d", index))).FillWidth().Gap(6).Children(func() {
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, row.Note.Word).FontSize(em(t, 22)).FontWeight(600).Grow(1).MinWidth(0)
					if s.States.State(row.Note.Word) == learning.Focused {
						ui.Text(c, p.T("重点", "Focus")).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted)
					}
				})
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(8).Children(func() {
					if row.Note.Phonetic != "" {
						ui.Text(c, "/"+strings.Trim(row.Note.Phonetic, "/")+"/").TextColor(t.TextMuted)
					}
					for _, accent := range []string{"uk", "us"} {
						label := p.T("英音", "UK")
						if accent == "us" {
							label = p.T("美音", "US")
						}
						if studyTextAction(c, label, p.T("获取发音：有道，失败时使用微软语音。", "Get pronunciation from Youdao, with Microsoft speech fallback."), false).Clicked() && p.Actions.StudySpeak != nil {
							p.Actions.StudySpeak(s, row.Note.Word, accent)
						}
					}
				})
				if row.Note.Meaning != "" {
					meaning := row.Note.Meaning
					if !detail {
						meaning = strings.SplitN(meaning, "\n", 2)[0]
						if utf8.RuneCountInString(meaning) > 100 {
							meaning = string([]rune(meaning)[:100]) + "…"
						}
					}
					p.selectable(c, ui.Column(c).FillWidth(), func() { ui.Text(c, meaning).FillWidth().FontSize(em(t, sizeBody)).LineHeight(1.6) })
				}
				if detail {
					source := p.T("ECDICT 离线词典", "ECDICT · Offline")
					if row.Entry.Word == "" {
						source = p.T("AI 分析或手动填写 · 建卡前请核对", "AI or manual entry · Check before saving")
					}
					ui.Text(c, source).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted)
					if row.Entry.Definition != "" {
						ui.Text(c, row.Entry.Definition).FillWidth().TextColor(t.TextMuted).LineHeight(1.5)
					}
					if len(row.Entry.Tags) > 0 || row.Entry.Oxford || row.Entry.Collins > 0 {
						ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
							labels := append([]string(nil), row.Entry.Tags...)
							if p.Language == "en-US" {
								for i, label := range labels {
									if english := map[string]string{"中考": "Middle school", "高考": "High school", "四级": "CET-4", "六级": "CET-6", "考研": "Postgraduate", "托福": "TOEFL", "雅思": "IELTS", "GRE": "GRE"}[label]; english != "" {
										labels[i] = english
									}
								}
							}
							if row.Entry.Oxford {
								labels = append(labels, p.T("牛津核心词", "Oxford core"))
							}
							if row.Entry.Collins > 0 {
								labels = append(labels, fmt.Sprintf(p.T("柯林斯 %d 星", "Collins %d stars"), row.Entry.Collins))
							}
							for _, label := range labels {
								ui.Text(c, label).FontSize(em(t, sizeMeta)).Padding(2, 6).Radius(4).Background(t.SurfaceHover)
							}
						})
					}
					if row.Entry.Word != "" {
						ui.Text(c, "BNC "+dictionary.RankLabel(row.Entry.BNC)+" · COCA "+dictionary.RankLabel(row.Entry.COCA)).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted)
					}
					if len(row.Entry.Lemmas) > 0 {
						ui.Text(c, p.T("原型：", "Lemma: ")+strings.Join(row.Entry.Lemmas, ", ")).FillWidth()
					}
					forms := []struct{ key, zh, en string }{{"p", "过去式", "Past"}, {"d", "过去分词", "Past participle"}, {"i", "现在分词", "Present participle"}, {"3", "第三人称单数", "Third person"}, {"s", "复数", "Plural"}, {"r", "比较级", "Comparative"}, {"t", "最高级", "Superlative"}}
					var parts []string
					for _, form := range forms {
						if value := row.Entry.Forms[form.key]; value != "" {
							parts = append(parts, p.T(form.zh, form.en)+": "+value)
						}
					}
					if len(parts) > 0 {
						ui.Text(c, strings.Join(parts, " · ")).FillWidth().TextColor(t.TextMuted).LineHeight(1.5)
					}
				}
				ui.Row(c).FillWidth().Wrap().Gap(12).Children(func() {
					{
						label := p.T("展开详情", "Show details")
						if detail {
							label = p.T("收起详情", "Hide details")
						}
						if studyTextAction(c, label, label, false).Clicked() {
							if detail {
								s.Expanded = -1
							} else {
								s.Expanded = index
							}
						}
					}
					focusLabel := p.T("加强学习", "Focus")
					focusState := learning.Focused
					if s.States.State(row.Note.Word) == learning.Focused {
						focusLabel = p.T("取消加强", "Unfocus")
						focusState = ""
					}
					if studyTextAction(c, focusLabel, p.T("以后在原文中出现时优先学习", "Prioritize this term in future reading"), s.StateWorking).Clicked() && p.Actions.StudyState != nil {
						p.Actions.StudyState(s, row.Note.Word, focusState)
					}
					ignoreLabel := p.T("忽略", "Ignore")
					ignoreState := learning.Ignored
					if s.States.State(row.Note.Word) == learning.Ignored {
						ignoreLabel = p.T("恢复学习", "Restore")
						ignoreState = ""
					}
					if studyTextAction(c, ignoreLabel, p.T("以后不在句子词条中展示，可随时恢复", "Hide from future sentence vocabulary; you can restore it"), s.StateWorking).Clicked() && p.Actions.StudyState != nil {
						p.Actions.StudyState(s, row.Note.Word, ignoreState)
					}
					if row.Added {
						if studyTextAction(c, p.T("已加入 · 查看", "Added · View"), "", s.Working).Clicked() && p.Actions.StudyNavigate != nil {
							p.Actions.StudyNavigate(s, false, row.Note.Word)
						}
					} else if iconButton(c, "book", p.T("加入生词本", "Add to vocabulary"), !s.Connected || s.Working || p.Busy).Tooltip(s.AnkiError).Clicked() {
						s.edit(index)
					}
				})
				if row.Error != "" {
					ui.Row(c).FillWidth().Gap(6).Children(func() {
						ui.Text(c, row.Error).Grow(1).MinWidth(0).TextColor(t.Danger)
						if iconButton(c, "close", p.T("关闭提示", "Dismiss"), false).Clicked() {
							row.Error = ""
						}
					})
				}
				p.speechFeedback(c, t, "learning-"+row.Note.Word+"-uk")
				p.speechFeedback(c, t, "learning-"+row.Note.Word+"-us")
				if s.Draft != nil && s.Editing == index {
					p.studyEditor(c, t, s)
				}
				if index < len(s.Rows)-1 {
					ui.Divider(c).Margin(6, 0)
				}
			})
		}
		for _, entry := range s.Online {
			ui.Column(c.Key("online-" + entry.Word)).FillWidth().Gap(4).Children(func() {
				ui.Text(c, p.T("英英补充 · ", "English dictionary · ")+entry.Word).FontWeight(600)
				for _, meaning := range entry.Meanings {
					for _, definition := range meaning.Definitions {
						ui.Text(c, meaning.PartOfSpeech+" · "+definition.Definition).FillWidth()
						if definition.Example != "" {
							ui.Text(c, definition.Example).FillWidth().TextColor(t.TextMuted)
						}
					}
				}
				// Attribution is returned by the dictionary service; retain it with data.
				ui.Text(c, entry.License.Name+" "+entry.License.URL).FillWidth().FontSize(em(t, sizeMeta)).TextColor(t.TextMuted)
			})
		}
		ui.Collapsible(c, p.T("管理学习词条", "Manage learning terms"), &s.StatesOpen, func() { p.studyStatesView(c, t, s) })
		ui.Collapsible(c, p.T("生词本与学习工具", "Vocabulary and study tools"), &s.ToolsOpen, func() {
			ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
				if ui.Button(c, p.T("检测 Anki", "Check Anki")).Disabled(s.Checking || s.Working).Clicked() && p.Actions.StudyCheck != nil {
					p.Actions.StudyCheck(s)
				}
				if ui.Button(c, p.T("打开生词本", "Open vocabulary")).Disabled(!s.Connected || s.Working).Clicked() && p.Actions.StudyNavigate != nil {
					p.Actions.StudyNavigate(s, false, "")
				}
				if ui.Button(c, p.T("开始复习", "Review")).Disabled(!s.Connected || s.Working).Clicked() && p.Actions.StudyNavigate != nil {
					p.Actions.StudyNavigate(s, true, "")
				}
				if len(s.Rows) > 1 && ui.Button(c, p.T("全部加入", "Add all")).Disabled(!s.Connected || s.Working || p.Busy).Clicked() && p.Actions.StudyAdd != nil {
					indices := []int{}
					for i, row := range s.Rows {
						if !row.Added {
							indices = append(indices, i)
						}
					}
					p.Actions.StudyAdd(s, indices)
				}
				if s.Working && s.cancel != nil && ui.Button(c, p.T("停止建卡", "Stop adding")).Clicked() {
					s.cancel()
				}
			})
			if s.AnkiError != "" {
				ui.Text(c, s.AnkiError).FillWidth().TextColor(t.TextMuted).LineHeight(1.5)
			}
		})
		if s.Working {
			ui.Text(c, fmt.Sprintf(p.T("正在处理 %d/%d", "Processing %d/%d"), s.Progress, s.Total)).TextColor(t.TextMuted)
		}
		feedback := strings.TrimSpace(strings.Join([]string{s.Error, s.Notice}, "\n"))
		s.visibility.observe(feedback)
		if feedback != "" && !s.visibility.dismissed {
			ui.Row(c).FillWidth().Gap(6).Children(func() {
				ui.Text(c, feedback).Grow(1).MinWidth(0).LineHeight(1.5).TextColor(t.TextMuted)
				p.closeFeedback(c, &s.visibility, p.T("关闭学习提示", "Dismiss learning notice"))
			})
		}
		ui.Divider(c)
	})
}

// Secondary commands stay readable and keyboard accessible, but have no
// permanent filled face competing with vocabulary and definitions.
func studyTextAction(c *ui.Context, label, hint string, disabled bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(label).Tooltip(hint).Padding(4, 0).MinHeight(28).TextColor(t.TextMuted).Disabled(disabled).Cursor(ui.CursorPointer).Radius(4)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Text(c, label).FontSize(em(t, sizeMeta)) })
	return b
}
func (p *Panel) studyEditor(c *ui.Context, t *ui.Theme, s *study) {
	ui.Column(c.Key("note-editor")).FillWidth().Gap(8).Children(func() {
		ui.Text(c, p.T("核对并加入生词本", "Review and add card")).FontWeight(600)
		for _, field := range []struct {
			zh, en string
			value  *string
		}{{"词条", "Word or phrase", &s.Draft.Word}, {"音标", "Phonetic", &s.Draft.Phonetic}, {"释义", "Meaning", &s.Draft.Meaning}, {"例句", "Examples", &s.Draft.Example}, {"词根摘要", "Etymology", &s.Draft.Etymology}} {
			ui.Field(c, p.T(field.zh, field.en), func() {
				if field.en == "Word or phrase" || field.en == "Phonetic" {
					ui.TextInput(c, field.value).FillWidth().Disabled(s.Working)
				} else {
					ui.TextArea(c, field.value).Lines(2, 5).Disabled(s.Working)
				}
			}).FillWidth()
		}
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			if ui.Button(c, p.T("确认加入", "Add card")).Disabled(s.Working || strings.TrimSpace(s.Draft.Meaning) == "" || strings.TrimSpace(s.Draft.Word) == "").Clicked() && p.Actions.StudyAdd != nil {
				index := s.Editing
				s.Rows[index].Note = *s.Draft
				s.Draft = nil
				p.Actions.StudyAdd(s, []int{index})
			}
			if ui.Button(c, p.T("取消", "Cancel")).Disabled(s.Working).Clicked() {
				s.Draft = nil
			}
		})
	})
}

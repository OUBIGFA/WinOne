package resultui

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"slices"
	"strings"
	"winone/internal/learning"
)

// Management remains available after ignored terms disappear from new results.
func (p *Panel) studyStatesView(c *ui.Context, t *ui.Theme, s *study) {
	if ui.TextInput(c, &s.StateQuery).FillWidth().Label(p.T("搜索词条", "Search terms")).Changed() {
		s.StatePage = 0
	}
	var terms []string
	for term := range s.States {
		if strings.Contains(term, learning.TermKey(s.StateQuery)) {
			terms = append(terms, term)
		}
	}
	slices.Sort(terms)
	pages := max(1, (len(terms)+19)/20)
	s.StatePage = min(s.StatePage, pages-1)
	for _, term := range terms[min(s.StatePage*20, len(terms)):min((s.StatePage+1)*20, len(terms))] {
		ui.Row(c.Key(term)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, term).Grow(1).MinWidth(0)
			state := p.T("重点", "Focus")
			if s.States[term] == learning.Ignored {
				state = p.T("已忽略", "Ignored")
			}
			ui.Text(c, state).TextColor(t.TextMuted).FontSize(em(t, sizeMeta))
			if iconButton(c, "retry", p.T("恢复普通学习", "Restore normal learning"), s.StateWorking).Clicked() && p.Actions.StudyState != nil {
				p.Actions.StudyState(s, term, "")
			}
		})
	}
	if len(terms) == 0 {
		ui.Text(c, p.T("没有匹配的词条", "No matching terms")).TextColor(t.TextMuted)
	}
	if pages > 1 {
		ui.Row(c).Gap(8).Children(func() {
			if studyTextAction(c, p.T("上一页", "Previous"), "", s.StatePage == 0).Clicked() {
				s.StatePage--
			}
			ui.Text(c, fmt.Sprintf("%d / %d", s.StatePage+1, pages))
			if studyTextAction(c, p.T("下一页", "Next"), "", s.StatePage+1 >= pages).Clicked() {
				s.StatePage++
			}
		})
	}
}

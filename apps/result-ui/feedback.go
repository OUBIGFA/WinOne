package resultui

import "github.com/egoist/mygo/ui"

// Feedback is owned by the panel, not by a transient row index. Dismissal must
// survive moving an error between result/reply areas and must never clear the
// request failure, busy state, current draft or permission behind that message.
type feedbackVisibility struct {
	content   string
	dismissed bool
}

func (f *feedbackVisibility) observe(content string) {
	if f.content != content {
		f.content, f.dismissed = content, false
	}
}
func (p *Panel) closeFeedback(c *ui.Context, f *feedbackVisibility, label string) {
	if contentIconButton(c, label, false, func() { ui.Icon(c, svg("close")).Size(14, 14) }).Size(22, 22).Clicked() {
		f.dismissed = true
	}
}

// A repeated action (for example Copy twice) creates a new notice, even when
// its wording matches the previous one which the owner already dismissed.
func (p *Panel) setNotice(value string) {
	p.Notice = value
	p.noticeVisibility = feedbackVisibility{content: value}
}

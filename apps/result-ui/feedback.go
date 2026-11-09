package resultui

import (
	"github.com/egoist/mygo/ui"
	"time"
)

// Feedback is owned by the panel, not by a transient row index. Dismissal must
// survive moving an error between result/reply areas and must never clear the
// request failure, busy state, current draft or permission behind that message.
type feedbackVisibility struct {
	content   string
	dismissed bool
	until     time.Time
}

func (f *feedbackVisibility) observe(content string) {
	if f.content != content {
		f.content, f.dismissed = content, false
		f.until = time.Time{}
	}
}

// A repeated action (for example Copy twice) creates a new notice, even when
// its wording matches the previous one which the owner already dismissed.
func (p *Panel) setNotice(value string) {
	p.Notice = value
	p.noticeVisibility = feedbackVisibility{content: value}
}

// The UI clock schedules one wake-up; repeated actions restart the interval
// without a background timer that could expire a newer notice.
func (f *feedbackVisibility) update(c *ui.Context) {
	if f.content == "" || f.dismissed {
		return
	}
	if f.until.IsZero() {
		f.until = c.Now().Add(5 * time.Second)
	}
	if remaining := f.until.Sub(c.Now()); remaining > 0 {
		c.After(remaining)
	} else {
		f.dismissed = true
	}
}

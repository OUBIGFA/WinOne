package chat

import (
	"math"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit/icons"
)

// scrollControls are the transcript's scroll controls as the last frame
// laid it out, as Cindy's ChatScrollControls: each user message starts a
// round, the stops are the rounds' tops in the content, and previous and
// next are the rounds around the reader (-1 when none).
type scrollControls struct {
	targets        []float32
	previous, next int
	top, bottom    bool
}

// measureRounds reads where the rounds were in the committed frame. The
// rows and the content column come from one layout, so the stops do not
// depend on how far the reader scrolled since.
func (p *Chat) measureRounds(c *ui.Context, content ui.Handle) scrollControls {
	s := &p.ui.transcript
	n := scrollControls{previous: -1, next: -1, top: s.Y <= 1, bottom: s.MaxY-s.Y <= 1}
	viewport, column := s.Bounds(c), content.Bounds(c)
	if column.H == 0 {
		return n
	}
	offset := viewport.Y - column.Y
	for _, key := range p.ui.items.turnOrder {
		if h := p.ui.items.turnRows[key]; h != nil {
			if b := h.Bounds(c); b.H > 0 {
				n.targets = append(n.targets, max(0, b.Y-column.Y-12))
			}
		}
	}
	current := -1
	if len(n.targets) > 0 {
		current = 0
	}
	for i, top := range n.targets {
		if top <= offset+16 {
			current = i
		}
	}
	// A short last round may never reach the top: the bottom is its.
	if n.bottom && !n.top {
		current = len(n.targets) - 1
	}
	if current > 0 {
		n.previous = current - 1
	}
	if !n.bottom && current+1 < len(n.targets) {
		n.next = current + 1
	}
	return n
}

// navWidth is the room the controls take at the reading area's right,
// their 12 DIP margin and a gap included.
const navWidth = 44

// scrollNav is the column of scroll controls at the right of the reading
// area, centered in its height: to the top, the previous and next round,
// and to the latest. Jumping up stops following the reply; to the latest
// follows it again.
func (p *Chat) scrollNav(c *ui.Context) {
	s, n := &p.ui.transcript, p.ui.items.controls
	height := s.Bounds(c).H
	jump := func(y float32) {
		p.ui.follow, p.ui.pinNext = false, false
		// A followed transcript holds MaxFloat32 for its end, and a glide
		// starts from the container's own offset: settle that on where the
		// reader is for a frame first (startJump).
		s.Y = max(0, min(s.Y, s.MaxY))
		p.ui.items.jump, p.ui.items.jumpWait = &y, true
		p.ui.services.Invalidate()
	}
	ui.Column(c.Key("scroll-nav")).Attach(ui.AnchorTopRight, ui.AnchorTopRight).Top(max(8, (height-118)/2)).Right(12).
		Gap(2).AlignItems(ui.Center).Label(i18n.T("chat.scrollNavigation")).Children(func() {
		button := func(icon *ui.SVG, label string, disabled bool) bool {
			t := c.Theme()
			b := ui.ButtonBase(c).Size(28, 28).Radius(t.Radius).Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label).Disabled(disabled)
			if !disabled && b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			b.Children(func() {
				i := ui.Icon(c, icon).FontSize(16).TextColor(t.TextMuted)
				if disabled {
					i.Opacity(0.4)
				}
			})
			return b.Clicked()
		}
		if button(icons.ArrowUpToLine, i18n.T("navigation.scrollToTop"), n.top) {
			jump(0)
		}
		if button(icons.ChevronUp, i18n.T("chat.previousTurn"), n.previous < 0) {
			jump(n.targets[n.previous])
		}
		if button(icons.ChevronDown, i18n.T("chat.nextTurn"), n.next < 0) {
			jump(n.targets[n.next])
		}
		if button(icons.ArrowDownToLine, i18n.T("navigation.scrollToBottom"), n.bottom) {
			// Following starts again once the glide comes within reach of
			// the end (transcriptView's 80 DIPs).
			jump(math.MaxFloat32)
		}
	})
}

// startJump starts the scroll controls' jump the frame after the click,
// once the container took the reader's offset; it reports a jump on its
// way, during which following neither stops nor starts.
func (p *Chat) startJump() bool {
	s, items := &p.ui.transcript, &p.ui.items
	if items.jump != nil {
		if items.jumpWait {
			items.jumpWait = false
			p.ui.services.Invalidate()
		} else {
			s.ScrollTo(0, *items.jump, ui.Smooth)
			items.jump = nil
		}
	}
	return items.jump != nil || s.Scrolling()
}

// readingPadding is the transcript's right padding: wide enough that the
// controls never cover text when the reading column reaches them.
func (p *Chat) readingPadding(c *ui.Context) float32 {
	width := p.ui.transcript.Bounds(c).W
	gutter := (width - min(width, readingWidth+48)) / 2
	return max(24, navWidth-gutter)
}

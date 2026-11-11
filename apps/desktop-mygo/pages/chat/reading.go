package chat

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Reading sizes of the transcript. Replies are what the page is for, so
// their text is one step above the interface's 14 and has room between
// lines; the work process stays at the interface's size, below it.
const (
	// readingSize is a reply's and a user message's text, in the theme's
	// units of 14: 15 DIPs at Windows' default text size.
	readingSize = 15.0 / 14
	// readingLineHeight suits Chinese and Latin text in one paragraph.
	readingLineHeight = 1.65
	// turnGap separates one round from the next; stepGap, the parts of a
	// round: the message, its work process, its reply.
	turnGap = 32
	stepGap = 10
)

// readingKey identifies a reading theme: what it is derived from.
type readingKey struct {
	dark bool
	size float32
	text ui.Color
}

// useReadingTheme makes the Markdown look of this frame's replies. The
// result window set the vocabulary (docs/working-rules/ui.md): headings
// carried by size and space with no weight above 600, 1.6 or more between
// lines, a quiet code block. Chat keeps syntax colors, as its code is read.
func (p *Chat) useReadingTheme(c *ui.Context) {
	t := c.Theme()
	key := readingKey{dark: t.Dark, size: t.Rem(readingSize), text: t.Text}
	if p.ui.reading != nil && p.ui.readingKey == key {
		return
	}
	th := &markdown.Theme{
		FontSize:            key.size,
		CodeFont:            kit.MonoFont,
		LineHeight:          readingLineHeight,
		CodeLineHeight:      1.55,
		StrongWeight:        600,
		CodeSize:            0.9,
		BlockSpacing:        0.85,
		ItemSpacing:         0.3,
		CodeBlockBackground: t.Text.Alpha(0.035),
		CodeBlockBorder:     kit.Colors(c).Hairline,
		CodeBlockRadius:     t.Radius,
		TableBorder:         kit.Colors(c).Hairline,
		TableHeader:         t.Text.Alpha(0.035),
		TableCellPadding:    [2]float32{0.75, 0.5},
		QuoteBar:            t.Border,
	}
	sizes := [6]float32{1.35, 1.2, 1.08, 1, 1, 0.95}
	for i := range th.Headings {
		th.Headings[i] = markdown.HeadingStyle{Size: sizes[i], Weight: 600, Top: 1.5, Bottom: 0.5}
	}
	th.Headings[5].Color = t.TextMuted
	p.ui.reading, p.ui.readingKey = th, key
}

// codeBlock is a reply's code block: its language, a copy button named in
// WinOne's language (MyGo's own follows the window's locale, which WinOne
// does not set), and the code.
func (p *Chat) codeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	t, pal := c.Theme(), kit.Colors(c)
	box := ui.Column(c).FillWidth().MinWidth(0).Radius(t.Radius).Border(1, pal.Hairline).Background(t.Text.Alpha(0.035)).Clip().
		Role(ui.RoleGroup).Label(b.Language)
	copied := ui.Local(box, "copied", func() time.Time { return time.Time{} })
	box.Children(func() {
		ui.Row(c).FillWidth().MinHeight(32).Padding(0, 4, 0, 12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, b.Language).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).Grow(1).Unselectable()
			icon := icons.Copy
			if done := c.Now().Sub(*copied) < 2*time.Second; done {
				icon = icons.Check
				c.After(time.Second)
			}
			// A block still being written is copied once it closes.
			if messageAction(c, icon, i18n.T("chat.copy")).Disabled(b.Open).Clicked() {
				p.copy(b.Code())
				*copied = c.Now()
			}
		})
		b.Body(c)
	})
}

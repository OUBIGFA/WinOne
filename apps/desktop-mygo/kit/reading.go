package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit/icons"
)

// Reading sizes of long text: a reply in Chat, a Markdown file beside it.
// Such text is what its view is for, so it is one step above the interface's
// 14 and has room between lines.
const (
	// ReadingSize is the text's size in the theme's units of 14: 15 DIPs at
	// Windows' default text size.
	ReadingSize = 15.0 / 14
	// readingLineHeight suits Chinese and Latin text in one paragraph.
	readingLineHeight = 1.65
)

type readingKey struct {
	dark bool
	size float32
	text ui.Color
}

// readingThemes is touched on the UI thread only.
var readingThemes = map[readingKey]*markdown.Theme{}

// ReadingTheme is the Markdown look of long text for the context's theme.
// The result window set the vocabulary (docs/working-rules/ui.md): headings
// carried by size and space with no weight above 600, 1.6 or more between
// lines, a quiet code block. Syntax colors stay, as the code is read.
// The result is shared; do not change it.
func ReadingTheme(c *ui.Context) *markdown.Theme {
	t, p := c.Theme(), Colors(c)
	key := readingKey{dark: t.Dark, size: t.Rem(ReadingSize), text: t.Text}
	if th := readingThemes[key]; th != nil {
		return th
	}
	th := &markdown.Theme{
		FontSize:            key.size,
		CodeFont:            MonoFont,
		LineHeight:          readingLineHeight,
		CodeLineHeight:      1.55,
		StrongWeight:        600,
		CodeSize:            0.9,
		BlockSpacing:        0.85,
		ItemSpacing:         0.3,
		CodeBlockBackground: t.Text.Alpha(0.035),
		CodeBlockBorder:     p.Hairline,
		CodeBlockRadius:     t.Radius,
		TableBorder:         p.Hairline,
		TableHeader:         t.Text.Alpha(0.035),
		TableCellPadding:    [2]float32{0.75, 0.5},
		QuoteBar:            t.Border,
	}
	sizes := [6]float32{1.35, 1.2, 1.08, 1, 1, 0.95}
	for i := range th.Headings {
		th.Headings[i] = markdown.HeadingStyle{Size: sizes[i], Weight: 600, Top: 1.5, Bottom: 0.5}
	}
	th.Headings[5].Color = t.TextMuted
	readingThemes[key] = th
	return th
}

// CodeBlock is a code block of long text, for markdown.Options.CodeBlock:
// its language, a copy button named in WinOne's language (MyGo's own follows
// the window's locale, which WinOne does not set), and the code.
func CodeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	t, p := c.Theme(), Colors(c)
	box := ui.Column(c).FillWidth().MinWidth(0).Radius(t.Radius).Border(1, p.Hairline).Background(t.Text.Alpha(0.035)).Clip().
		Role(ui.RoleGroup).Label(b.Language)
	copied := ui.Local(box, "copied", func() time.Time { return time.Time{} })
	box.Children(func() {
		ui.Row(c).FillWidth().MinHeight(32).Padding(0, 4, 0, 12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, b.Language).FontSize(CaptionSize(c)).TextColor(t.TextMuted).Grow(1).Unselectable()
			icon := icons.Copy
			if c.Now().Sub(*copied) < 2*time.Second {
				icon = icons.Check
				c.After(time.Second)
			}
			label := i18n.T("chat.copy")
			// A block still being written is copied once it closes.
			button := ui.ButtonBase(c).Size(24, 24).Radius(t.Radius).Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label).Disabled(b.Open)
			color := t.TextMuted
			if button.Hovered() || button.FocusVisible() {
				button.Background(t.SurfaceHover)
				color = t.Text
			}
			button.Children(func() { ui.Icon(c, icon).FontSize(13).TextColor(color) })
			if button.Clicked() {
				c.Services().WriteClipboard(b.Code())
				*copied = c.Now()
			}
		})
		b.Body(c)
	})
}

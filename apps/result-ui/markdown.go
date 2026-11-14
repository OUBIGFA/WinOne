package resultui

import (
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// Replies and results are MyGo's own Markdown view (ui/markdown): CommonMark
// and GitHub's extensions, streamed text parsed incrementally, and one
// selection per message copied as text, HTML or Markdown. WinOne adds only
// what its rules require: code that wraps instead of scrolling sideways,
// tables that fit the reading column, image links instead of automatic
// requests, its own link policy and copy button, and local Mermaid.

type markdownRole uint8

const (
	markdownReply markdownRole = iota
	markdownResult
)

// markdownViews are the options the views of one panel share, one for
// each role and streaming state, and the themes they show, kept by value
// so a theme compares equal from frame to frame and the views keep their
// laid-out text.
type markdownViews struct {
	options [2][2]*markdown.Options
	themes  map[markdownThemeKey]*markdown.Theme
}

type markdownThemeKey struct {
	role         markdownRole
	dark, strong bool
	size         float32
	text         ui.Color
}

func (p *Panel) markdownOptions(c *ui.Context, role markdownRole, working bool) *markdown.Options {
	streaming := 0
	if working {
		streaming = 1
	}
	o := p.markdown.options[role][streaming]
	if o == nil {
		o = &markdown.Options{
			WrapCode:  true,
			OnLink:    p.openLink,
			CodeBlock: p.codeBlock,
			Block:     p.markdownBlock,
		}
		p.markdown.options[role][streaming] = o
	}
	o.Theme = p.markdownTheme(c, role)
	// Words come in at an even pace and fade in, as the chat apps show
	// replies; with less motion asked of the desktop they show at once.
	o.Reveal = nil
	if c.Preferences().ReduceMotion {
		o.Reveal = &markdown.Reveal{Unit: markdown.AtOnce, Fade: -1}
	}
	return o
}

func (p *Panel) markdownTheme(c *ui.Context, role markdownRole) *markdown.Theme {
	t := c.Theme()
	size := em(t, sizeBody)
	if role == markdownResult {
		size = em(t, sizeResult)
	}
	key := markdownThemeKey{role: role, dark: t.Dark, strong: c.Preferences().HighContrast, size: size, text: t.Text}
	if th := p.markdown.themes[key]; th != nil {
		return th
	}
	if p.markdown.themes == nil {
		p.markdown.themes = map[markdownThemeKey]*markdown.Theme{}
	}
	code := neutralCodeColors(t)
	th := &markdown.Theme{
		FontSize:            size,
		Font:                systemFont,
		CodeFont:            monoFont,
		LineHeight:          1.6,
		CodeLineHeight:      1.5,
		Link:                t.Text,
		StrongWeight:        600,
		CodeBackground:      t.Text.Alpha(0.07),
		BlockSpacing:        0.7,
		CodeBlockBackground: t.Text.Alpha(0.035),
		CodeBlockBorder:     t.Border,
		CodeBlockRadius:     8,
		CodeColors:          code,
		TableBorder:         t.Border,
	}
	// Size and spacing carry the headings; no weight above 600.
	sizes := [6]float32{1.3, 1.18, 1.08, 1, 1, 0.95}
	for i := range th.Headings {
		th.Headings[i] = markdown.HeadingStyle{Size: sizes[i], Weight: 600, Top: 1.2, Bottom: 0.45}
	}
	th.Headings[5].Color = t.TextMuted
	p.markdown.themes[key] = th
	return th
}

// neutralCodeColors keeps code in the interface's grays: comments and
// punctuation recede, everything else reads as text. Only changes of a
// diff and invalid code keep their semantic colors.
func neutralCodeColors(t *ui.Theme) *editor.Theme {
	ct := editor.DefaultTheme(t)
	for k := range ct.Tokens {
		ct.Tokens[k] = editor.TokenStyle{}
	}
	quiet := t.Text.Mix(t.TextMuted, 0.55)
	ct.Tokens[editor.TokenComment] = editor.TokenStyle{Color: t.TextMuted, Italic: true}
	ct.Tokens[editor.TokenMeta] = editor.TokenStyle{Color: t.TextMuted}
	ct.Tokens[editor.TokenPunctuation] = editor.TokenStyle{Color: quiet}
	ct.Tokens[editor.TokenOperator] = editor.TokenStyle{Color: quiet}
	ct.Tokens[editor.TokenString] = editor.TokenStyle{Color: quiet}
	ct.Tokens[editor.TokenInserted] = editor.TokenStyle{Color: t.Success}
	ct.Tokens[editor.TokenDeleted] = editor.TokenStyle{Color: t.Danger}
	ct.Tokens[editor.TokenInvalid] = editor.TokenStyle{Color: t.Danger}
	return ct
}

// markdownView shows raw under key, streaming while working, and notes
// its selection for the selection assistant.
func (p *Panel) markdownView(c *ui.Context, key string, raw string, role markdownRole, working bool) {
	ui.Column(c).Key(key).FillWidth().Children(func() {
		o := p.markdownOptions(c, role, working)
		var root ui.Element
		if working {
			root = markdown.Stream(c, raw, o)
		} else {
			root = markdown.View(c, raw, o)
		}
		root.FillWidth()
		p.noteSelection(root)
	})
}

// openLink opens only web addresses with a host and no credentials; other
// schemes, local paths and user info do nothing, as before.
func (p *Panel) openLink(destination string) {
	if safeMarkdownLink(destination) {
		p.services.OpenURL(destination)
	}
}

func safeMarkdownLink(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil && ui.SafeToOpen(value)
}

// codeBlock shows its language and WinOne's copy button above the code,
// which wraps rather than scrolling sideways (Options.WrapCode).
func (p *Panel) codeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	t := c.Theme()
	ui.Column(c).FillWidth().Border(1, t.Border).Radius(8).Background(t.Text.Alpha(0.035)).Clip().Role(ui.RoleGroup).Label(p.T("代码", "Code") + " " + b.Language).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(2, 2, 0, 12).Children(func() {
			ui.Text(c, b.Language).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine().Unselectable()
			if iconButton(c, "copy", p.T("复制代码", "Copy code"), false).Clicked() && p.Actions.Copy != nil {
				p.Actions.Copy(b.Code())
			}
		})
		b.Body(c)
	})
}

// markdownBlock adapts blocks whose default presentation would scroll
// horizontally, load pictures, or omit WinOne's local diagram renderer.
func (p *Panel) markdownBlock(c *ui.Context, d *parser.Document, i int) bool {
	switch b := d.Block(i); b.Kind {
	case parser.CodeBlock:
		if b.Language() != "mermaid" {
			return false
		}
		root := ui.Column(c).FillWidth()
		ready := !d.Streaming() || !b.Open
		if !ready {
			// Stream intentionally withholds a final marker-only line until
			// a newline arrives. A closed Mermaid fence is already usable:
			// ask the same parser without tail healing, never guess the fence
			// grammar. Cache per block so idle frames do not reparse it.
			check := ui.Local(root, "fence", func() fenceCheck { return fenceCheck{} })
			if check.source != d.Source() {
				check.source, check.closed = d.Source(), false
				last := strings.TrimSpace(check.source[strings.LastIndex(check.source, "\n")+1:])
				if strings.HasPrefix(last, "```") || strings.HasPrefix(last, "~~~") {
					complete := parser.Parse(check.source)
					for j := 1; j < complete.Len(); j++ {
						block := complete.Block(j)
						if block.Start == b.Start && block.Kind == parser.CodeBlock && !block.Open {
							check.closed = true
							break
						}
					}
				}
			}
			ready = check.closed
		}
		root.Children(func() { p.renderDiagram(c, c.Theme(), d.Lines(i), ready) })
		return true
	case parser.MathBlock:
		// MyGo has no formula renderer; preserve TeX as readable, selectable
		// source, but replace its default horizontal scroller with wrapping.
		ui.Text(c, strings.TrimRight(d.Lines(i), "\n")).FillWidth().Font(monoFont).TextColor(c.Theme().TextMuted)
		return true
	case parser.Table:
		p.markdownTable(c, d, i)
		return true
	case parser.Paragraph:
		in := d.Inlines(i)
		for k := range in {
			if in[k].Kind == parser.Image {
				p.inlineParagraph(c, d, i, false)
				return true
			}
		}
	}
	return false
}

type fenceCheck struct {
	source string
	closed bool
}

// markdownTable fits a table into the reading column: each column takes a
// share of the width by its widest cell, cells wrap, and nothing scrolls
// sideways (WinOne forbids horizontal scrolling).
func (p *Panel) markdownTable(c *ui.Context, d *parser.Document, i int) {
	t := c.Theme()
	table := d.Block(i)
	columns := len(table.Align)
	if columns == 0 {
		return
	}
	type row struct {
		cells  []int
		header bool
	}
	var rows []row
	widths := make([]float32, columns)
	for r := i + 1; r < table.Next; r = d.Block(r).Next {
		line := row{header: d.Block(r).Header}
		for cell := r + 1; cell < d.Block(r).Next && len(line.cells) < columns; cell = d.Block(cell).Next {
			line.cells = append(line.cells, cell)
			width, _ := c.MeasureText(0, ui.Span{Text: parser.PlainText(d.Inlines(cell))})
			widths[len(line.cells)-1] = max(widths[len(line.cells)-1], width)
		}
		rows = append(rows, line)
	}
	tracks := make([]ui.Track, columns)
	natural := float32(0)
	for k, width := range widths {
		// A share by content, bounded so one long cell cannot starve the rest.
		share := min(max(width, em(t, 40)), em(t, 320))
		tracks[k] = ui.Fr(share)
		natural += share + 2*em(t, 10)
	}
	ui.Grid(c).ColumnTracks(tracks...).MaxWidth(natural).Border(1, t.Border).Radius(6).Clip().Role(ui.RoleTable).Children(func() {
		for index, line := range rows {
			for k := 0; k < columns; k++ {
				cell := ui.Column(c).MinWidth(0).Padding(6, 10)
				if line.header {
					cell.Background(t.Text.Alpha(0.04)).Role(ui.RoleColumnHeader)
				} else {
					cell.Role(ui.RoleCell)
				}
				if index > 0 || k > 0 {
					top, left := float32(0), float32(0)
					if index > 0 {
						top = 1
					}
					if k > 0 {
						left = 1
					}
					cell.BorderWidth(top, 0, 0, left).BorderColor(t.Border)
				}
				cell.Children(func() {
					if k >= len(line.cells) {
						return
					}
					align := ui.Start
					switch table.Align[k] {
					case parser.AlignCenter:
						align = ui.Center
					case parser.AlignRight:
						align = ui.End
					}
					p.inlineParagraph(c, d, line.cells[k], line.header).TextAlign(align)
				})
			}
		}
	})
}

// inlineParagraph builds the inline content of block i as one paragraph.
func (p *Panel) inlineParagraph(c *ui.Context, d *parser.Document, i int, strong bool) ui.Element {
	in := d.Inlines(i)
	paragraph := ui.RichText(c).FillWidth()
	if strong {
		paragraph.FontWeight(600)
	}
	return paragraph.Children(func() { p.inlines(c, in, 0, len(in)) })
}

// inlines builds in[from:to] as text elements of the paragraph being built:
// emphasis, code, links and images become links only to the addresses
// openLink accepts. Raw HTML stays text.
func (p *Panel) inlines(c *ui.Context, in []parser.Inline, from, to int) {
	t := c.Theme()
	for k := from; k < to; {
		n := in[k]
		next := max(n.Next, k+1)
		switch n.Kind {
		case parser.Text, parser.Mention, parser.Reference:
			// A result has no GitHub repository context to resolve these;
			// preserve the author's spelling instead of inventing a link.
			ui.Text(c, n.Text)
		case parser.SoftBreak:
			ui.Text(c, " ")
		case parser.LineBreak:
			ui.Text(c, "\n")
		case parser.Code:
			ui.Text(c, n.Text).Font(monoFont).Background(t.Text.Alpha(0.07))
		case parser.Math:
			ui.Text(c, n.Text).Font(monoFont).TextColor(t.TextMuted)
		case parser.HTML:
			if tag := strings.ToLower(strings.TrimSpace(n.Text)); tag == "<br>" || tag == "<br/>" || tag == "<br />" {
				ui.Text(c, "\n")
			} else {
				ui.Text(c, n.Text)
			}
		case parser.Emphasis:
			ui.RichText(c).Italic().Children(func() { p.inlines(c, in, k+1, next) })
		case parser.Strong:
			ui.RichText(c).FontWeight(600).Children(func() { p.inlines(c, in, k+1, next) })
		case parser.Strikethrough:
			ui.RichText(c).Strikethrough().Children(func() { p.inlines(c, in, k+1, next) })
		case parser.Link, parser.Image:
			// Pictures are never fetched: an image is a link to its address,
			// named by its description.
			label := parser.PlainText(in[k+1 : next])
			if n.Kind == parser.Image && label == "" {
				label = p.T("图片", "Image")
			}
			if !n.Pending && safeMarkdownLink(n.Dest) {
				ui.Link(c, label, n.Dest).Description(n.Dest)
			} else {
				ui.Text(c, label)
			}
		case parser.FootnoteRef:
			ui.Text(c, "[^"+n.Text+"]")
		}
		k = next
	}
}

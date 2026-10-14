package resultui

import (
	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"html"
	"net/url"
	"strconv"
	"strings"
)

var markdownParser = goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()

// Reuse each message's parsed document across theme, hover and scroll frames.
type markdownDocument struct {
	raw    string
	source []byte
	root   ast.Node
	tables map[ast.Node]*ui.ListState
}

func (p *Panel) markdown(c *ui.Context, t *ui.Theme, raw string, cache **markdownDocument, working bool) {
	if *cache == nil || (*cache).raw != raw {
		source := []byte(raw)
		*cache = &markdownDocument{raw: raw, source: source, root: markdownParser.Parse(gmtext.NewReader(source)), tables: map[ast.Node]*ui.ListState{}}
	}
	doc := *cache
	ui.Column(c).FillWidth().Gap(8).Children(func() { p.markdownBlocks(c, t, doc, doc.root, working) })
}
func (p *Panel) markdownBlocks(c *ui.Context, t *ui.Theme, doc *markdownDocument, parent ast.Node, working bool) {
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		switch n := node.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			p.markdownInline(c, t, doc, node).FillWidth().Selectable().FontSize(t.FontSize + 1).LineHeight(1.55)
		case *ast.Heading:
			sizes := []float32{22, 20, 18, 17, 16, 15}
			heading:=p.markdownInline(c, t, doc, node).FillWidth().Selectable().FontSize(sizes[n.Level-1]).FontWeight(600)
			// The reading stack owns the entry gap. A leading heading must not
			// silently add a second top margin to thinking/answer separation.
			if node!=parent.FirstChild(){heading.Margin(4,0,4,0)}else{heading.Margin(0,0,4,0)}
		case *ast.List:
			index := n.Start
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				for item := n.FirstChild(); item != nil; item = item.NextSibling() {
					marker := "•"
					if n.IsOrdered() {
						marker = strconv.Itoa(index) + "."
					}
					index++
					ui.Row(c).AlignItems(ui.Start).Gap(8).Children(func() {
						ui.Text(c, marker).Width(20).TextColor(t.TextMuted)
						ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() { p.markdownBlocks(c, t, doc, item, working) })
					})
				}
			})
		case *ast.Blockquote:
			ui.Row(c).Gap(10).Children(func() {
				ui.Box(c).Width(2).AlignSelf(ui.Stretch).Background(t.Border)
				ui.Column(c).Grow(1).MinWidth(0).Gap(6).TextColor(t.TextMuted).Children(func() { p.markdownBlocks(c, t, doc, node, working) })
			})
		case *ast.ThematicBreak:
			ui.Divider(c).Margin(4, 0)
		case *ast.FencedCodeBlock:
			source := string(n.Lines().Value(doc.source))
			language := string(n.Language(doc.source))
			if strings.EqualFold(language, "mermaid") {
				closed := false
				if count := n.Lines().Len(); count > 0 {
					rest := strings.TrimLeft(string(doc.source[n.Lines().At(count-1).Stop:]), " \t\r\n")
					closed = strings.HasPrefix(rest, "```") || strings.HasPrefix(rest, "~~~")
				}
				p.renderDiagram(c, t, source, !working || closed)
			} else {
				p.codeBlock(c, t, language, source)
			}
		case *ast.CodeBlock:
			p.codeBlock(c, t, "", string(n.Lines().Value(doc.source)))
		case *extast.Table:
			p.markdownTable(c, t, doc, n)
		case *ast.HTMLBlock:
			// HTML stays inert; it never enters a browser or script interpreter.
			ui.Text(c, string(n.Lines().Value(doc.source))).FillWidth().Selectable().TextColor(t.TextMuted)
		default:
			if node.HasChildren() {
				p.markdownBlocks(c, t, doc, node, working)
			}
		}
	}
}
func (p *Panel) markdownInline(c *ui.Context, t *ui.Theme, doc *markdownDocument, node ast.Node) *ui.Element {
	return ui.RichText(c).Children(func() { p.inlineChildren(c, t, doc, node) })
}
func (p *Panel) inlineChildren(c *ui.Context, t *ui.Theme, doc *markdownDocument, parent ast.Node) {
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			text := n.Segment.Value(doc.source)
			value := string(text)
			if !n.IsRaw() {
				value = html.UnescapeString(string(util.UnescapePunctuations(text)))
			}
			if n.HardLineBreak() {
				value += "\n"
			} else if n.SoftLineBreak() {
				value += "\n"
			}
			ui.Text(c, value)
		case *ast.String:
			ui.Text(c, string(n.Value))
		case *ast.Emphasis:
			text := p.markdownInline(c, t, doc, n)
			if n.Level == 2 {
				text.FontWeight(600)
			} else {
				text.Italic()
			}
		case *ast.CodeSpan:
			p.markdownInline(c, t, doc, n).Font("Consolas").Background(t.SurfaceHover)
		case *extast.Strikethrough:
			p.markdownInline(c, t, doc, n).Strikethrough()
		case *ast.Link:
			label := html.UnescapeString(string(n.Text(doc.source)))
			destination := string(n.Destination)
			if safeMarkdownLink(destination) {
				ui.Link(c, label, destination).Tooltip(destination)
			} else {
				ui.Text(c, label)
			}
		case *ast.AutoLink:
			label, destination := string(n.Label(doc.source)), string(n.URL(doc.source))
			if safeMarkdownLink(destination) {
				ui.Link(c, label, destination)
			} else {
				ui.Text(c, label)
			}
		case *ast.Image:
			// Images from a reply are explicit links, not automatic requests to third
			// parties or local files. Mermaid is rendered locally by the diagram worker.
			label := string(n.Text(doc.source))
			if label == "" {
				label = p.T("图片", "Image")
			}
			destination := string(n.Destination)
			if safeMarkdownLink(destination) {
				ui.Link(c, label, destination).Tooltip(destination)
			} else {
				ui.Text(c, label)
			}
		case *extast.TaskCheckBox:
			if n.IsChecked {
				ui.Text(c, "☑ ")
			} else {
				ui.Text(c, "☐ ")
			}
		case *ast.RawHTML:
			text := string(n.Text(doc.source))
			if strings.EqualFold(strings.TrimSpace(text), "<br>") || strings.EqualFold(strings.TrimSpace(text), "<br/>") {
				ui.Text(c, "\n")
			} else {
				ui.Text(c, text)
			}
		default:
			if child.HasChildren() {
				p.inlineChildren(c, t, doc, child)
			}
		}
	}
}
func safeMarkdownLink(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil
}
func (p *Panel) codeBlock(c *ui.Context, t *ui.Theme, language, source string) {
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, language).TextColor(t.TextMuted).Grow(1)
			if iconButton(c, "copy", p.T("复制代码", "Copy code")).Clicked() && p.Actions.Copy != nil {
				p.Actions.Copy(source)
			}
		})
		height := min(260, float32(strings.Count(source, "\n")+1)*20+16)
		ui.ScrollBoth(c).Height(height).FillWidth().Padding(8).Radius(6).Background(t.SurfaceHover).Children(func() {
			ui.Text(c, strings.TrimSuffix(source, "\n")).NoWrap().Font("Consolas").FontSize(14).LineHeight(1.45).Selectable()
		})
	})
}
func (p *Panel) markdownTable(c *ui.Context, t *ui.Theme, doc *markdownDocument, node *extast.Table) {
	var columns []ui.TableColumn
	var rows [][]ast.Node
	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		cells := []ast.Node{}
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, cell)
		}
		if _, header := row.(*extast.TableHeader); header {
			for i, cell := range cells {
				columns = append(columns, ui.TableColumn{ID: strconv.Itoa(i), Title: string(cell.Text(doc.source)), MinWidth: 80})
			}
		} else {
			rows = append(rows, cells)
		}
	}
	state := doc.tables[node]
	if state == nil {
		state = &ui.ListState{}
		doc.tables[node] = state
	}
	ui.Table(c, state, columns, len(rows), func(row, col int) {
		if col < len(rows[row]) {
			p.markdownInline(c, t, doc, rows[row][col]).Selectable()
		}
	}).FillWidth().Height(min(280, float32(len(rows)+1)*36+4))
}

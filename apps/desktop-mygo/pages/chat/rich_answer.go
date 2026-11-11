package chat

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"winone/desktopmygo/chat/workspace"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/chat/richanswer"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	resultui "winone/resultui"
)

// Native view of Cindy's declarative answers (RichAnswer.tsx). No generated markup
// or script exists here: every node maps to a native control, colors stay roles,
// and a send button only offers visible text the user clicks to submit. Tracked as
// chat-rich-answer in third_party/cindy/native-ports.json.

// richState is what a fragment keeps: its parser, and the user's values, which live
// on this device only and are never model context.
type richFragment struct{ stream richanswer.Stream }

// richMemory is the view state of every answer, kept beside Chat's preferences.
type richMemory struct {
	loaded    bool
	path      string
	fragments map[string]*richFragment
	Values    map[string]map[string]any `json:"values"`
	Sent      map[string]string         `json:"sent"`
	order     []string
	sending   map[string]bool
	failed    map[string]bool
}

const richMemoryEntries = 256

func (m *richMemory) load(directory string) {
	if m.loaded {
		return
	}
	m.loaded = true
	m.path = filepath.Join(directory, "chat-rich-answers.json")
	m.fragments, m.sending, m.failed = map[string]*richFragment{}, map[string]bool{}, map[string]bool{}
	// View-state recovery must not prevent reading the answer.
	if raw, err := os.ReadFile(m.path); err == nil && len(raw) <= 524288 {
		_ = json.Unmarshal(raw, m)
	}
	if m.Values == nil {
		m.Values = map[string]map[string]any{}
	}
	if m.Sent == nil {
		m.Sent = map[string]string{}
	}
	for key := range m.Values {
		m.order = append(m.order, key)
	}
}

func (m *richMemory) save() {
	for len(m.order) > richMemoryEntries {
		delete(m.Values, m.order[0])
		m.order = m.order[1:]
	}
	if raw, err := json.Marshal(m); err == nil {
		// A failed write keeps the in-memory values; the next change tries again.
		_ = os.WriteFile(m.path, raw, 0o600)
	}
}

func (m *richMemory) set(key string, patch map[string]any) {
	values := m.Values[key]
	if values == nil {
		values = map[string]any{}
		m.Values[key] = values
		m.order = append(m.order, key)
	}
	for name, value := range patch {
		values[name] = value
	}
	if raw, err := json.Marshal(values); err == nil && len(raw) <= 65536 {
		m.save()
	}
}

// answerCodeBlocks are the code blocks an answer renders rather than lists.
func (p *Chat) answerCodeBlocks(o *markdown.Options, key string) {
	o.Code = map[string]func(c *ui.Context, b *markdown.CodeBlockView){
		richanswer.Language: func(c *ui.Context, b *markdown.CodeBlockView) { p.richAnswer(c, key, b) },
		"mermaid":           func(c *ui.Context, b *markdown.CodeBlockView) { p.mermaidBlock(c, key, b) },
	}
}

// richAnswer shows one fence. The user's values and identities survive content
// patches, because the stream reads only what was appended.
func (p *Chat) richAnswer(c *ui.Context, key string, b *markdown.CodeBlockView) {
	s := &p.richMemory
	s.load(p.env.Directory)
	stateKey := p.activeID + ":" + key + ":" + strconv.Itoa(b.Index)
	fragment := s.fragments[stateKey]
	if fragment == nil {
		fragment = &richFragment{}
		s.fragments[stateKey] = fragment
	}
	complete := !b.Open
	doc := fragment.stream.Update(b.Code(), complete)
	values := richanswer.Resolved(doc, s.Values[stateKey])
	view := &richView{p: p, doc: doc, values: values, stateKey: stateKey, complete: complete}
	root := ui.Column(c.Key(stateKey)).FillWidth().MinWidth(0).Gap(12)
	width := ui.Local(root, "rich-answer-width", func() ui.Handle { return ui.Handle{} })
	root.Bind(width).Children(func() {
		view.width = width.Bounds(c).W
		if view.width <= 0 {
			c.Invalidate() // Publish actual parent allocation, not a guessed window width.
		}
		for _, id := range doc.Roots {
			view.node(c, id)
		}
		if doc.Issues > 0 {
			kit.Notice(c, kit.Caution, "", botT("richAnswer.incomplete"), nil)
		}
	})
}

type richView struct {
	p        *Chat
	doc      *richanswer.Document
	values   map[string]any
	stateKey string
	complete bool
	width    float32
}

func (v *richView) caption(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(kit.HelpSize(c)).TextColor(c.Theme().TextMuted).MinWidth(0)
}

func (v *richView) change(key string, value any) {
	v.p.richMemory.set(v.stateKey, map[string]any{key: value})
}

func richRoleColor(t *ui.Theme, role string) ui.Color {
	switch role {
	case "secondary", "tertiary":
		return t.TextMuted
	case "success":
		return t.Success
	case "warning":
		return t.Warning
	case "danger":
		return t.Danger
	}
	return t.Text
}

// applyRichStyle puts a node's validated style on its element. Fixed widths never
// exceed the available width: the same answer is read on phones.
func applyRichStyle(c *ui.Context, e ui.Element, style richanswer.Style, availableWidth float32) {
	for key, value := range style.Lengths {
		size := float32(value)
		switch key {
		case "width":
			e.Width(size).MaxWidthPercent(100)
		case "maxWidth":
			e.MaxWidth(size)
		case "minWidth":
			// CSS min-width outranks max-width. Port Cindy's
			// min(author-width, 100%) rather than hoping MaxWidth caps it.
			e.MinWidth(min(size, max(0, availableWidth))).MaxWidthPercent(100)
		case "height":
			e.Height(size)
		case "minHeight":
			e.MinHeight(size)
		case "maxHeight":
			e.MaxHeight(size)
		case "padding":
			e.Padding(size)
		case "gap":
			e.Gap(size)
		case "borderRadius":
			e.Radius(size)
		case "fontSize":
			e.FontSize(size)
		case "lineHeight":
			// Up to 4 is a multiplier of the font size; above it, pixels.
			if value <= 4 {
				e.LineHeight(size)
			}
		case "letterSpacing":
			e.LetterSpacing(size)
		case "flexGrow":
			e.Grow(size)
		case "flexShrink":
			e.Shrink(size)
		}
	}
	lengths := style.Lengths
	if lengths["paddingTop"] != 0 || lengths["paddingRight"] != 0 || lengths["paddingBottom"] != 0 || lengths["paddingLeft"] != 0 {
		e.Padding(float32(lengths["paddingTop"]), float32(lengths["paddingRight"]), float32(lengths["paddingBottom"]), float32(lengths["paddingLeft"]))
	}
	if lengths["marginTop"] != 0 || lengths["marginBottom"] != 0 {
		e.Margin(float32(lengths["marginTop"]), 0, float32(lengths["marginBottom"]), 0)
	}
	for key, value := range style.Percent {
		size := float32(value)
		switch key {
		case "width":
			e.WidthPercent(min(100, size)).MaxWidthPercent(100)
		case "maxWidth":
			e.MaxWidthPercent(size)
		case "minWidth":
			e.MinWidthPercent(min(100, size))
		case "height":
			e.HeightPercent(size)
		}
	}
	if style.Color != "" {
		e.TextColor(richRoleColor(c.Theme(), style.Color))
	}
	if style.Opacity != nil {
		e.Opacity(float32(*style.Opacity))
	}
	if style.Border {
		e.Border(1, c.Theme().Border)
	}
	if weight, ok := style.Text["fontWeight"]; ok {
		switch weight {
		case "bold":
			e.FontWeight(700)
		case "normal":
			e.FontWeight(400)
		default:
			if n, err := strconv.Atoi(weight); err == nil {
				e.FontWeight(n)
			}
		}
	}
	switch style.Text["alignItems"] {
	case "flex-start":
		e.AlignItems(ui.Start)
	case "center":
		e.AlignItems(ui.Center)
	case "flex-end":
		e.AlignItems(ui.End)
	case "stretch":
		e.AlignItems(ui.Stretch)
	}
	switch style.Text["justifyContent"] {
	case "flex-start":
		e.Justify(ui.Start)
	case "center":
		e.Justify(ui.Center)
	case "flex-end":
		e.Justify(ui.End)
	case "space-between":
		e.Justify(ui.SpaceBetween)
	}
	if style.Text["flexWrap"] == "wrap" {
		e.Wrap()
	}
	switch style.Text["textAlign"] {
	case "center":
		e.TextAlign(ui.Center)
	case "right":
		e.TextAlign(ui.End)
	}
	if style.Text["fontStyle"] == "italic" {
		e.Italic()
	}
	switch style.Text["textDecorationLine"] {
	case "underline":
		e.Underline()
	case "line-through":
		e.Strikethrough()
	}
}

// node draws one component with its style.
func (v *richView) node(c *ui.Context, id string) {
	node := v.doc.Nodes[id]
	if node == nil {
		return
	}
	if richanswer.CalculationUnavailable(node, v.values) {
		text := "…"
		if v.complete {
			kit.Status(c.Key(id), botT("richAnswer.calculationUnavailable"))
		} else {
			v.caption(c.Key(id), text)
		}
		return
	}
	style := richanswer.ReadStyle(node)
	c = c.Key(id)
	applyRichStyle(c, v.content(c, node), style, v.width)
}

func (v *richView) children(c *ui.Context, node *richanswer.Node) {
	for _, child := range node.Children {
		v.node(c, child)
	}
}

func (v *richView) content(c *ui.Context, node *richanswer.Node) ui.Element {
	t := c.Theme()
	label := richanswer.String(node, "label", "")
	bind := richanswer.String(node, "bind", node.ID)
	switch node.Kind {
	case "image", "video", "audio":
		return v.media(c, node)
	case "markdown", "code", "math", "diagram":
		source, _ := richanswer.Markdown(node)
		root := ui.Column(c).FillWidth().Gap(4)
		root.Children(func() {
			key := v.stateKey + ":" + node.ID
			o := v.p.richMarkdownOptions(key)
			markdown.View(c, quoteSpacedImagePaths(source), o).FillWidth()
			kit.InlineError(c, v.p.ui.items.errors["image:rich:"+key])
			if caption := richanswer.String(node, "caption", ""); caption != "" {
				v.caption(c, caption)
			}
		})
		return root
	case "svg":
		return v.vector(c, node, label)
	case "heading":
		return ui.Text(c, richanswer.Label(node, v.values)).FontWeight(600).FontSize(kit.HelpSize(c) + 3).MinWidth(0)
	case "text":
		return ui.Text(c, richanswer.Label(node, v.values)).LineHeight(1.5).Selectable().MinWidth(0)
	case "icon":
		root := ui.Row(c).FillWidth().Gap(10)
		root.Children(func() {
			ui.Icon(c, richIcon(richanswer.String(node, "name", ""))).FontSize(float32(richanswer.Number(node, "size", 20))).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, first(label, richanswer.String(node, "name", ""))).FontWeight(600).MinWidth(0)
				if text := richanswer.String(node, "text", ""); text != "" {
					ui.Text(c, text).MinWidth(0)
				}
			})
		})
		return root
	case "divider":
		return ui.Column(c).FillWidth().Height(1).Background(t.Border)
	case "stack", "group":
		root := ui.Column(c).FillWidth().Gap(10)
		root.Children(func() { v.children(c, node) })
		return root
	case "row", "grid":
		return v.layout(c, node)
	case "stat":
		root := ui.Column(c).Gap(2).MinWidth(0)
		root.Children(func() {
			v.caption(c, label)
			ui.Text(c, richanswer.Label(node, v.values)).FontSize(kit.HelpSize(c) + 8).FontWeight(600).MinWidth(0)
		})
		return root
	case "range":
		return v.slider(c, node, bind, label)
	case "input":
		text := richanswer.Text(v.values[bind])
		root := ui.Column(c).FillWidth().Gap(4)
		root.Children(func() {
			if label != "" {
				ui.Text(c, label).MinWidth(0)
			}
			input := kit.TextInput(c.Key("input"), &text).FillWidth().Label(label).Placeholder(richanswer.String(node, "placeholder", ""))
			if input.Changed() {
				v.change(bind, text)
			}
		})
		return root
	case "check":
		checked, _ := v.values[bind].(bool)
		box := ui.Checkbox(c, &checked, first(label, richanswer.String(node, "text", "")))
		if box.Changed() {
			v.change(bind, checked)
		}
		return box
	case "select":
		return v.selectBox(c, node, bind, label)
	case "button":
		if _, sends := node.Props["send"]; sends {
			return v.sendAction(c, node)
		}
		button := kit.Button(c, first(label, richanswer.String(node, "text", "")), nil, false)
		if button.Clicked() {
			v.p.richMemory.set(v.stateKey, richanswer.Action(node, v.values))
		}
		return button
	case "form":
		root := ui.Column(c).FillWidth().Gap(10)
		root.Children(func() {
			v.children(c, node)
			if _, sends := node.Props["send"]; sends {
				v.sendAction(c, node)
			} else if label != "" && kit.Button(c.Key("submit"), label, nil, false).Clicked() {
				v.p.richMemory.set(v.stateKey, richanswer.Action(node, v.values))
			}
		})
		return root
	case "details":
		open, _ := v.values[bind].(bool)
		was := open
		root := ui.Collapsible(c, label, &open, func() {
			ui.Column(c).FillWidth().Gap(10).Children(func() { v.children(c, node) })
		})
		if open != was {
			v.change(bind, open)
		}
		return root
	case "tabs":
		return v.tabs(c, node, bind, label)
	case "chart":
		return v.chart(c, node, label)
	}
	// The stream admits only the kinds above.
	return ui.Column(c)
}

// layout is a row or grid: below 600 DIPs they become one column unless the
// answer says how many a phone keeps.
func (v *richView) layout(c *ui.Context, node *richanswer.Node) ui.Element {
	columns := int(math.Round(max(1, richanswer.Number(node, "columns", 2))))
	narrow := v.p.ui.compact
	weights, _ := node.Props["weights"].([]any)
	isGrid := node.Kind == "grid" || len(weights) > 0 || richanswer.String(node, "layout", "") == "aside"
	if narrow {
		columns = int(math.Round(max(1, richanswer.Number(node, "mobileColumns", 1))))
	}
	root := ui.Column(c).FillWidth().Gap(10)
	if !isGrid {
		root = ui.Row(c).FillWidth().Wrap().Gap(10)
		root.Children(func() { v.children(c, node) })
		return root
	}
	if len(weights) > 0 && !narrow {
		columns = max(columns, len(weights))
	}
	root.Children(func() {
		for start := 0; start < len(node.Children); start += columns {
			end := min(start+columns, len(node.Children))
			ui.Row(c.Key(strconv.Itoa(start))).FillWidth().Gap(10).AlignItems(ui.Start).Children(func() {
				for i := start; i < end; i++ {
					weight := float32(1)
					if i-start < len(weights) && !narrow {
						if value, ok := weights[i-start].(float64); ok && value > 0 {
							weight = float32(value)
						}
					}
					column := ui.Column(c.Key(node.Children[i])).Grow(weight).Basis(0).MinWidth(0)
					area := ui.Local(column, "rich-grid-width", func() ui.Handle { return ui.Handle{} })
					column.Bind(area).Children(func() {
						child := *v
						child.width = area.Bounds(c).W
						if child.width <= 0 {
							c.Invalidate()
						}
						child.node(c, node.Children[i])
					})
				}
			})
		}
	})
	return root
}

func (v *richView) slider(c *ui.Context, node *richanswer.Node, bind, label string) ui.Element {
	lo, hi, step, value := richanswer.Range(node, v.values[bind])
	root := ui.Column(c).FillWidth().Gap(4)
	root.Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			ui.Text(c, label).Grow(1).MinWidth(0)
			ui.Text(c, richanswer.Text(value)+richanswer.String(node, "suffix", "")).Shrink(0)
		})
		current := value
		slider := ui.Slider(c, &current, lo, hi).Step(step).FillWidth().Label(label)
		if slider.Changed() {
			v.change(bind, current)
		}
	})
	return root
}

func (v *richView) selectBox(c *ui.Context, node *richanswer.Node, bind, label string) ui.Element {
	selected := richanswer.Text(v.values[bind])
	options := []kit.SelectOption{{Value: "", Label: richanswer.String(node, "placeholder", label)}}
	for _, option := range richanswer.Options(node) {
		options = append(options, kit.SelectOption{Value: option.Value, Label: option.Label})
	}
	root := ui.Column(c).FillWidth().Gap(4)
	root.Children(func() {
		if label != "" {
			ui.Text(c, label).MinWidth(0)
		}
		if changed, _ := kit.Select(c, &selected, options); changed {
			v.change(bind, selected)
		}
	})
	return root
}

func (v *richView) tabs(c *ui.Context, node *richanswer.Node, bind, label string) ui.Element {
	if len(node.Children) == 0 {
		return ui.Column(c)
	}
	index := 0
	labels := make([]string, len(node.Children))
	for i, child := range node.Children {
		labels[i] = child
		if n := v.doc.Nodes[child]; n != nil {
			labels[i] = richanswer.String(n, "label", child)
		}
		if richanswer.Text(v.values[bind]) == child {
			index = i
		}
	}
	root := ui.Column(c).FillWidth().Gap(10).Label(label)
	root.Children(func() {
		before := index
		ui.Tabs(c, &index, labels...)
		if index != before {
			v.change(bind, node.Children[index])
		}
		v.node(c, node.Children[index])
	})
	return root
}

// sendAction is the only way an answer reaches the conversation: the exact visible
// text, on an explicit click, once. A failed send keeps the values for a retry.
func (v *richView) sendAction(c *ui.Context, node *richanswer.Node) ui.Element {
	s := &v.p.richMemory
	text := richanswer.SendText(node, v.values)
	key := v.stateKey + ":sent:" + node.ID
	sent := text != "" && s.Sent[key] == text
	sending := s.sending[key]
	label := first(richanswer.String(node, "label", ""), richanswer.String(node, "text", ""), botT("richAnswer.send"))
	switch {
	case sending:
		label = botT("richAnswer.sending")
	case sent:
		label = botT("richAnswer.sent")
	}
	root := ui.Column(c.Key("send")).FillWidth().Gap(6).AlignItems(ui.Start)
	root.Children(func() {
		if text != "" {
			ui.Text(c, text).FontSize(kit.HelpSize(c)).TextColor(c.Theme().TextMuted).MinWidth(0)
		}
		ready := v.complete && v.doc.Issues == 0 && text != "" && !sending && !sent && !v.p.busy && !v.p.opening && !v.p.reconnecting && !v.p.transferBusy && !isPaused(v.p.view) && !(v.p.bot != nil && v.p.bot.ReadOnly)
		if kit.Button(c, label, nil, sending).Disabled(!ready).Clicked() {
			s.sending[key] = true
			delete(s.failed, key)
			v.p.run(func(done func(error)) {
				v.p.send(&queuedSend{text: text}, "", func(err error) {
					delete(s.sending, key)
					if err != nil {
						s.failed[key] = true
					} else {
						s.Sent[key] = text
						s.save()
					}
					done(err)
				})
			})
		}
		if s.failed[key] {
			kit.InlineError(c, botT("richAnswer.sendFailed"))
		}
	})
	return root
}

// playRichMedia starts the native player in the answer for a local file. Anything
// else, as a web page of a video site, opens through the answer's link path.
func (p *Chat) playRichMedia(key, source, name string) {
	target := model.TargetOf(source, p.directory(), p.attachments())
	if !target.Local() {
		p.openLink(source, "answer:"+key)
		return
	}
	if p.richOpening[key] || p.richMedia[key] != nil {
		return
	}
	if p.richOpening == nil {
		p.richOpening = map[string]bool{}
	}
	p.richOpening[key] = true
	p.setItemError("media:"+key, "")
	id := p.activeID
	core.Go(p.env, func() (workspace.FilePreview, error) {
		var preview workspace.FilePreview
		err := p.env.Host.CallInto(p.method("file.preview"), map[string]string{"id": id, "path": target.Path}, &preview)
		return preview, err
	}, func(preview workspace.FilePreview, err error) {
		delete(p.richOpening, key)
		if p.closed() || p.activeID != id {
			return
		}
		var pane *workspace.MediaPane
		if err == nil {
			preview.Name = name
			pane, err = workspace.NewMediaPane(p.env, preview)
		}
		if err != nil {
			p.setItemError("media:"+key, err.Error())
			return
		}
		if p.richMedia == nil {
			p.richMedia = map[string]*workspace.MediaPane{}
		}
		p.richMedia[key] = pane
	})
}

// media draws pictures inline; a video or audio shows its button until the user
// asks to play it, rather than loading while scrolling.
func (v *richView) media(c *ui.Context, node *richanswer.Node) ui.Element {
	root := ui.Column(c).FillWidth().Gap(4)
	root.Children(func() {
		source := richanswer.String(node, "src", "")
		caption := richanswer.String(node, "caption", "")
		if node.Kind == "image" {
			text, _ := richanswer.Markdown(node)
			key := v.stateKey + ":" + node.ID
			markdown.View(c, quoteSpacedImagePaths(text), v.p.richMarkdownOptions(key)).FillWidth()
			kit.InlineError(c, v.p.ui.items.errors["image:rich:"+key])
		} else if source != "" {
			icon := icons.Video
			if node.Kind == "audio" {
				icon = icons.AudioLines
			}
			name := first(richanswer.String(node, "alt", ""), richanswer.String(node, "label", ""), caption, filepath.Base(source))
			key := v.stateKey + ":" + node.ID
			if pane := v.p.richMedia[key]; pane != nil {
				v.p.mediaShown[key] = true
				height := float32(64)
				if node.Kind == "video" {
					height = 300
				}
				ui.Column(c.Key("player")).FillWidth().Height(height).Children(func() { pane.View(c) })
			} else if kit.Button(c.Key("open"), name, icon, v.p.richOpening[key]).Disabled(v.p.richOpening[key]).Tooltip(source).Clicked() {
				v.p.playRichMedia(key, source, name)
			}
			if err := v.p.ui.items.errors["media:"+key]; err != "" {
				kit.InlineError(c, err)
			}
		}
		if caption != "" {
			v.caption(c, caption)
		}
	})
	return root
}

func richIcon(name string) *ui.SVG {
	switch name {
	case "mail":
		return icons.Mail
	case "calendar":
		return icons.CalendarDays
	case "folder":
		return icons.Folder
	case "chart":
		return icons.ChartColumn
	case "check":
		return icons.Check
	case "clock":
		return icons.Clock
	case "location":
		return icons.MapPin
	case "coffee":
		return icons.Coffee
	case "image":
		return icons.Image
	case "users":
		return icons.Users
	case "star":
		return icons.Star
	}
	return icons.Info
}

func (v *richView) chart(c *ui.Context, node *richanswer.Node, label string) ui.Element {
	t := c.Theme()
	data := richanswer.Chart(node, v.values)
	color := richRoleColor(t, richanswer.ColorRole(node.Props["color"]))
	root := ui.Column(c).FillWidth().Gap(8).Label(label)
	root.Children(func() {
		if label != "" {
			ui.Text(c, label).FontWeight(600).MinWidth(0)
		}
		if richanswer.String(node, "variant", "") == "line" {
			v.lineChart(c, node, data, color)
			return
		}
		lo, hi := 0.0, 0.0
		for _, item := range data {
			lo, hi = min(lo, item.Value), max(hi, item.Value)
		}
		span := hi - lo
		if span == 0 {
			span = 1
		}
		colors, _ := node.Props["colors"].([]any)
		for i, item := range data {
			barColor := color
			if i < len(colors) {
				if role := richanswer.ColorRole(colors[i]); role != "" {
					barColor = richRoleColor(t, role)
				}
			}
			ui.Column(c.Key(strconv.Itoa(i))).FillWidth().Gap(3).Children(func() {
				ui.Row(c).FillWidth().Children(func() {
					ui.Text(c, item.Label).FontSize(kit.HelpSize(c)).Grow(1).MinWidth(0)
					ui.Text(c, richanswer.Text(item.Value)).FontSize(kit.HelpSize(c)).Shrink(0)
				})
				at, size := (min(0, item.Value)-lo)/span, math.Abs(item.Value)/span
				ui.Box(c).FillWidth().Height(8).Radius(4).Background(t.Border).Draw(func(paint *ui.Painter, r ui.Rect) {
					paint.Fill(ui.Rect{X: r.X + r.W*float32(at), Y: r.Y, W: r.W * float32(size), H: r.H}, barColor, 4)
				})
			})
		}
	})
	return root
}

// lineChart paints richLinePlot's coordinates at the width the chart has.
func (v *richView) lineChart(c *ui.Context, node *richanswer.Node, data []richanswer.Point, color ui.Color) {
	t := c.Theme()
	font := kit.CaptionSize(c)
	height := richanswer.PlotLine(data, 480, node.Props).Height
	ui.Box(c.Key("plot")).FillWidth().Height(float32(height)).Draw(func(paint *ui.Painter, r ui.Rect) {
		plot := richanswer.PlotLine(data, float64(r.W), node.Props)
		x := func(value float64) float32 { return r.X + float32(value) }
		y := func(value float64) float32 { return r.Y + float32(value) }
		for _, tick := range plot.Ticks {
			if node.Props["showGrid"] != false {
				paint.Line(x(plot.Left), y(tick.Y), x(plot.Width-plot.Right), y(tick.Y), 1, t.Border)
			}
			width := richTextWidth(paint, tick.Label, font)
			paint.Text(x(plot.Left-6)-width, y(tick.Y-float64(font)/2), tick.Label, font, t.TextMuted)
		}
		var line ui.Path
		for i, point := range plot.Points {
			if i == 0 {
				line.MoveTo(x(point.X), y(point.Y))
			} else {
				line.LineTo(x(point.X), y(point.Y))
			}
		}
		paint.StrokePath(&line, float32(richanswer.Number(node, "strokeWidth", 2)), color)
		for _, label := range plot.Labels {
			width := richTextWidth(paint, label.Label, font)
			paint.Text(x(label.X)-width/2, y(plot.LabelY-float64(font)), label.Label, font, t.TextMuted)
		}
		radius := float32(richanswer.Number(node, "pointRadius", 3))
		for _, point := range plot.Points {
			if node.Props["showPoints"] != false {
				var dot ui.Path
				dot.Circle(x(point.X), y(point.Y), radius)
				paint.FillPath(&dot, color)
			}
			if plot.ShowValues {
				text := richanswer.Text(point.Value)
				width := richTextWidth(paint, text, font)
				paint.Text(x(point.X)-width/2, y(point.Y-10-float64(font)), text, font, t.Text)
			}
		}
	})
}

var richNumbers = regexp.MustCompile(`[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?`)

func richNumberList(text string) []float64 {
	var out []float64
	for _, field := range richNumbers.FindAllString(text, -1) {
		if value, err := strconv.ParseFloat(field, 64); err == nil {
			out = append(out, value)
		}
	}
	return out
}

// richTransform is an affine transform of a shape's coordinates.
type richTransform [6]float64

func (a richTransform) apply(x, y float64) (float64, float64) {
	return a[0]*x + a[2]*y + a[4], a[1]*x + a[3]*y + a[5]
}

func (a richTransform) then(b richTransform) richTransform {
	return richTransform{
		a[0]*b[0] + a[2]*b[1], a[1]*b[0] + a[3]*b[1],
		a[0]*b[2] + a[2]*b[3], a[1]*b[2] + a[3]*b[3],
		a[0]*b[4] + a[2]*b[5] + a[4], a[1]*b[4] + a[3]*b[5] + a[5],
	}
}

var richTransforms = regexp.MustCompile(`(translate|scale|rotate|matrix|skewX|skewY)\(([^)]*)\)`)

func parseRichTransform(text string) richTransform {
	out := richTransform{1, 0, 0, 1, 0, 0}
	for _, match := range richTransforms.FindAllStringSubmatch(text, -1) {
		args := richNumberList(match[2])
		arg := func(i int, fallback float64) float64 {
			if i < len(args) {
				return args[i]
			}
			return fallback
		}
		switch match[1] {
		case "translate":
			out = out.then(richTransform{1, 0, 0, 1, arg(0, 0), arg(1, 0)})
		case "scale":
			out = out.then(richTransform{arg(0, 1), 0, 0, arg(1, arg(0, 1)), 0, 0})
		case "rotate":
			angle := arg(0, 0) * math.Pi / 180
			sin, cos := math.Sincos(angle)
			rotation := richTransform{cos, sin, -sin, cos, 0, 0}
			if len(args) >= 3 {
				rotation = richTransform{1, 0, 0, 1, args[1], args[2]}.then(rotation).then(richTransform{1, 0, 0, 1, -args[1], -args[2]})
			}
			out = out.then(rotation)
		case "matrix":
			if len(args) == 6 {
				out = out.then(richTransform{args[0], args[1], args[2], args[3], args[4], args[5]})
			}
		case "skewX":
			out = out.then(richTransform{1, 0, math.Tan(arg(0, 0) * math.Pi / 180), 1, 0, 0})
		case "skewY":
			out = out.then(richTransform{1, math.Tan(arg(0, 0) * math.Pi / 180), 0, 1, 0, 0})
		}
	}
	return out
}

// vector paints an svg node: declarative shapes only, scaled to the width it has.
func (v *richView) vector(c *ui.Context, node *richanswer.Node, label string) ui.Element {
	t := c.Theme()
	box, shapes := richanswer.Vector(node)
	root := ui.Box(c).FillWidth().Label(first(label, richanswer.String(node, "alt", ""))).AspectRatio(float32(box[2] / box[3]))
	root.Draw(func(paint *ui.Painter, r ui.Rect) {
		scale := float64(r.W) / box[2]
		view := richTransform{scale, 0, 0, scale, float64(r.X) - box[0]*scale, float64(r.Y) - box[1]*scale}
		for _, shape := range shapes {
			paintRichShape(paint, t, view, scale, shape)
		}
	})
	return root
}

func paintRichShape(paint *ui.Painter, t *ui.Theme, view richTransform, scale float64, shape richanswer.Shape) {
	number := func(key string) float64 { value, _ := shape.Props[key].(float64); return value }
	text := func(key string) string { value, _ := shape.Props[key].(string); return value }
	matrix := view
	if transform := text("transform"); transform != "" {
		matrix = view.then(parseRichTransform(transform))
	}
	var path ui.Path
	move := func(x, y float64) { px, py := matrix.apply(x, y); path.MoveTo(float32(px), float32(py)) }
	line := func(x, y float64) { px, py := matrix.apply(x, y); path.LineTo(float32(px), float32(py)) }
	ellipse := func(cx, cy, rx, ry float64) {
		const steps = 48
		for i := 0; i <= steps; i++ {
			angle := 2 * math.Pi * float64(i) / steps
			if i == 0 {
				move(cx+rx*math.Cos(angle), cy+ry*math.Sin(angle))
			} else {
				line(cx+rx*math.Cos(angle), cy+ry*math.Sin(angle))
			}
		}
		path.Close()
	}
	fillDefault := true
	switch shape.Type {
	case "path":
		richPath(&path, text("d"), matrix)
	case "rect":
		x, y, w, h := number("x"), number("y"), number("width"), number("height")
		move(x, y)
		line(x+w, y)
		line(x+w, y+h)
		line(x, y+h)
		path.Close()
	case "circle":
		ellipse(number("cx"), number("cy"), number("r"), number("r"))
	case "ellipse":
		ellipse(number("cx"), number("cy"), number("rx"), number("ry"))
	case "line":
		fillDefault = false
		move(number("x1"), number("y1"))
		line(number("x2"), number("y2"))
	case "polyline", "polygon":
		points := richNumberList(text("points"))
		for i := 0; i+1 < len(points); i += 2 {
			if i == 0 {
				move(points[i], points[i+1])
			} else {
				line(points[i], points[i+1])
			}
		}
		if shape.Type == "polygon" {
			path.Close()
		}
	case "text":
		size := number("fontSize")
		if size == 0 {
			size = 14
		}
		fontSize := float32(size * scale)
		color := t.Text
		if role := richanswer.ColorRole(shape.Props["fill"]); role != "" {
			color = richRoleColor(t, role)
		}
		x, y := matrix.apply(number("x"), number("y"))
		width := richTextWidth(paint, shape.Text, fontSize)
		switch text("textAnchor") {
		case "middle":
			x -= float64(width) / 2
		case "end":
			x -= float64(width)
		}
		paint.Text(float32(x), float32(y)-fontSize, shape.Text, fontSize, color)
		return
	}
	// Unpainted shapes take the text color, as they do in Cindy.
	fill, stroke := text("fill"), text("stroke")
	if fill == "" && fillDefault || richanswer.ColorRole(fill) != "" {
		paint.FillPath(&path, richRoleColor(t, richanswer.ColorRole(fill)))
	}
	if richanswer.ColorRole(stroke) != "" || !fillDefault && stroke == "" {
		width := number("strokeWidth")
		if width == 0 {
			width = 1
		}
		if dash := richNumberList(text("strokeDasharray")); len(dash) > 0 {
			lengths := make([]float32, len(dash))
			for i, value := range dash {
				lengths[i] = float32(value * scale)
			}
			path.Dash(0, lengths...)
		}
		paint.StrokePath(&path, float32(width*scale), richRoleColor(t, richanswer.ColorRole(stroke)))
	}
}

var richPathTokens = regexp.MustCompile(`[MmZzLlHhVvCcSsQqTtAa]|[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?`)

// richPath follows SVG path data through the shape's transform. Arcs become short
// lines: a declarative answer needs their shape, not their curve primitives.
func richPath(path *ui.Path, d string, m richTransform) {
	tokens := richPathTokens.FindAllString(d, -1)
	var command byte
	var cx, cy, sx, sy, lastCx, lastCy float64
	var previous byte
	i := 0
	arg := func() float64 {
		if i >= len(tokens) {
			return 0
		}
		value, _ := strconv.ParseFloat(tokens[i], 64)
		i++
		return value
	}
	move := func(x, y float64) { px, py := m.apply(x, y); path.MoveTo(float32(px), float32(py)) }
	line := func(x, y float64) { px, py := m.apply(x, y); path.LineTo(float32(px), float32(py)) }
	quad := func(a, b, x, y float64) {
		pa, pb := m.apply(a, b)
		px, py := m.apply(x, y)
		path.QuadTo(float32(pa), float32(pb), float32(px), float32(py))
	}
	cube := func(a, b, e, f, x, y float64) {
		pa, pb := m.apply(a, b)
		pe, pf := m.apply(e, f)
		px, py := m.apply(x, y)
		path.CubeTo(float32(pa), float32(pb), float32(pe), float32(pf), float32(px), float32(py))
	}
	for i < len(tokens) {
		if token := tokens[i]; len(token) == 1 && strings.ContainsAny(token, "MmZzLlHhVvCcSsQqTtAa") {
			command = token[0]
			i++
		} else if command == 0 {
			return
		}
		relative := command >= 'a'
		base := func(x, y float64) (float64, float64) {
			if relative {
				return cx + x, cy + y
			}
			return x, y
		}
		upper := command &^ 0x20
		switch upper {
		case 'M':
			cx, cy = base(arg(), arg())
			sx, sy = cx, cy
			move(cx, cy)
			// Further pairs of a move are lines.
			if relative {
				command = 'l'
			} else {
				command = 'L'
			}
		case 'Z':
			path.Close()
			cx, cy = sx, sy
		case 'L':
			cx, cy = base(arg(), arg())
			line(cx, cy)
		case 'H':
			value := arg()
			if relative {
				cx += value
			} else {
				cx = value
			}
			line(cx, cy)
		case 'V':
			value := arg()
			if relative {
				cy += value
			} else {
				cy = value
			}
			line(cx, cy)
		case 'C':
			a, b := base(arg(), arg())
			e, f := base(arg(), arg())
			x, y := base(arg(), arg())
			cube(a, b, e, f, x, y)
			lastCx, lastCy, cx, cy = e, f, x, y
		case 'S':
			a, b := cx, cy
			if previous == 'C' || previous == 'S' {
				a, b = 2*cx-lastCx, 2*cy-lastCy
			}
			e, f := base(arg(), arg())
			x, y := base(arg(), arg())
			cube(a, b, e, f, x, y)
			lastCx, lastCy, cx, cy = e, f, x, y
		case 'Q':
			a, b := base(arg(), arg())
			x, y := base(arg(), arg())
			quad(a, b, x, y)
			lastCx, lastCy, cx, cy = a, b, x, y
		case 'T':
			a, b := cx, cy
			if previous == 'Q' || previous == 'T' {
				a, b = 2*cx-lastCx, 2*cy-lastCy
			}
			x, y := base(arg(), arg())
			quad(a, b, x, y)
			lastCx, lastCy, cx, cy = a, b, x, y
		case 'A':
			rx, ry := arg(), arg()
			rotation, large, sweep := arg(), arg(), arg()
			x, y := base(arg(), arg())
			for _, point := range richArc(cx, cy, rx, ry, rotation, large != 0, sweep != 0, x, y) {
				line(point[0], point[1])
			}
			cx, cy = x, y
		default:
			return
		}
		previous = upper
	}
}

// richArc is the SVG arc conversion from endpoints to a center, sampled as lines.
func richArc(x0, y0, rx, ry, rotation float64, large, sweep bool, x1, y1 float64) [][2]float64 {
	if rx == 0 || ry == 0 {
		return [][2]float64{{x1, y1}}
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	sin, cos := math.Sincos(rotation * math.Pi / 180)
	dx, dy := (x0-x1)/2, (y0-y1)/2
	xp, yp := cos*dx+sin*dy, -sin*dx+cos*dy
	if lambda := xp*xp/(rx*rx) + yp*yp/(ry*ry); lambda > 1 {
		rx, ry = rx*math.Sqrt(lambda), ry*math.Sqrt(lambda)
	}
	numerator := rx*rx*ry*ry - rx*rx*yp*yp - ry*ry*xp*xp
	denominator := rx*rx*yp*yp + ry*ry*xp*xp
	factor := 0.0
	if denominator != 0 && numerator > 0 {
		factor = math.Sqrt(numerator / denominator)
	}
	if large == sweep {
		factor = -factor
	}
	cxp, cyp := factor*rx*yp/ry, -factor*ry*xp/rx
	cx, cy := cos*cxp-sin*cyp+(x0+x1)/2, sin*cxp+cos*cyp+(y0+y1)/2
	start := math.Atan2((yp-cyp)/ry, (xp-cxp)/rx)
	end := math.Atan2((-yp-cyp)/ry, (-xp-cxp)/rx)
	delta := end - start
	if sweep && delta < 0 {
		delta += 2 * math.Pi
	} else if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	}
	const steps = 32
	points := make([][2]float64, 0, steps)
	for i := 1; i <= steps; i++ {
		angle := start + delta*float64(i)/steps
		x, y := rx*math.Cos(angle), ry*math.Sin(angle)
		points = append(points, [2]float64{cos*x - sin*y + cx, sin*x + cos*y + cy})
	}
	return points
}

// richMarkdownOptions are the Markdown options of a node's text: links go through
// the same file and link path as the rest of the answer.
func (p *Chat) richMarkdownOptions(key string) *markdown.Options {
	return p.answerOptions("rich:" + key)
}

// chatDiagram is a Mermaid block drawn by the one-shot local renderer.
type chatDiagram struct {
	bitmap  *ui.Bitmap
	failure string
	loading bool
}

// mermaidBlock draws a complete diagram; while it streams, or when it cannot be
// drawn, its source stays readable and copyable.
func (p *Chat) mermaidBlock(c *ui.Context, key string, b *markdown.CodeBlockView) {
	t := c.Theme()
	source := strings.TrimRight(b.Code(), "\n")
	showSource := func() { ui.Text(c, source).Font(kit.MonoFont).TextColor(t.TextMuted).Selectable().MinWidth(0) }
	if b.Open {
		showSource()
		return
	}
	if p.diagrams == nil {
		p.diagrams = map[string]*chatDiagram{}
	}
	id := fmt.Sprintf("%x/%t", sha256.Sum256([]byte(source)), t.Dark)
	entry := p.diagrams[id]
	if entry == nil {
		entry = &chatDiagram{loading: true}
		p.diagrams[id] = entry
		dark := t.Dark
		core.Go(p.env, func() (*ui.Bitmap, error) {
			return resultui.RenderDiagram(context.Background(), source, dark)
		}, func(bitmap *ui.Bitmap, err error) {
			entry.loading, entry.bitmap = false, bitmap
			if err != nil {
				entry.failure = err.Error()
			}
		})
	}
	ui.Column(c.Key(id)).FillWidth().Gap(6).Children(func() {
		switch {
		case entry.loading:
			kit.Loader(c)
		case entry.bitmap != nil:
			w, h := entry.bitmap.Size()
			ui.Image(c, entry.bitmap).FillWidth().MaxWidth(float32(w)).AspectRatio(float32(w) / float32(max(h, 1))).Label(source)
		default:
			kit.InlineError(c, entry.failure)
			showSource()
		}
		if kit.IconButton(c, icons.Copy, i18n.T("chat.copy")).AlignSelf(ui.End).Clicked() {
			p.copy(source)
		}
	})
}

func richTextWidth(paint *ui.Painter, text string, size float32) float32 {
	width, _ := paint.MeasureText(math.MaxFloat32, ui.Span{Text: text, Size: size})
	return width
}

package pluginhost

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/net/html"

	"winone/desktopmygo/chat/workspace"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
)

// A plugin's card is HTML the plugin host already sanitized to a small set of tags
// (cardSanitizer.ts), with no scripts and no styles that reach the window. The
// native view draws those tags as they read; a plugin's own colors and animation
// have no meaning here, as every WinOne surface takes its colors from the theme.

type card struct {
	CallID                                string            `json:"callId"`
	SessionID                             string            `json:"sessionId"`
	GhostID                               string            `json:"ghostId"`
	Mail                                  *mailCardData     `json:"mail"`
	RootCallID                            string            `json:"rootCallId"`
	ViewHTML                              string            `json:"viewHtml"`
	Fields                                []cardFormField   `json:"fields"`
	FormChange                            string            `json:"formChange"`
	UpdatedAt                             int64             `json:"updatedAt"`
	Children                              []json.RawMessage `json:"children"`
	values, baseValues                    map[string]string
	formSending, formDirty                bool
	readerMail                            bool
	formGeneration                        uint64
	formWaiters                           []func(bool)
	expanded                              map[string]bool
	promptAction, promptHint, promptValue string
	err                                   string
	HTML                                  string `json:"html"`
	State                                 string `json:"state"`
	root                                  *html.Node
	source                                string
	loading                               bool
	busy                                  bool
}

type cards struct {
	env          *core.Env
	byCall       map[string]*card
	images       map[string]*ui.Bitmap
	asked        map[string]bool
	media        map[string]*workspace.MediaPane
	mediaVisible map[string]bool
	mail         *mailController
}

func newCards(env *core.Env) *cards {
	return &cards{env: env, byCall: map[string]*card{}, images: map[string]*ui.Bitmap{}, asked: map[string]bool{}, media: map[string]*workspace.MediaPane{}, mediaVisible: map[string]bool{}, mail: newMailController()}
}

func (s *cards) update(raw json.RawMessage) {
	var next card
	if json.Unmarshal(raw, &next) != nil || next.CallID == "" {
		return
	}
	if old := s.byCall[next.CallID]; old != nil {
		if mailIdentity(old.Mail) != mailIdentity(next.Mail) || old.FormChange != next.FormChange && old.Mail == nil && next.Mail == nil {
			retireCardForm(old)
		}
		old.HTML, old.ViewHTML, old.RootCallID, old.State, old.UpdatedAt = next.HTML, next.ViewHTML, next.RootCallID, next.State, next.UpdatedAt
		old.SessionID, old.GhostID, old.Mail = next.SessionID, next.GhostID, next.Mail
		old.Fields, old.FormChange, old.loading = next.Fields, next.FormChange, false
		mergeCardFields(old)
		if !mailCanEdit(old) {
			old.formDirty = false
		}
	} else {
		mergeCardFields(&next)
		s.byCall[next.CallID] = &next
	}
	for _, child := range next.Children {
		s.update(child)
	}
}

func (s *cards) parsed(item *card) *html.Node {
	source := item.ViewHTML
	if source == "" && len(item.Fields) == 0 {
		source = item.HTML
	}
	if item.root == nil || item.source != source {
		item.root, _ = html.Parse(strings.NewReader(source))
		item.source = source
	}
	return item.root
}

// Card draws the card a tool call supplied, if it supplied one.
func (h *Host) Card(c *ui.Context, callID string) {
	s := h.cards
	item := s.byCall[callID]
	if item == nil {
		item = &card{CallID: callID, loading: true}
		s.byCall[callID] = item
		h.call("card-get", map[string]string{"callId": callID}, func(value json.RawMessage, err error) {
			item.loading = false
			if err == nil {
				s.update(value)
			} else {
				item.err = err.Error()
			}
		})
	}
	if item.HTML == "" {
		if item.loading {
			kit.Loader(c)
		}
		kit.InlineError(c, item.err)
		return
	}
	t := c.Theme()
	if item.Mail != nil || isLegacyMail(item) {
		h.mailCard(c.Key(callID), item, false)
	} else {
		ui.Column(c.Key(callID)).FillWidth().MinWidth(0).Gap(6).Padding(10).Radius(8).BorderWidth(1).BorderColor(t.Border).Children(func() {
			h.cardFields(c, item)
			root := s.parsed(item)
			if body := findBody(root); body != nil {
				h.cardChildren(c, item, body, "")
			}
		})
	}
	h.cardPrompt(c, item)
	if item.RootCallID == "" || item.RootCallID == callID {
		children := []*card{}
		for id, child := range s.byCall {
			if id != callID && child.RootCallID == callID {
				children = append(children, child)
			}
		}
		sort.Slice(children, func(i, j int) bool { return children[i].UpdatedAt < children[j].UpdatedAt })
		for _, child := range children {
			h.Card(c.Key(child.CallID), child.CallID)
		}
	}
}

func findBody(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := findBody(child); found != nil {
			return found
		}
	}
	return nil
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	if n.Type == html.ElementNode && n.Data == "br" {
		return "\n"
	}
	var out strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "style" {
			continue
		}
		out.WriteString(textOf(child))
	}
	return out.String()
}

var blockTags = map[string]bool{"div": true, "p": true, "h1": true, "h2": true, "h3": true, "h4": true, "ul": true, "ol": true, "li": true, "hr": true, "img": true, "figure": true, "figcaption": true, "blockquote": true, "pre": true, "table": true, "thead": true, "tbody": true, "tr": true, "section": true, "header": true, "footer": true, "button": true, "a": true, "video": true, "audio": true}

func containsBlock(n *html.Node) bool {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && (blockTags[child.Data] || containsBlock(child)) {
			return true
		}
	}
	return false
}

// cardChildren groups running inline text into one paragraph between the blocks.
func (h *Host) cardChildren(c *ui.Context, item *card, parent *html.Node, key string) {
	t := c.Theme()
	var inline strings.Builder
	index := 0
	flush := func() {
		text := strings.Join(strings.Fields(inline.String()), " ")
		inline.Reset()
		if text != "" {
			index++
			ui.Text(c.Key(key+"/t"+itoa(index)), text).Selectable().MinWidth(0)
		}
	}
	for child := parent.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			inline.WriteString(child.Data)
			continue
		}
		if child.Type != html.ElementNode || child.Data == "style" {
			continue
		}
		if !blockTags[child.Data] && !containsBlock(child) {
			inline.WriteString(textOf(child))
			continue
		}
		flush()
		index++
		childKey := key + "/" + itoa(index)
		switch child.Data {
		case "h1", "h2", "h3", "h4":
			size := map[string]float32{"h1": 1.4, "h2": 1.25, "h3": 1.1, "h4": 1}[child.Data]
			ui.Text(c.Key(childKey), strings.TrimSpace(textOf(child))).FontWeight(600).FontSize(t.Rem(size)).Selectable().MinWidth(0)
		case "hr":
			ui.Column(c.Key(childKey)).FillWidth().Height(1).Background(t.Border)
		case "pre":
			ui.Text(c.Key(childKey), textOf(child)).Font(kit.MonoFont).Selectable().MinWidth(0)
		case "img":
			h.cardImage(c, childKey, attr(child, "src"), attr(child, "alt"))
		case "video", "audio":
			source := attr(child, "src")
			if source == "" {
				for node := child.FirstChild; node != nil; node = node.NextSibling {
					if node.Data == "source" {
						source = attr(node, "src")
						break
					}
				}
			}
			h.cardMedia(c, childKey, source, child.Data)
		case "button", "a":
			h.cardButton(c, item, childKey, child)
		case "li":
			ui.Row(c.Key(childKey)).FillWidth().Gap(6).Children(func() {
				ui.Text(c, "•").Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() { h.cardChildren(c, item, child, childKey) })
			})
		case "tr":
			ui.Row(c.Key(childKey)).FillWidth().Gap(8).Children(func() {
				cell := 0
				for column := child.FirstChild; column != nil; column = column.NextSibling {
					if column.Type == html.ElementNode {
						cell++
						weight := 400
						if column.Data == "th" {
							weight = 600
						}
						ui.Text(c.Key(childKey+"/c"+itoa(cell)), strings.TrimSpace(textOf(column))).FontWeight(weight).Grow(1).Basis(0).MinWidth(0).Selectable()
					}
				}
			})
		default:
			ui.Column(c.Key(childKey)).FillWidth().Gap(6).Children(func() { h.cardChildren(c, item, child, childKey) })
		}
	}
	flush()
}

func itoa(value int) string {
	const digits = "0123456789"
	if value < 10 {
		return digits[value : value+1]
	}
	return itoa(value/10) + digits[value%10:value%10+1]
}

func (h *Host) cardButton(c *ui.Context, item *card, key string, n *html.Node) {
	action := attr(n, "data-ghost-action")
	label := strings.TrimSpace(textOf(n))
	link, prompt := attr(n, "data-ghost-link"), attr(n, "data-ghost-prompt")
	modelPicker := hasAttr(n, "data-ghost-model-select")
	if kit.Button(c.Key(key), label, nil, false).Disabled(action == "" && link == "" || item.busy || item.formSending).Clicked() {
		if link != "" {
			h.cardDispatch(item, "card-link", map[string]any{"url": link})
			return
		}
		if hasAttr(n, "data-ghost-prompt") {
			item.promptAction, item.promptHint, item.promptValue = action, prompt, ""
			return
		}
		h.cardDispatch(item, "card-action", map[string]any{"action": action, "pickModel": modelPicker})
	}
}

// cardImage draws a picture from the local media store; sanitized cards can point
// nowhere else.
func (h *Host) cardImage(c *ui.Context, key, source, alt string) {
	s := h.cards
	if source == "" {
		return
	}
	bitmap := s.images[source]
	if bitmap == nil && !s.asked[source] {
		s.asked[source] = true
		core.Go(h.env, func() (*ui.Bitmap, error) {
			var answer struct {
				Path string `json:"path"`
			}
			if err := h.env.Host.CallInto("chat.ghosts.media-path", map[string]string{"url": source}, &answer); err != nil {
				return nil, err
			}
			data, err := os.ReadFile(answer.Path)
			if err != nil {
				return nil, err
			}
			return ui.DecodeBitmap(data)
		}, func(value *ui.Bitmap, err error) {
			if err == nil {
				s.images[source] = value
			}
		})
	}
	if bitmap == nil {
		return
	}
	w, height := bitmap.Size()
	ui.Image(c.Key(key), bitmap).FillWidth().MaxWidth(float32(w)).AspectRatio(float32(w) / float32(max(height, 1))).Label(alt)
}

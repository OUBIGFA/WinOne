// Package resultui provides the shared MyGo result and conversation view:
// OCR and translation of a capture, the selection toolbar and its results,
// and the Quick assistant all use this one Panel and Controller.
package resultui

import (
	"embed"
	"math"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"winone/internal/ai"
	"winone/internal/learning"
	"winone/internal/resultwindow"
	"winone/modules/nativeui/modelmenu"
)

//go:embed assets/*.svg assets/providers/*.svg assets/providers/*.png assets/provider-bitmaps/*.png assets/logo.png
var assets embed.FS
var iconCache sync.Map

type Message struct {
	Study                                                                     *study
	Role, Text, Model, Provider, ProviderName, ProviderIcon, Reasoning, State string
	ActionIcon, ActionLabel                                                   string
	ReasoningOpen                                                             bool
	ReasoningStartedAt, ReasoningDurationMs                                   int64
	ReasoningActive                                                           bool
	thinkingInline                                                            *thinkingDocument
	thinkingScroll                                                            ui.ScrollState
	thinkingMeasuredText                                                      string
	thinkingMeasuredWidth, thinkingMeasuredHeight, thinkingMeasuredScale      float32
	thinkingMeasuredOpen                                                      bool
}
type Actions struct {
	StudyCheck                        func(*study)
	StudySpeak                        func(*study, string, string)
	StudyNavigate                     func(*study, bool, string)
	StudyAdd                          func(*study, []int)
	StudyState                        func(*study, string, learning.TermState)
	loadAvatar                        func(string, func(*providerAvatar, error))
	OpenQuick                         func(string)
	Quick                             func(resultwindow.SelectionAction)
	NewChat                           func()
	ClearAll, Restart                 func()
	Copy                              func(string)
	Speak                             func(key, text string, markdown bool)
	Retry, RetryResult, Cancel, Close func()
	Send                              func(string)
	Pin                               func(bool)
	Target                            func(string)
	Selection                         func(*ai.Selection)
	RenderDiagram                     func(string, bool, func(*ui.Bitmap, error))
}

// View state is confined to MyGo's main thread. Async callers update it with
// Window.Update; original OCR text is independent of every conversation reply.
type Panel struct {
	Study                                               *study
	avatars                                             map[string]*providerAvatar
	avatarClock                                         uint64
	QuickReasoning                                      *bool
	QuickActions                                        []resultwindow.SelectionAction
	FocusComposer                                       bool
	ContextReset, ShowComposerHint                      bool
	composerEdited                                      bool
	Mode, Language, Theme                               string
	Source, Action                                      string
	ActionLabel                                         string
	Original, Result, Prompt, Error, Notice             string
	errorVisibility, noticeVisibility, speechVisibility feedbackVisibility
	Target                                              string
	Busy, Pinned                                        bool
	Recognizing                                         bool
	Messages                                            []Message
	Pending, ResultPending                              *Message
	ResultDetails                                       Message
	Actions                                             Actions
	Configuration                                       ai.Configuration
	ActionSelection                                     ai.Selection
	ResultSelection                                     ai.Selection
	Selection                                           *ai.Selection
	TranslationSelection                                *ai.Selection
	TranslationModelChanged                             bool
	TranslationPickerOpen                               bool
	ModelPickerOpen                                     bool
	modelMenu, translationMenu                          modelmenu.State
	scroll                                              ui.ScrollState
	composerHeight                                      float32
	diagrams                                            map[string]*diagramEntry
	diagramClock                                        uint64
	SpeechKey, SpeechErrorKey, SpeechError              string
	SpeechLoading                                       bool
	// markdown holds the options every Markdown view of this panel shares.
	markdown markdownViews
	// selected is the text the focused selectable block or the input
	// selects, collected while this pass builds them (see noteSelection).
	selected string
	// services opens links and repaints from callbacks between builds.
	services ui.Services
}

func NewPanel(mode, language, theme string) *Panel {
	return &Panel{Mode: mode, Language: language, Theme: theme, Target: "auto"}
}
func (p *Panel) FollowEnd()  { p.scroll.Y = math.MaxFloat32 }
func (p *Panel) AtEnd() bool { return p.scroll.Y >= p.scroll.MaxY-1 }
func (p *Panel) T(zh, en string) string {
	if p.Language == "en-US" {
		return en
	}
	return zh
}
func Logo() []byte { data, _ := assets.ReadFile("assets/logo.png"); return data }
func svg(name string) *ui.SVG {
	if value, ok := iconCache.Load(name); ok {
		return value.(*ui.SVG)
	}
	data, err := assets.ReadFile("assets/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	value := ui.MustParseSVG(data)
	actual, _ := iconCache.LoadOrStore(name, value)
	return actual.(*ui.SVG)
}
func providerSVG(name string) *ui.SVG {
	if name == "model" {
		return svg("model")
	}
	if name == "" || name == "service" || strings.ContainsAny(name, "/\\.") {
		return svg("provider-default")
	}
	key := "providers/" + name
	if cached, ok := iconCache.Load(key); ok {
		return cached.(*ui.SVG)
	}
	data, err := assets.ReadFile("assets/" + key + ".svg")
	if err != nil {
		return svg("provider-default")
	}
	icon, _ := iconCache.LoadOrStore(key, ui.MustParseSVG(data))
	return icon.(*ui.SVG)
}

// Keep one asset inventory: the native icon renderer supplies a theme-colored
// silhouette in dark mode; light mode preserves the official brand colors.
func providerImage(c *ui.Context, t *ui.Theme, name string, size float32) ui.Element {
	if bitmap := providerBitmap(name, t); bitmap != nil {
		return ui.Image(c, bitmap).Size(size, size).Fit(ui.Contain).Shrink(0).Role(ui.RoleNone)
	}
	// Magpie supplies ZCode as monochrome PNG; keep its actual mark instead
	// of tracing a replacement SVG or flattening its opaque background to a mask.
	if name == "zcode" {
		cached, ok := iconCache.Load("bitmap/zcode")
		if !ok {
			data, err := assets.ReadFile("assets/providers/zcode.png")
			if err != nil {
				panic(err)
			}
			bitmap, err := ui.DecodeBitmap(data)
			if err != nil {
				panic(err)
			}
			cached, _ = iconCache.LoadOrStore("bitmap/zcode", bitmap)
		}
		return ui.Image(c, cached.(*ui.Bitmap)).Size(size, size).Shrink(0).Role(ui.RoleNone)
	}
	if t.Dark {
		return ui.Icon(c, providerSVG(name)).Size(size, size).Shrink(0).TextColor(t.TextMuted).Role(ui.RoleNone)
	}
	return ui.Image(c, providerSVG(name)).Size(size, size).Shrink(0).TextColor(t.TextMuted).Role(ui.RoleNone)
}

func iconButton(c *ui.Context, name, label string, disabled bool) ui.Element {
	return contentIconButton(c, label, disabled, func() { ui.Icon(c, svg(name)).Size(18, 18) })
}

// Rotate only the glyph; the shared hit area, focus ring and tooltip stay upright.
func rotatedIconButton(c *ui.Context, name, label string, degrees float32) ui.Element {
	return contentIconButton(c, label, false, func() { ui.Icon(c, svg(name)).Size(18, 18).Rotate(degrees) })
}

// contentIconButton is the one icon-only button of the window: a 28 DIP
// square with no face until hovered. Its hover and press only tint the
// face, so nothing moves under the pointer. label names it for assistive
// technology and shows as its tooltip.
func contentIconButton(c *ui.Context, label string, disabled bool, content func()) ui.Element {
	t := c.Theme()
	tn := toneFor(t, c.Preferences().HighContrast)
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(iconButtonSize, iconButtonSize).Shrink(0).Radius(6).
		Justify(ui.Center).AlignItems(ui.Center).TextColor(t.TextMuted).Cursor(ui.CursorPointer).Disabled(disabled)
	if !disabled {
		if b.Pressed() {
			b.Background(tn.pressed)
		} else if b.Hovered() {
			b.Background(tn.hover)
		}
	}
	return b.Children(content)
}

func (p *Panel) palette(c *ui.Context) *ui.Theme {
	dark := c.Theme().Dark
	if p.Theme == "light" {
		dark = false
	}
	if p.Theme == "dark" {
		dark = true
	}
	var t ui.Theme
	if c.Preferences().HighContrast {
		// The system's high contrast colors replace the palette entirely.
		t = *c.Theme()
	} else if dark {
		t = darkResultTheme
	} else {
		t = lightResultTheme
	}
	t.Font = systemFont
	t.FontSize = 14
	// Keep controls consistent with the shared native theme.
	t.Radius = 8
	t.Spacing = 4
	if scale := c.Preferences().TextScale; scale > 0 {
		t.FontSize *= scale
	}
	return &t
}

func (p *Panel) View(c *ui.Context) {
	t := p.palette(c)
	c.SetTheme(t)
	p.services = c.Services()
	p.errorVisibility.observe(p.Error)
	p.noticeVisibility.observe(p.Notice)
	p.speechVisibility.observe(p.SpeechErrorKey + "\x00" + p.SpeechError)
	p.selected = ""
	_, height := c.Size()
	// The framework splitter owns dragging, keyboard resizing and capture. Keep
	// an input-height preference so enlarging the window still gives space to reading.
	available := max(0, height-resultHeaderHeight-1)
	// The shared top region holds spacing or the quick toolbar. Its height
	// replaces the spacing, so the toolbar stays centered between both edges.
	topHeight := composerInset
	if p.Source == "quick-assistant" {
		topHeight = quickToolbarHeight
	}
	desired := p.composerHeight
	if desired == 0 {
		desired = 120 + topHeight
		if p.Source == "quick-assistant" {
			desired = max(desired, p.promptHeight(c, t)+topHeight+composerInset+18+8+iconButtonSize+2)
		}
	}
	minimum := 96 + topHeight
	composer := max(minimum, min(desired, available-72))
	reading := max(40, available-composer-1)
	// A draft keeps growing between replies from the Host, as the Markdown
	// view reveals its words. Follow it while the reader stays at the end,
	// as a log does; scrolling up stops following.
	if p.Busy && (p.Pending != nil || p.ResultPending != nil) && p.AtEnd() {
		p.FollowEnd()
	}
	// The native window owns its outer border/corners; only content is painted here.
	windowSurface(ui.Column(c), t).Children(func() {
		p.header(c, t)
		ui.Divider(c)
		split := ui.SplitVertical(c, &reading, func() { p.reading(c, t, reading) }, func() { p.composer(c, t) }).Grow(1).MinHeight(0).Label(p.T("调整输入框高度", "Resize message input"))
		if split.Changed() {
			p.composerHeight = max(minimum, min(available-reading-1, available-72))
		}
	})
}

// header keeps the drag handle at the exact center of the title bar: the
// two side groups always share the remaining width equally, so neither the
// title, the status text nor the buttons can move it (user requirement).
// Empty parts of the bar drag the window too, as a title bar does.
func (p *Panel) header(c *ui.Context, t *ui.Theme) {
	title := p.T("文字识别", "Text recognition")
	if p.Mode == "translate" {
		title = p.T("翻译", "Translation")
	}
	if p.Source == "selection" {
		title = p.selectionTitle()
	}
	if p.Source == "quick-assistant" {
		title = p.T("快捷助手", "Quick assistant")
	}
	ui.Row(c).Key("title").Height(resultHeaderHeight).AlignItems(ui.Center).Padding(0, 6).Gap(8).DragWindow().Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).AlignItems(ui.Center).Gap(6).DragWindow().Children(func() {
			pinName := p.T("固定结果窗", "Pin result window")
			pinRotation := float32(0)
			pinColor := t.TextMuted
			if !c.Preferences().HighContrast {
				pinColor = pinColor.Alpha(0.7)
			}
			if p.Pinned {
				pinName = p.T("取消固定", "Unpin result window")
				pinRotation = 45
				pinColor = t.Text
			}
			if rotatedIconButton(c, "pin", pinName, pinRotation).TextColor(pinColor).Clicked() {
				p.Pinned = !p.Pinned
				if p.Actions.Pin != nil {
					p.Actions.Pin(p.Pinned)
				}
			}
			ui.Text(c, title).FontSize(em(t, sizeLabel)).FontWeight(500).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Tooltip(title).MinWidth(0).Shrink(1).DragWindow()
		})
		// The handle shows where to grab; its 28 DIP box is the cursor's area.
		ui.Row(c).Size(iconButtonSize, iconButtonSize).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).DragWindow().Cursor(ui.CursorGrab).Label(p.T("拖动结果窗", "Drag result window")).Children(func() {
			ui.Icon(c, svg("grip")).Size(16, 16).TextColor(t.TextMuted).DragWindow().Cursor(ui.CursorGrab).Role(ui.RoleNone)
		})
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).AlignItems(ui.Center).Justify(ui.End).Gap(6).DragWindow().Children(func() {
			if p.Notice != "" && !p.noticeVisibility.dismissed {
				ui.Text(c, p.Notice).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Tooltip(p.Notice).MinWidth(0).Shrink(1).DragWindow()
				p.closeFeedback(c.Key("notice-close"), &p.noticeVisibility, p.T("关闭提示", "Dismiss notice"))
			}
			if iconButton(c, "close", p.T("关闭", "Close"), false).Clicked() && p.Actions.Close != nil {
				p.Actions.Close()
			}
		})
	})
}

// noteSelection records the text a Selectable container selects when it
// holds the keyboard focus, as the old focused-editor read did. Containers
// commit the release coordinate even between two frames, which a lone
// selectable paragraph does not; every selectable text is in one.
func (p *Panel) noteSelection(root ui.Element) {
	if !root.FocusWithin() {
		return
	}
	var parts []string
	for _, part := range root.SelectedTexts() {
		runes := []rune(part.Text)
		start, end := max(0, min(part.Start, len(runes))), max(0, min(part.End, len(runes)))
		if start < end {
			parts = append(parts, string(runes[start:end]))
		}
	}
	if len(parts) > 0 {
		p.selected = strings.Join(parts, "\n")
	}
}

// selectable builds a block whose text the user selects and copies as
// one, and notes its selection.
func (p *Panel) selectable(c *ui.Context, block ui.Element, children func()) ui.Element {
	block.Selectable().Children(children)
	p.noteSelection(block)
	return block
}

func (p *Panel) selectionTitle() string {
	if p.ActionLabel != "" {
		return p.ActionLabel
	}
	switch p.Action {
	case "translate":
		return p.T("翻译", "Translation")
	case "explain":
		return p.T("解释", "Explanation")
	case "summarize":
		return p.T("总结", "Summary")
	case "polish":
		return p.T("润色", "Polish")
	case "code":
		return p.T("代码", "Code")
	default:
		return p.T("划词助手", "Selection assistant")
	}
}

// The action model is also used for its follow-up conversation unless changed.
func (p *Panel) defaultSelection() ai.Selection {
	if p.ResultSelection.Provider != "" {
		return p.ResultSelection
	}
	if p.ActionSelection.Provider != "" {
		return p.ActionSelection
	}
	if p.Source == "selection" || p.Source == "quick-assistant" {
		return p.Configuration.Selected("default")
	}
	if selection := p.Configuration.Selected(p.Mode); selection.Provider != "" {
		return selection
	}
	return p.Configuration.Selected("chat")
}

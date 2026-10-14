// Package resultui provides the shared MyGo result and conversation view.
package resultui

import (
	"embed"
	"fmt"
	"github.com/egoist/mygo/ui"
	"math"
	"strings"
	"sync"
	"time"
	"winone/internal/ai"
	"winone/internal/resultwindow"
)

//go:embed assets/*.svg assets/providers/*.svg assets/providers/*.png assets/provider-bitmaps/*.png assets/logo.png
var assets embed.FS
var iconCache sync.Map

type Message struct {
	Role, Text, Model, Provider, ProviderName, ProviderIcon, Reasoning, State string
	ActionIcon, ActionLabel                                                   string
	ReasoningOpen                                                             bool
	ReasoningStartedAt, ReasoningDurationMs                                   int64
	ReasoningActive                                                           bool
	markdown                                                                  *markdownDocument
	thinkingInline                                                            *thinkingDocument
	thinkingScroll                                                            ui.ScrollState
	thinkingMeasuredText                                                      string
	thinkingMeasuredWidth, thinkingMeasuredHeight, thinkingMeasuredScale      float32
	thinkingMeasuredOpen                                                      bool
}
type Actions struct {
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
	avatars                                 map[string]*providerAvatar
	avatarClock                             uint64
	QuickReasoning                          *bool
	QuickActions                            []resultwindow.SelectionAction
	FocusComposer                           bool
	ContextReset, ShowComposerHint          bool
	composerEdited                          bool
	quickMore                               bool
	Mode, Language, Theme                   string
	Source, Action                          string
	ActionLabel                             string
	Original, Result, Prompt, Error, Notice string
	Target                                  string
	Busy, Pinned                            bool
	Recognizing                             bool
	Messages                                []Message
	Pending, ResultPending                  *Message
	ResultDetails                           Message
	Actions                                 Actions
	Configuration                           ai.Configuration
	ActionSelection                         ai.Selection
	ResultSelection                         ai.Selection
	Selection                               *ai.Selection
	TranslationSelection                    *ai.Selection
	TranslationModelChanged                 bool
	TranslationPickerOpen                   bool
	ModelPickerOpen                         bool
	modelPickerCollapsed                    map[string]bool
	scroll                                  ui.ScrollState
	composerHeight                          float32
	diagrams                                map[string]*diagramEntry
	diagramClock                            uint64
	SpeechKey, SpeechErrorKey, SpeechError  string
	SpeechLoading                           bool
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
func providerImage(c *ui.Context, t *ui.Theme, name string, size float32) *ui.Element {
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
func iconButton(c *ui.Context, name, label string) *ui.Element {
	return rotatedIconButton(c, name, label, 0)
}

// Rotate only the glyph; the shared hit area, focus ring and tooltip stay upright.
func rotatedIconButton(c *ui.Context, name, label string, degrees float32) *ui.Element {
	return contentIconButton(c, label, func() { ui.Icon(c, svg(name)).FontSize(18).Rotate(degrees) })
}

// Build exactly one glyph/loading indicator while preserving shared hit areas.
func contentIconButton(c *ui.Context, label string, content func()) *ui.Element {
	b := ui.Button(c, "").Label(label).Tooltip(label).Size(28, 28).Padding(5).Border(1, ui.Transparent).Cursor(ui.CursorPointer).Children(content)
	if !b.Hovered() && !b.FocusVisible() {
		b.Background(ui.Transparent)
	}
	return b
}
func (p *Panel) palette(c *ui.Context) *ui.Theme {
	dark := c.Theme().Dark
	if p.Theme == "light" {
		dark = false
	}
	if p.Theme == "dark" {
		dark = true
	}
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	if c.Preferences().HighContrast {
		copy := *c.Theme()
		t = &copy
	} else {
		t.Background = ui.Hex("#ffffff")
		t.Surface = t.Background
		t.SurfaceHover = ui.Hex("#f5f5f5")
		t.Text = ui.Hex("#171717")
		t.TextMuted = ui.Hex("#737373")
		t.Border = ui.Hex("#d4d4d4")
		if dark {
			t.Background = ui.Hex("#171717")
			t.Surface = t.Background
			t.SurfaceHover = ui.Hex("#262626")
			t.Text = ui.Hex("#fafafa")
			t.TextMuted = ui.Hex("#a3a3a3")
			t.Border = ui.Hex("#404040")
		}
		t.Accent = t.Text
		t.AccentText = t.Background
	}
	t.Font = "Segoe UI"
	t.FontSize = 14
	t.Radius = 8
	t.Spacing = 4
	if scale := c.Preferences().TextScale; scale > 0 {
		t.FontSize *= scale
	}
	return t
}

const resultHeaderHeight float32 = 40

func (p *Panel) View(c *ui.Context) {
	t := p.palette(c)
	c.SetTheme(t)
	_, height := c.Size()
	// The framework splitter owns dragging, keyboard resizing and capture. Keep
	// an input-height preference so enlarging the window still gives space to reading.
	available := max(0, height-resultHeaderHeight-1)
	desired := p.composerHeight
	if desired == 0 {
		desired = 120
		if p.Source == "quick-assistant" {
			desired = 170
		}
	}
	minimum := float32(96)
	if p.Source == "quick-assistant" {
		minimum = 150
	}
	composer := max(minimum, min(desired, available-72))
	reading := max(40, available-composer-1)
	// The native window owns its outer border/corners; only content is painted here.
	windowSurface(ui.Column(c), t).Children(func() {
		p.header(c, t)
		ui.Divider(c)
		split := ui.SplitVertical(c, &reading, func() { p.reading(c, t) }, func() { p.composer(c, t) }).Grow(1).MinHeight(0).Label(p.T("调整输入框高度", "Resize message input"))
		if split.Changed() {
			p.composerHeight = max(minimum, min(available-reading-1, available-72))
		}

	})
}
func (p *Panel) reading(c *ui.Context, t *ui.Theme) {
	ui.Scroll(c).Key("results").Grow(1).TrackScroll(&p.scroll).Padding(20, 28, 32, 28).Children(func() {
		// Center the reading column, not the text. Narrow windows use all
		// available space; wide windows keep a generous, bounded line length.
		ui.Column(c).FillWidth().MaxWidth(840).AlignSelf(ui.Center).Gap(28).Children(func() {
			background := ui.Hex("#eeeeee")
			if t.Dark {
				background = ui.Hex("#252525")
			}
			if c.Preferences().HighContrast {
				background = t.Surface
			}
			// Bleed the surface into the existing gutter: its text keeps exactly
			// the same reading width and action edge as the conversation below.
			if p.Source != "quick-assistant" {
				ui.Column(c).Key("result-surface").Margin(0, -20).Padding(20).Gap(16).Radius(10).Background(background).Children(func() {
					if p.Source != "selection" && p.Mode == "translate" && p.Original != "" {
						p.original(c, t)
						ui.Divider(c)
					}
					p.result(c, t)
				})
			} else if len(p.Messages) == 0 && p.Pending == nil {
				ui.Column(c).Gap(10).Padding(20, 0).Children(func() {
					ui.Text(c, p.T("有什么可以帮你？", "How can I help?")).FontSize(20)
					ui.Text(c, p.T("直接发送消息，或输入内容后选择翻译、解释等功能。", "Send a message, or enter content and choose Translate, Explain, or another action.")).TextColor(t.TextMuted)
				})
			}
			if len(p.Messages) > 0 || p.Pending != nil {
				p.conversation(c, t)
			}
			if p.Busy && p.Pending == nil && p.ResultPending == nil {
				ui.Spinner(c).Size(16, 16).AlignSelf(ui.Start).Label(p.T("正在处理…", "Working…"))
			}
			p.errorFeedback(c, t, "general")
		})
	})
}
func (p *Panel) header(c *ui.Context, t *ui.Theme) {
	width, _ := c.Size()
	// Keep symmetric side slots, but size them from the actual notice. The
	// former fixed 60-DIP slot left only 28 DIPs after the pin button, clipping
	// even "已复制" in a wide window. Long notices may still elide in a narrow
	// window; the pin, close button and drag target always retain their space.
	noticeWidth, _ := c.MeasureText(0, ui.Span{Text: p.Notice})
	sideWidth := min(max(float32(60), 28+4+noticeWidth), max(float32(60), (width-24-16-24)/2))
	centerWidth := max(0, width-24-2*sideWidth-16)
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
	ui.Row(c).Key("title").Height(resultHeaderHeight).AlignItems(ui.Center).Padding(6, 12).Gap(8).Children(func() {
		ui.Row(c).Width(sideWidth).MinWidth(0).AlignItems(ui.Center).Gap(4).Children(func() {
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
			if p.Notice != "" {
				ui.Text(c, p.Notice).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(p.Notice).TextColor(t.TextMuted)
			}
		})
		ui.Row(c).Width(centerWidth).MinWidth(0).AlignItems(ui.Center).Justify(ui.Center).DragWindow().Cursor(ui.CursorGrab).Children(func() {
			ui.Row(c).MaxWidth(centerWidth).Height(28).MinWidth(0).AlignItems(ui.Center).Gap(6).DragWindow().Cursor(ui.CursorGrab).Children(func() {
				ui.Icon(c, svg("grip")).Size(16, 16).Shrink(0).TextColor(t.TextMuted).Label(p.T("拖动结果窗", "Drag result window")).DragWindow().Cursor(ui.CursorGrab)
				ui.Text(c, title).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(title).DragWindow().Cursor(ui.CursorGrab)
			})
		})
		ui.Row(c).Width(sideWidth).MinWidth(0).AlignItems(ui.Center).Justify(ui.End).Gap(4).Children(func() {
			if iconButton(c, "close", p.T("关闭", "Close")).Clicked() && p.Actions.Close != nil {
				p.Actions.Close()
			}
		})
	})
}

func (p *Panel) original(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Key("original-preview").FillWidth().Gap(3).Children(func() {
		ui.Row(c).Height(28).Gap(8).Children(func() {
			ui.Text(c, p.T("原文", "Original")).FontSize(13).TextColor(t.TextMuted).Grow(1)
			p.speechButton(c, "original", p.Original, p.T("朗读原文", "Read original aloud"), false, false)
			if iconButton(c, "copy", p.T("复制原文", "Copy original")).Clicked() && p.Actions.Copy != nil {
				p.Actions.Copy(p.Original)
			}
		})
		// OCR hard wraps belong to the copied source, not to this two-line preview.
		preview := strings.Join(strings.Fields(p.Original), " ")
		ui.Text(c, preview).Key("original-body").FillWidth().TextColor(t.TextMuted).LineHeight(1.5).MaxLines(2).Ellipsis("…").Selectable()
		p.speechFeedback(c, t, "original")
	})
}
func (p *Panel) result(c *ui.Context, t *ui.Theme) {
	// The captured result is the primary reading task; follow-up answers use
	// the regular body scale and never replace or collapse this section.
	resultTheme := *t
	resultTheme.FontSize++
	ui.Column(c).Key("initial-result").Gap(10).Children(func() {
		ui.Row(c).Key("result-actions").Gap(6).MinHeight(30).Children(func() {
			label, copyLabel, text := p.T("识别结果", "OCR result"), p.T("复制原文", "Copy original"), p.Original
			if p.Mode == "translate" {
				label, copyLabel, text = p.T("译文", "Translation"), p.T("复制译文", "Copy translation"), p.Result
				if p.Recognizing {
					label, copyLabel = p.T("识别原文", "Recognizing source"), p.T("复制原文", "Copy original")
				}
			}
			if text == "" && p.ResultPending != nil {
				text = p.ResultPending.Text
			}
			if p.Source == "selection" {
				copyLabel, text = p.T("复制结果", "Copy result"), p.Result
				details := &p.ResultDetails
				if p.ResultPending != nil {
					details = p.ResultPending
					if text == "" {
						text = details.Text
					}
				}
				if p.Mode == "translate" {
					p.translationModelPicker(c, t)
					ui.Spacer(c)
				} else {
					p.messageIdentity(c, t, details).Grow(1)
				}
			} else {
				ui.Text(c, label).FontSize(14).FontWeight(600).TextColor(t.Text)
			}
			if p.Source != "selection" && p.Mode == "translate" {
				p.targetLanguage(c)
				ui.Spacer(c)
			} else if p.Source != "selection" {
				ui.Spacer(c)
			}
			if iconButton(c, "retry", p.T("重试", "Retry")).Disabled(p.Busy).Clicked() && p.Actions.RetryResult != nil {
				p.Actions.RetryResult()
			}
			speechLabel := p.T("朗读原文", "Read original aloud")
			if p.Mode == "translate" {
				speechLabel = p.T("朗读译文", "Read translation aloud")
			}
			if p.Source == "selection" {
				speechLabel = p.T("朗读结果", "Read result aloud")
			}
			p.speechButton(c, "result", text, speechLabel, p.Mode == "translate" || p.Source == "selection", p.Busy && p.ResultPending != nil || p.Recognizing)
			if iconButton(c, "copy", copyLabel).Disabled(text == "").Clicked() && p.Actions.Copy != nil {
				p.Actions.Copy(text)
			}
		})
		if p.Source != "selection" && p.Mode == "translate" {
			p.translationModelPicker(c, t)
		}
		if p.Result != "" {
			p.markdown(c, &resultTheme, p.Result, &p.ResultDetails.markdown, false)
		}
		if p.ResultPending != nil {
			if p.Result != "" {
				ui.Divider(c).Margin(6, 0)
				ui.Text(c, p.T("重新生成", "Regenerating")).TextColor(t.TextMuted)
			}
			if p.ResultPending.Text != "" {
				if p.Recognizing && p.Mode == "translate" {
					ui.Text(c, strings.Join(strings.Fields(p.ResultPending.Text), " ")).FillWidth().MaxLines(2).Ellipsis("…").Selectable()
				} else {
					p.markdown(c, &resultTheme, p.ResultPending.Text, &p.ResultPending.markdown, p.Busy)
				}
			}
			p.draftStatus(c, t, p.ResultPending)
		}
		p.errorFeedback(c, t, "result")
		p.speechFeedback(c, t, "result")
	})
}

func (p *Panel) errorFeedback(c *ui.Context, t *ui.Theme, area string) {
	if p.Error == "" {
		return
	}
	// Keep request failures next to their draft or initial result. Errors
	// without either context retain the shared feedback area below the content.
	location := "general"
	if p.ResultPending != nil || len(p.Messages) == 0 && p.Pending == nil {
		location = "result"
	} else if p.Pending != nil {
		location = "reply"
	}
	if location == area {
		ui.Text(c, p.Error).FillWidth().Selectable().LineHeight(1.5).TextColor(t.Danger)
	}
}
func (p *Panel) reasoning(c *ui.Context, t *ui.Theme, message *Message) {
	if message.Reasoning == "" || !p.reasoningVisible() {
		return
	}
	label := p.T("思考完成", "Thought")
	if message.ReasoningActive {
		label = p.T("正在思考…", "Thinking…") + " · " + thinkingDuration(time.Now().UnixMilli()-message.ReasoningStartedAt)
		c.After(500 * time.Millisecond)
	} else if message.State == "stopped" {
		label = p.T("思考已停止", "Thinking stopped")
	}
	if !message.ReasoningActive && message.ReasoningStartedAt > 0 {
		label = p.T("已思考 ", "Thought for ") + thinkingDuration(message.ReasoningDurationMs)
		if message.State == "stopped" {
			label = p.T("思考已停止 · ", "Thinking stopped · ") + thinkingDuration(message.ReasoningDurationMs)
		}
	}
	thinkingSection := ui.Column(c).FillWidth()
	thinkingSection.Children(func() {
		parts := ui.CollapsibleBase(c, &message.ReasoningOpen)
		trigger := parts.Trigger.Label(p.T("思考过程", "Reasoning")).FillWidth().MinHeight(25).AlignItems(ui.Center).Gap(6).Padding(2, 8).Radius(5).TextColor(t.TextMuted)
		if trigger.Hovered() {
			trigger.Background(t.SurfaceHover)
		}
		trigger.Children(func() {
			icon := "thinking-complete"
			if message.ReasoningActive || message.State == "stopped" {
				icon = "thinking"
			}
			ui.Icon(c, svg(icon)).Size(14, 14).Shrink(0)
			ui.Text(c, label).Grow(1).MinWidth(0).FontSize(14)
			ui.Icon(c, svg("chevron")).Size(14, 14).Shrink(0).Rotate(-90 + 90*parts.Progress())
		})
		parts.Panel(func() {
			if message.thinkingInline == nil || message.thinkingInline.raw != message.Reasoning {
				message.thinkingInline = &thinkingDocument{message.Reasoning, thinkingTokens(message.Reasoning)}
			}
			width, height := c.Size()
			if message.ReasoningActive && message.thinkingScroll.Y >= message.thinkingScroll.MaxY-120 {
				message.thinkingScroll.Y = math.MaxFloat32
			}
			ui.Scroll(c).Label(p.T("思考内容", "Thinking content")).FillWidth().MaxHeight(min(float32(320), height*0.4)).TrackScroll(&message.thinkingScroll).Children(func() {
				ui.Row(c).FillWidth().Gap(12).Margin(4, 0, 0, 0).Padding(6, 0, 0, 0).Children(func() {
					ui.Box(c).Width(2).AlignSelf(ui.Stretch).Background(t.Border)
					ui.RichText(c).Grow(1).MinWidth(0).FontSize(14).TextColor(t.TextMuted).LineHeight(1.6).Selectable().Children(func() {
						for _, token := range message.thinkingInline.tokens {
							span := ui.Text(c, token.Value)
							if token.Kind == "code" {
								span.Font("Consolas")
							} else {
								span.Italic()
								if token.Kind == "strong" {
									span.FontWeight(500)
								}
							}
						}
					})
				})
			})
			// One bounded follow-up sees the measured scroll range, including
			// systems with reduced motion where no animation supplies frames.
			if !message.thinkingMeasuredOpen || message.thinkingMeasuredText != message.Reasoning || message.thinkingMeasuredWidth != width || message.thinkingMeasuredHeight != height || message.thinkingMeasuredScale != c.Preferences().TextScale {
				message.thinkingMeasuredText = message.Reasoning
				message.thinkingMeasuredWidth = width
				message.thinkingMeasuredHeight = height
				message.thinkingMeasuredScale = c.Preferences().TextScale
				message.thinkingMeasuredOpen = true
				c.After(time.Millisecond)
			}
		})
		if !message.ReasoningOpen {
			message.thinkingMeasuredOpen = false
		}
	})
}

// This content stack owns the complete thinking-to-answer gap. Neither the
// thinking body nor the surrounding identity/action layout adds to it.
func (p *Panel) replyContent(c *ui.Context, t *ui.Theme, message *Message, working bool) {
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		p.reasoning(c, t, message)
		if message.Text != "" {
			p.markdown(c, t, message.Text, &message.markdown, working)
		}
	})
}
func (p *Panel) draftStatus(c *ui.Context, t *ui.Theme, message *Message) {
	if message.State == "running" || message.State == "" {
		ui.Spinner(c).Size(16, 16).AlignSelf(ui.Start).Label(p.T("正在生成…", "Generating…"))
		return
	}
	label := p.T("正在生成…", "Generating…")
	if message.State == "stopped" {
		label = p.T("已停止 · 内容未完成", "Stopped · Incomplete")
	}
	if message.State == "failed" {
		label = p.T("生成中断 · 内容未完成", "Interrupted · Incomplete")
	}
	ui.Text(c, label).TextColor(t.TextMuted)
}
func (p *Panel) replyActions(c *ui.Context, t *ui.Theme, message *Message, key string, retry bool) {
	// Keep actions after their answer and on a shared right edge. They remain
	// visible for keyboard users and each callback owns this exact message.
	ui.Row(c).FillWidth().Gap(4).Children(func() {
		ui.Spacer(c)
		if retry {
			if iconButton(c, "retry", p.T("重试回复", "Retry reply")).Clicked() && p.Actions.Retry != nil {
				p.Actions.Retry()
			}
		}
		p.speechButton(c, key, message.Text, p.T("朗读回复", "Read reply aloud"), true, message.State == "running" || key == "pending-reply" && p.Busy)
		if iconButton(c, "copy", p.T("复制回复", "Copy reply")).Disabled(message.Text == "").Clicked() && p.Actions.Copy != nil {
			p.Actions.Copy(message.Text)
		}
	})
	p.speechFeedback(c, t, key)
}

func (p *Panel) messageIdentity(c *ui.Context, t *ui.Theme, message *Message) *ui.Element {
	// Callers choose horizontal growth. Grow on a direct Column child would
	// consume vertical space and could overlap the following answer.
	return ui.Row(c).MinWidth(0).MinHeight(24).AlignItems(ui.Center).Gap(6).Children(func() {
		if message.Model != "" || message.ProviderName != "" {
			p.providerImage(c, t, p.messageProviderIcon(message), 18)
		}
		if message.ProviderName != "" {
			ui.Text(c, message.ProviderName).SingleLine().Ellipsis("…").Tooltip(message.ProviderName).MinWidth(0).MaxWidthPercent(40).FontSize(13).TextColor(t.TextMuted)
			if message.Model != "" {
				ui.Text(c, "·").TextColor(t.TextMuted).Shrink(0)
			}
		}
		ui.Text(c, message.Model).SingleLine().Ellipsis("…").Tooltip(message.Model).MinWidth(0).FontSize(14).TextColor(t.Text).Grow(1)
	})
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
func (p *Panel) conversation(c *ui.Context, t *ui.Theme) {
	for index := range p.Messages {
		message := &p.Messages[index]
		if message.Role == "user" {
			ui.Column(c).Key(index).Label(p.T("用户", "User")).AlignSelf(ui.End).MaxWidthPercent(85).Gap(4).Margin(4, 0).Children(func() {
				// The action belongs to this bubble; its marker follows the bubble's
				// left edge even when a short message is aligned to the right.
				if message.ActionIcon != "" {
					color := t.TextMuted
					if !c.Preferences().HighContrast {
						color = color.Alpha(0.65)
					}
					ui.Icon(c, svg(message.ActionIcon)).AlignSelf(ui.Start).Size(14, 14).TextColor(color).Label(p.T("快捷操作：", "Quick action: ") + message.ActionLabel).Tooltip(message.ActionLabel)
				}
				ui.Column(c).Label(p.T("用户消息气泡", "User message bubble")).Padding(10, 12).Background(t.SurfaceHover).Radius(10).Children(func() {
					ui.Text(c, message.Text).Selectable().FontSize(14).LineHeight(1.5)
				})
			})
		} else {
			ui.Column(c).Key(index).FillWidth().Gap(8).Margin(4, 0).Children(func() {
				p.messageIdentity(c, t, message).FillWidth().Margin(0, 0, 8, 0)
				p.replyContent(c, t, message, false)
				p.replyActions(c, t, message, fmt.Sprintf("reply-%d", index), index == len(p.Messages)-1 && !p.Busy && p.Pending == nil)
			})
		}
	}
	if p.Pending != nil {
		ui.Column(c).Key("pending-reply").FillWidth().Gap(8).Margin(4, 0).Children(func() {
			p.messageIdentity(c, t, p.Pending).FillWidth().Margin(0, 0, 8, 0)
			p.replyContent(c, t, p.Pending, p.Busy)
			p.replyActions(c, t, p.Pending, "pending-reply", !p.Busy)
			p.draftStatus(c, t, p.Pending)
			p.errorFeedback(c, t, "reply")
		})
	}
	if len(p.Messages) > 0 && p.Messages[len(p.Messages)-1].Role == "user" && !p.Busy && p.Pending == nil {
		if iconButton(c, "retry", p.T("重试回复", "Retry reply")).AlignSelf(ui.Start).Clicked() && p.Actions.Retry != nil {
			p.Actions.Retry()
		}
	}
}
func (p *Panel) composer(c *ui.Context, t *ui.Theme) {
	// Give the quick-action strip its complete region above the input. Its
	// symmetric inset centers the icons without moving or shrinking the input.
	topPadding := float32(12)
	if p.Source == "quick-assistant" {
		topPadding = 0
	}
	ui.Column(c).Fill().MaxWidth(872).AlignSelf(ui.Center).Padding(topPadding, 12, 12, 12).Children(func() {
		if p.Source == "quick-assistant" {
			p.quickToolbar(c, t)
		}
		box := ui.Column(c).Grow(1).MinHeight(0).Key("composer").Padding(10, 12).Gap(8).Border(1, t.Border).Radius(12).Children(func() {
			input := ui.TextArea(c, &p.Prompt).Placeholder(p.T("围绕结果继续提问…", "Ask about this result…")).Label(p.T("继续提问", "Ask a follow-up")).FontSize(14).LineHeight(1.5).Grow(1).MinHeight(24).Padding(0).Border(0, ui.Transparent).Background(ui.Transparent)
			if p.ContextReset {
				input.Placeholder(p.T("输入新一轮消息…", "Start a new conversation…"))
			}
			if p.Source == "quick-assistant" {
				input.Placeholder(p.T("输入消息…", "Message…")).Label(p.T("输入消息", "Message"))
			}
			if p.FocusComposer {
				input.Focus()
				p.FocusComposer = false
			}
			composing := input.Composing()
			if input.Changed() || composing {
				p.ShowComposerHint = false
				p.composerEdited = true
			}
			input.HandleInput(func(event ui.InputEvent) bool {
				if event.Kind != ui.InputKeyDown {
					return false
				}
				if composing {
					return true
				}
				if event.Key == ui.KeyEnter && event.Mods&ui.Shift == 0 {
					if !p.Busy && strings.TrimSpace(p.Prompt) != "" && p.Actions.Send != nil {
						p.Actions.Send(p.Prompt)
					}
					return true
				}
				return false
			})
			if p.ShowComposerHint {
				ui.Text(c, p.T("Enter 发送，Shift+Enter 换行", "Enter to send, Shift+Enter for a new line")).FontSize(13).TextColor(t.TextMuted)
			}
			ui.Row(c).Height(28).Gap(8).Children(func() {
				p.modelPicker(c, t)
				ui.Box(c).Grow(1).MinWidth(0)
				if p.expandedReasoning() {
					if iconButton(c, "collapse", p.T("收起思考过程", "Collapse reasoning")).Clicked() {
						p.collapseReasoning()
					}
				}
				if p.Busy {
					if iconButton(c, "stop", p.T("停止", "Stop")).Clicked() && p.Actions.Cancel != nil {
						p.Actions.Cancel()
					}
				} else {
					send := ui.PrimaryButton(c, "").Label(p.T("发送", "Send")).Tooltip(p.T("发送", "Send")).Disabled(strings.TrimSpace(p.Prompt) == "" || composing).Size(28, 28).Padding(6).Radius(99).Cursor(ui.CursorPointer).Children(func() { ui.Icon(c, svg("send")).FontSize(16) })
					if send.Clicked() && !composing && strings.TrimSpace(p.Prompt) != "" && p.Actions.Send != nil {
						p.Actions.Send(p.Prompt)
					}
				}
			})
		})
		if box.FocusWithin() {
			box.Border(1, t.Focus)
		}
		if (p.Source == "selection" || p.Source == "quick-assistant" || p.Actions.ClearAll != nil || p.Actions.Restart != nil) && !p.ModelPickerOpen && !p.TranslationPickerOpen && !p.quickMore {
			// MyGo's anchored overlay follows composer resizing without taking
			// space from the editor or being clipped by the split-pane boundary.
			gap := float32(8)
			if p.Source == "quick-assistant" {
				gap += 42 // Keep the complete quick-action strip unobstructed.
			}
			ui.Overlay(c, func() {
				ui.Column(c).Key("conversation-controls").AttachTo(box, ui.AnchorTopRight, ui.AnchorBottomRight).Margin(0, 0, gap, 0).Gap(4).Children(func() { p.conversationControls(c, t) })
			})
		}
	})
}
func (p *Panel) conversationControls(c *ui.Context, t *ui.Theme) {
	// Opaque button surfaces keep each floating glyph legible over old replies.
	clear := iconButton(c, "clear", p.T("清空全部", "Clear all")).TextColor(t.TextMuted).Radius(99).Border(1, t.Border).Tooltip(p.T("清空全部：结果、输入及对话上下文", "Clear all results, input and conversation context"))
	if !clear.Hovered() && !clear.FocusVisible() {
		clear.Background(t.Background)
	}
	if clear.Clicked() && p.Actions.ClearAll != nil {
		p.Actions.ClearAll()
	}
	restart := iconButton(c, "restart", p.T("重新开始", "Restart")).TextColor(t.TextMuted).Radius(99).Border(1, t.Border).Tooltip(p.T("重新开始：清理上下文，保留旧结果供参考", "Restart with a fresh context; keep earlier results for reference"))
	if !restart.Hovered() && !restart.FocusVisible() {
		restart.Background(t.Background)
	}
	if restart.Clicked() && p.Actions.Restart != nil {
		p.Actions.Restart()
	}
}

func (p *Panel) expandedReasoning() bool {
	if !p.reasoningVisible() {
		return false
	}
	if p.Pending != nil && p.Pending.ReasoningOpen && p.Pending.thinkingScroll.MaxY > 1 {
		return true
	}
	for _, message := range p.Messages {
		if message.ReasoningOpen && message.thinkingScroll.MaxY > 1 {
			return true
		}
	}
	return false
}
func (p *Panel) collapseReasoning() {
	if p.Pending != nil {
		p.Pending.ReasoningOpen = false
	}
	for i := range p.Messages {
		p.Messages[i].ReasoningOpen = false
	}
}
func (p *Panel) targetLanguage(c *ui.Context) {
	codes := []string{"auto", "zh-CN", "zh-TW", "en", "ja", "ko"}
	labels := []string{p.T("智能互译", "Smart translate"), p.T("简体中文", "Chinese"), p.T("繁体中文", "Traditional Chinese"), "English", "日本語", "한국어"}
	selected := labels[0]
	for index, code := range codes {
		if code == p.Target {
			selected = labels[index]
		}
	}
	if ui.Select(c, &selected, labels).Label(p.T("目标语言", "Target language")).Disabled(p.Busy).MinWidth(0).MaxWidth(160).FontSize(13).TextColor(c.Theme().TextMuted).Border(0, ui.Transparent).Background(ui.Transparent).Changed() {
		for index, label := range labels {
			if selected == label {
				p.Target = codes[index]
				if p.Actions.Target != nil {
					p.Actions.Target(p.Target)
				}
				break
			}
		}
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

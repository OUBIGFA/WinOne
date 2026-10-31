package resultui

import "github.com/egoist/mygo/ui"

// The result window's design tokens. Values follow the settings window's
// neutral palette (apps/desktop-mygo/kit/theme.go and
// docs/WinOne设置设计语言.md §4.3): the interface layer is gray only;
// Danger, Success and the provider artwork keep their colors because they
// carry meaning. desktop-mygo imports this module, so the values are kept
// here rather than imported; change both together.
var (
	lightResultTheme = ui.Theme{
		Background:     ui.Hex("#fcfcfc"),
		Surface:        ui.Hex("#f3f3f3"),
		SurfaceHover:   ui.Hex("#ebebeb"),
		SurfacePressed: ui.Hex("#e2e2e2"),
		Border:         ui.Hex("#d6d6d6"),
		Text:           ui.Hex("#262626"),
		TextMuted:      ui.Hex("#5e5e5e"),
		Accent:         ui.Hex("#2e2e2e"),
		AccentHover:    ui.Hex("#454545"),
		AccentPressed:  ui.Hex("#1c1c1c"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#c42b1c"),
		Warning:        ui.Hex("#9d5d00"),
		Success:        ui.Hex("#0f7b0f"),
		Selection:      ui.RGBA(46, 46, 46, 0.18),
		Focus:          ui.RGBA(46, 46, 46, 0.6),
		Inverse:        ui.Hex("#2b2b2b"),
		InverseText:    ui.Hex("#fafafa"),
		Scrollbar:      ui.RGBA(0, 0, 0, 0.3),
	}
	darkResultTheme = ui.Theme{
		Dark:           true,
		Background:     ui.Hex("#191919"),
		Surface:        ui.Hex("#2b2b2b"),
		SurfaceHover:   ui.Hex("#333333"),
		SurfacePressed: ui.Hex("#3b3b3b"),
		Border:         ui.Hex("#3d3d3d"),
		Text:           ui.Hex("#e9e9e9"),
		TextMuted:      ui.Hex("#adadad"),
		Accent:         ui.Hex("#e6e6e6"),
		AccentHover:    ui.Hex("#ffffff"),
		AccentPressed:  ui.Hex("#cccccc"),
		AccentText:     ui.Hex("#1a1a1a"),
		Danger:         ui.Hex("#ff99a4"),
		Warning:        ui.Hex("#fce100"),
		Success:        ui.Hex("#6ccb5f"),
		Selection:      ui.RGBA(233, 233, 233, 0.22),
		Focus:          ui.RGBA(233, 233, 233, 0.65),
		Inverse:        ui.Hex("#e9e9e9"),
		InverseText:    ui.Hex("#1a1a1a"),
		Scrollbar:      ui.RGBA(255, 255, 255, 0.3),
	}
)

// systemFont is Windows' interface face with the Chinese face DirectWrite
// would pick anyway named, so both languages keep one weight scale.
const (
	systemFont = "Segoe UI Variable Text, Segoe UI, Microsoft YaHei UI"
	monoFont   = "Cascadia Mono, Consolas, Microsoft YaHei UI"
)

// tones are the colors ui.Theme has no slot for.
type tones struct {
	// region is the common background of the initial OCR/translation
	// result, which separates the task result from the conversation.
	region ui.Color
	// bubble is the face of the user's own messages.
	bubble ui.Color
	// hover and pressed tint ghost buttons over any of the faces above.
	hover, pressed ui.Color
	// free colors Magpie's Free model badge, a data color.
	free, freeBackground ui.Color
}

func toneFor(t *ui.Theme, highContrast bool) tones {
	switch {
	case highContrast:
		return tones{region: t.Surface, bubble: t.Surface, hover: t.SurfaceHover, pressed: t.SurfacePressed, free: t.Text, freeBackground: t.SurfaceHover}
	case t.Dark:
		return tones{region: ui.Hex("#232323"), bubble: ui.Hex("#2b2b2b"), hover: t.Text.Alpha(0.09), pressed: t.Text.Alpha(0.14), free: ui.Hex("#86efac"), freeBackground: ui.Hex("#143322")}
	}
	return tones{region: ui.Hex("#efefef"), bubble: ui.Hex("#ebebeb"), hover: t.Text.Alpha(0.08), pressed: t.Text.Alpha(0.13), free: ui.Hex("#166534"), freeBackground: ui.Hex("#dcfce7")}
}

// Type roles in DIPs at 100% text size; em scales them with the system
// text size, which the theme's FontSize already carries. One family, few
// steps, no weight above 600 (docs/WinOne设置设计语言.md §4.1).
const (
	sizeMeta   = 13 // section labels, provider/model, hints, notices
	sizeLabel  = 14 // window title, buttons, reasoning
	sizeBody   = 15 // replies, the user's messages and the input
	sizeResult = 16 // the initial OCR/translation result, the main task
	sizeEmpty  = 18 // the Quick assistant's empty-state heading
	sizeBadge  = 12 // pills inside the model picker
)

func em(t *ui.Theme, size float32) float32 { return size * t.FontSize / 14 }

// Layout of the shared template, one control point per gap: every source
// (OCR, translation, selection, Quick assistant) uses these.
const (
	resultHeaderHeight float32 = 40
	// readingMaxWidth bounds the reading column on wide windows; narrow
	// windows use all the room inside the gutters.
	readingMaxWidth float32 = 760
	// gutter is the room between the window edge and the text; bleed is
	// how far the initial result's region and the input box extend into
	// it (and their inner padding), so every text starts on one line and
	// every face on another.
	gutter float32 = 28
	bleed  float32 = 16
	// composerInset is the composer's room at its sides and bottom, and
	// the least room above the input box: the input box's face lines up
	// with the result region's.
	composerInset      float32 = gutter - bleed
	quickToolbarHeight float32 = 42
	iconButtonSize     float32 = 28
)

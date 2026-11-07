package resultui

import (
	"github.com/egoist/mygo/ui"
	"winone/modules/nativeui/design"
)

// Independently hosted windows use the same theme as the main workspace.
var (
	lightResultTheme = design.Base(false)
	darkResultTheme  = design.Base(true)
)

const (
	systemFont = design.SystemFont
	monoFont   = design.MonoFont
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
		return tones{region: design.Colors(true).Panel, bubble: t.Surface, hover: t.Text.Alpha(0.09), pressed: t.Text.Alpha(0.14), free: ui.Hex("#86efac"), freeBackground: ui.Hex("#143322")}
	}
	return tones{region: t.Surface, bubble: t.SurfaceHover, hover: t.Text.Alpha(0.08), pressed: t.Text.Alpha(0.13), free: ui.Hex("#166534"), freeBackground: ui.Hex("#dcfce7")}
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

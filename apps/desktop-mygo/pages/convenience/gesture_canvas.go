package convenience

import (
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/modules/stroke"
)

// React source: features/mouse-gestures-canvas.tsx.

type point struct{ x, y float32 }

// previewPoints integrates Stroke's 128 recorded vectors into a trail, as
// Stroke.Configure.DrawGesture does; a gesture without vectors shows its
// points. This plots the data; it does not recognize anything.
func previewPoints(g stroke.Gesture) []point {
	if len(g.Vectors) > 0 {
		points := []point{{0, 0}}
		for i := 1; i < len(g.Vectors); i++ {
			v := g.Vectors[i]
			dx, dy := float32(0), float32(0)
			if len(v) >= 2 {
				dx, dy = float32(v[0])/127, float32(v[1])/127
			}
			last := points[i-1]
			points = append(points, point{last.x + dx, last.y + dy})
		}
		return points
	}
	points := make([]point, len(g.Points))
	for i, p := range g.Points {
		points[i] = point{float32(p.X), float32(p.Y)}
	}
	return points
}

var penColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// strokeColor is the pen's color at opacity; an invalid color shows gray.
func strokeColor(hex string, opacity float64) ui.Color {
	c := ui.RGB(128, 128, 128)
	if penColor.MatchString(hex) {
		c = ui.Hex(hex)
	}
	c.A = uint8(math.Round(255 * math.Max(0, math.Min(1, opacity))))
	return c
}

// paintTrail draws points in r: scaled to fit within padding when fit,
// else as recorded; a trail ends in an arrowhead showing its direction.
func paintTrail(p *ui.Painter, r ui.Rect, points []point, color ui.Color, fit bool, width, padding float32) {
	if len(points) < 2 || width == 0 || color.A == 0 {
		return
	}
	values := points
	if fit {
		minX, minY, maxX, maxY := points[0].x, points[0].y, points[0].x, points[0].y
		for _, pt := range points {
			minX, minY, maxX, maxY = min(minX, pt.x), min(minY, pt.y), max(maxX, pt.x), max(maxY, pt.y)
		}
		w, h := maxX-minX, maxY-minY
		scale := min((r.W-padding*2)/max(w, 1), (r.H-padding*2)/max(h, 1))
		values = make([]point, len(points))
		for i, pt := range points {
			values[i] = point{(pt.x-minX)*scale + (r.W-w*scale)/2, (pt.y-minY)*scale + (r.H-h*scale)/2}
		}
	}
	path := new(ui.Path).Cap(ui.RoundCap).Join(ui.RoundJoin)
	path.MoveTo(r.X+values[0].x, r.Y+values[0].y)
	for _, pt := range values[1:] {
		path.LineTo(r.X+pt.x, r.Y+pt.y)
	}
	p.StrokePath(path, width, color)
	end, prior := values[len(values)-1], values[len(values)-2]
	angle := math.Atan2(float64(end.y-prior.y), float64(end.x-prior.x))
	head := new(ui.Path)
	head.MoveTo(r.X+end.x, r.Y+end.y)
	for _, turn := range []float64{-0.4, 0.4} {
		head.LineTo(r.X+end.x-10*float32(math.Cos(angle+turn)), r.Y+end.y-10*float32(math.Sin(angle+turn)))
	}
	head.Close()
	p.FillPath(head, color)
}

// gestureThumbnail is a scene row's small trail.
func gestureThumbnail(c *ui.Context, g stroke.Gesture, color string) ui.Element {
	points := previewPoints(g)
	ink := strokeColor(color, 1)
	return ui.Box(c).Size(64, 48).Shrink(0).Role(ui.RoleImage).Label(i18n.T("stroke.preview")+": "+g.Name).
		Draw(func(p *ui.Painter, r ui.Rect) { paintTrail(p, r, points, ink, true, 2, 8) })
}

// gesturePreview is the gesture's large trail. A click, or a right click,
// records it again; a middle click resets it.
func gesturePreview(c *ui.Context, g stroke.Gesture, color string, disabled bool, record, reset func()) {
	t, pal := c.Theme(), kit.Colors(c)
	points := previewPoints(g)
	ink := strokeColor(color, 1)
	b := ui.ButtonBase(c).FillWidth().Radius(kit.PanelRadius).Border(1, pal.Hairline).Background(t.Background).
		Label(i18n.T("stroke.preview")).Disabled(disabled).Cursor(ui.CursorPointer)
	b.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputPointerDown || disabled {
			return false
		}
		switch ev.Button {
		case 1:
			record()
			return true
		case 2:
			reset()
			return true
		}
		return false
	})
	b.Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			ui.Box(c).FillWidth().Height(208).Draw(func(p *ui.Painter, r ui.Rect) { paintTrail(p, r, points, ink, true, 3, 24) })
			if len(points) < 2 {
				ui.Column(c).Padding(12).Children(func() { kit.Help(c, i18n.T("stroke.recordEmpty")) })
			}
		})
	})
	if b.Clicked() {
		record()
	}
}

// triggerButtons map Stroke's trigger to MyGo's pointer buttons; the side
// buttons reach no element as pointer input, so the recorder reads them
// from Windows.
var triggerButtons = map[string]int{"left": 0, "right": 1, "middle": 2}

// recorded is what stroke.record.read returns.
type recorded struct {
	ID      string         `json:"id"`
	State   string         `json:"state"`
	Points  []stroke.Point `json:"points"`
	Vectors [][]int        `json:"vectors"`
	Match   *struct {
		ID    string  `json:"id"`
		Name  string  `json:"name"`
		Score float64 `json:"score"`
	} `json:"match,omitempty"`
}

// gestureRecorder records a gesture's trail with the trigger button, in a
// dialog, through the worker's recognizer (stroke.record.*): the Host
// turns the trail into Stroke's vectors, blended with the old ones when
// asked.
type gestureRecorder struct {
	env       *core.Env
	gesture   stroke.Gesture
	trigger   string
	pen       stroke.Pen
	open      bool
	id        string
	ready     bool
	busy      bool
	closed    bool
	err       string
	drawing   bool
	points    []stroke.Point
	result    *recorded
	blend     bool
	use       func(points []stroke.Point, vectors [][]int)
	lastPoint point
}

func newGestureRecorder(env *core.Env, g stroke.Gesture, preferences stroke.Preferences, use func([]stroke.Point, [][]int)) *gestureRecorder {
	r := &gestureRecorder{env: env, gesture: g, trigger: preferences.TriggerButton, pen: preferences.Pen,
		open: true, blend: len(g.Vectors) > 0, use: use}
	core.Go(env, func() (string, error) {
		var started struct {
			ID string `json:"id"`
		}
		err := env.Host.CallCompletingInto("stroke.record.start", nil, &started)
		return started.ID, err
	}, func(id string, err error) {
		if err != nil {
			if !r.closed {
				r.err = err.Error()
			}
			return
		}
		r.id = id
		if r.closed {
			r.cancel()
			return
		}
		r.ready = true
	})
	return r
}

// cancel releases the worker's recording.
func (r *gestureRecorder) cancel() {
	if r.id == "" {
		return
	}
	id := r.id
	r.id = ""
	go func() { _ = r.env.Host.CallCompletingInto("stroke.record.cancel", map[string]string{"id": id}, nil) }()
}

// close ends the dialog and the recording.
func (r *gestureRecorder) close() {
	if r.closed {
		return
	}
	r.closed, r.open = true, false
	r.cancel()
}

func (r *gestureRecorder) add(x, y float32) {
	if len(r.points) >= 4096 {
		return
	}
	r.points = append(r.points, stroke.Point{X: int(math.Round(float64(x))), Y: int(math.Round(float64(y)))})
}

func (r *gestureRecorder) begin(x, y float32) {
	r.drawing, r.result, r.points = true, nil, nil
	r.add(x, y)
}

func (r *gestureRecorder) finish(x, y float32) {
	r.add(x, y)
	r.drawing = false
	if r.id == "" || len(r.points) < 2 {
		return
	}
	r.busy, r.err = true, ""
	request := map[string]any{"id": r.id, "gestureId": r.gesture.ID, "blend": r.blend, "priorVectors": r.gesture.Vectors, "points": r.points}
	core.Go(r.env, func() (recorded, error) {
		var value recorded
		err := r.env.Host.CallCompletingInto("stroke.record.read", request, &value)
		return value, err
	}, func(value recorded, err error) {
		r.busy = false
		if r.closed {
			return
		}
		if err != nil {
			r.err = err.Error()
			return
		}
		r.result = &value
	})
}

// View shows the dialog while it is open.
func (r *gestureRecorder) View(c *ui.Context) {
	if !r.open {
		return
	}
	theme, pal := c.Theme(), kit.Colors(c)
	description := i18n.T("stroke.recordHint") + " " + i18n.T("stroke."+r.trigger)
	open := r.open
	kit.Dialog(c, &open, kit.DialogOptions{
		Title: i18n.T("stroke.recordTitle"), Message: description, Width: 720,
		Body: func() {
			points := make([]point, len(r.points))
			for i, p := range r.points {
				points[i] = point{float32(p.X), float32(p.Y)}
			}
			ink := strokeColor(r.pen.Color, r.pen.Opacity)
			area := ui.Box(c).FillWidth().Height(320).Radius(kit.PanelRadius).Border(1, pal.Hairline).
				Background(theme.Surface).Label(i18n.T("stroke.recordArea")).Cursor(ui.CursorCrosshair).
				Draw(func(p *ui.Painter, rect ui.Rect) { paintTrail(p, rect, points, ink, false, float32(r.pen.Thickness), 0) })
			button, pointer := triggerButtons[r.trigger]
			area.HandleInput(func(ev ui.InputEvent) bool {
				if !r.ready || r.busy {
					return false
				}
				switch ev.Kind {
				case ui.InputPointerDown:
					if !pointer || ev.Button != button {
						return false
					}
					r.begin(ev.X, ev.Y)
					return true
				case ui.InputPointerMove:
					r.lastPoint = point{ev.X, ev.Y}
					// A side button is held: Windows reports it, MyGo does not.
					if !pointer && !r.drawing {
						if xButtonHeld(r.trigger) {
							r.begin(ev.X, ev.Y)
						}
						return false
					}
					if r.drawing {
						r.add(ev.X, ev.Y)
						return true
					}
				case ui.InputPointerUp:
					if pointer && r.drawing && ev.Button == button {
						r.finish(ev.X, ev.Y)
						return true
					}
				}
				return false
			})
			// The side buttons' release is read from Windows as well.
			if !pointer && r.drawing {
				if xButtonHeld(r.trigger) {
					c.After(30 * time.Millisecond)
				} else {
					r.finish(r.lastPoint.x, r.lastPoint.y)
				}
			}
			has := len(r.gesture.Vectors) > 0
			kit.Toggle(c, &r.blend, i18n.T("stroke.blend"), "").Disabled(!has || r.busy)
			switch {
			case r.busy:
				kit.Status(c, i18n.T("stroke.recordBusy"))
			case r.result != nil:
				text := i18n.T("stroke.recordDone")
				if m := r.result.Match; m != nil {
					text += " · " + i18n.T("stroke.recordMatch", "name", m.Name, "score", strconv.FormatFloat(m.Score, 'f', -1, 64), "id", m.ID)
				}
				kit.Status(c, text)
			}
			if r.err != "" {
				kit.Notice(c, kit.Failure, "", r.err, nil)
			}
		},
		Actions: func() {
			if kit.GhostButton(c, i18n.T("stroke.cancelRecord"), nil, false).Clicked() {
				r.close()
			}
			if kit.PrimaryButton(c, i18n.T("stroke.useRecord"), icons.Check, false).Disabled(r.result == nil || r.busy).Clicked() {
				r.use(r.result.Points, r.result.Vectors)
				r.close()
			}
		},
	})
	if !open {
		r.close()
	}
}

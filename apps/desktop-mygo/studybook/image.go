package studybook

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"math"
	"strconv"
	"strings"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type maskProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type maskShape struct {
	Shape      string         `json:"shape"`
	Properties []maskProperty `json:"properties"`
}
type maskGroup struct {
	Ordinal int         `json:"ordinal"`
	Shapes  []maskShape `json:"shapes"`
}
type imageDraft struct {
	ImageData           string      `json:"imageData"`
	Path                string      `json:"path"`
	NoteID              string      `json:"noteId"`
	Occlusions          []maskGroup `json:"occlusions"`
	Header              string      `json:"header"`
	BackExtra           string      `json:"backExtra"`
	Tags                []string    `json:"tags"`
	OccludeInactive     bool        `json:"occludeInactive"`
	bitmap              *ui.Bitmap
	shape, tagText      string
	width, height, x, y float32
	drawing, dirty      bool
	group               string
	points              [][2]float32
	history             [][]maskGroup
}

func maskNumber(shape maskShape, name string) float32 {
	for _, p := range shape.Properties {
		if p.Name == name {
			number, _ := strconv.ParseFloat(p.Value, 32)
			return float32(number)
		}
	}
	return 0
}
func property(name string, value float32) maskProperty {
	return maskProperty{Name: name, Value: strconv.FormatFloat(float64(value), 'f', 6, 32)}
}
func (d *imageDraft) remember() {
	raw, _ := json.Marshal(d.Occlusions)
	var saved []maskGroup
	_ = json.Unmarshal(raw, &saved)
	d.history = append(d.history, saved)
	d.dirty = true
}
func (d *imageDraft) add(shape maskShape) {
	d.remember()
	ordinal, _ := strconv.Atoi(d.group)
	if ordinal < 1 {
		ordinal = 1
		for _, group := range d.Occlusions {
			ordinal = max(ordinal, group.Ordinal+1)
		}
	}
	for i := range d.Occlusions {
		if d.Occlusions[i].Ordinal == ordinal {
			d.Occlusions[i].Shapes = append(d.Occlusions[i].Shapes, shape)
			return
		}
	}
	d.Occlusions = append(d.Occlusions, maskGroup{Ordinal: ordinal, Shapes: []maskShape{shape}})
}
func (b *Book) openImage(path, noteID string) {
	params := map[string]string{"path": path}
	if noteID != "" {
		params = map[string]string{"noteId": noteID}
	}
	b.act("winone.image.open", params, func(raw json.RawMessage) {
		var draft imageDraft
		if err := json.Unmarshal(raw, &draft); err != nil {
			b.err = err.Error()
			return
		}
		data, err := base64.StdEncoding.DecodeString(draft.ImageData)
		if err != nil {
			b.err = err.Error()
			return
		}
		draft.bitmap, err = ui.DecodeBitmap(data)
		if err != nil {
			b.err = err.Error()
			return
		}
		draft.shape = "rect"
		draft.tagText = strings.Join(draft.Tags, " ")
		b.image = &draft
	})
}
func (b *Book) chooseImage() {
	core.Go(b.env, func() (string, error) {
		return b.env.Dialogs.PickFile(text("imageOcclusion"), core.Filter{Name: text("images"), Extensions: []string{"png", "jpg", "jpeg", "webp", "gif", "bmp"}})
	}, func(path string, err error) {
		if b.closed {
			return
		}
		if err != nil {
			b.err = err.Error()
			return
		}
		if path != "" {
			b.openImage(path, "")
		}
	})
}
func (b *Book) imageEditor(c *ui.Context) {
	d := b.image
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		if kit.GhostButton(c, text("back"), icons.ChevronLeft, false).Clicked() {
			change := func() { b.image = nil }
			if b.env.RequestChange != nil {
				b.env.RequestChange(change)
			} else {
				change()
			}
		}
		kit.Select(c, &d.shape, []kit.SelectOption{{Value: "rect", Label: text("rectangle")}, {Value: "ellipse", Label: text("ellipse")}, {Value: "polygon", Label: text("polygon")}, {Value: "text", Label: text("textLabel")}})
		if kit.GhostButton(c, text("undo"), icons.Undo2, false).Disabled(len(d.history) == 0).Clicked() {
			d.Occlusions = d.history[len(d.history)-1]
			d.history = d.history[:len(d.history)-1]
			d.dirty = true
		}
	})
	kit.Help(c, text("maskHint"))
	w, h := d.bitmap.Size()
	area := ui.Box(c).FillWidth().AspectRatio(float32(w) / float32(h)).Clip().Cursor(ui.CursorCrosshair).Focusable().Label(text("maskCanvas"))
	area.Children(func() {
		ui.Image(c, d.bitmap).Fill().Fit(ui.Contain).IgnoreInput(true)
		for gi, group := range d.Occlusions {
			for si, shape := range group.Shapes {
				if shape.Shape != "text" {
					continue
				}
				label := ""
				for _, p := range shape.Properties {
					if p.Name == "text" {
						label = p.Value
					}
				}
				ui.Text(c.Key(fmt.Sprintf("text-%d-%d", gi, si)), label).Absolute().LeftPercent(100 * maskNumber(shape, "left")).TopPercent(100 * maskNumber(shape, "top")).MaxWidth(float32(w)).IgnoreInput(true)
			}
		}
	})
	area.Draw(func(p *ui.Painter, r ui.Rect) {
		d.width, d.height = r.W, r.H
		for _, group := range d.Occlusions {
			for _, shape := range group.Shapes {
				x, y := r.X+maskNumber(shape, "left")*r.W, r.Y+maskNumber(shape, "top")*r.H
				width, height := maskNumber(shape, "width")*r.W, maskNumber(shape, "height")*r.H
				color := ui.RGBA(100, 100, 100, .55)
				if shape.Shape == "ellipse" {
					path := &ui.Path{}
					for i := 0; i <= 48; i++ {
						angle := 2 * math.Pi * float64(i) / 48
						px, py := x+width/2+width/2*float32(math.Cos(angle)), y+height/2+height/2*float32(math.Sin(angle))
						if i == 0 {
							path.MoveTo(px, py)
						} else {
							path.LineTo(px, py)
						}
					}
					p.FillPath(path.Close(), color)
				} else if shape.Shape == "polygon" {
					path := &ui.Path{}
					for _, item := range shape.Properties {
						if item.Name != "points" {
							continue
						}
						for i, pair := range strings.Fields(item.Value) {
							values := strings.Split(pair, ",")
							if len(values) != 2 {
								continue
							}
							a, _ := strconv.ParseFloat(values[0], 32)
							z, _ := strconv.ParseFloat(values[1], 32)
							if i == 0 {
								path.MoveTo(x+float32(a)*r.W, y+float32(z)*r.H)
							} else {
								path.LineTo(x+float32(a)*r.W, y+float32(z)*r.H)
							}
						}
					}
					p.FillPath(path.Close(), color)
				} else if shape.Shape != "text" {
					p.Fill(ui.Rect{X: x, Y: y, W: width, H: height}, color, 0)
				}
			}
		}
		for _, point := range d.points {
			p.Fill(ui.Rect{X: r.X + point[0]*r.W - 3, Y: r.Y + point[1]*r.H - 3, W: 6, H: 6}, c.Theme().Text, 3)
		}
	})
	px, py, _ := area.PointerPosition()
	pressed := area.Pressed()
	if pressed && !d.drawing && d.width > 0 && d.height > 0 {
		d.drawing = true
		d.x, d.y = px/d.width, py/d.height
	}
	if !pressed && d.drawing {
		d.drawing = false
		x, y := max(float32(0), min(float32(1), px/d.width)), max(float32(0), min(float32(1), py/d.height))
		left, top := min(d.x, x), min(d.y, y)
		width, height := max(d.x, x)-left, max(d.y, y)-top
		if d.shape == "polygon" {
			d.points = append(d.points, [2]float32{x, y})
		} else if d.shape == "text" {
			d.add(maskShape{Shape: "text", Properties: []maskProperty{property("left", x), property("top", y), {Name: "text", Value: text("textLabel")}, {Name: "scale", Value: "1"}, {Name: "fs", Value: ".04"}}})
		} else if width > .002 && height > .002 {
			props := []maskProperty{property("left", left), property("top", top), property("width", width), property("height", height)}
			if d.shape == "ellipse" {
				props = append(props, property("rx", width/2), property("ry", height/2))
			}
			d.add(maskShape{Shape: d.shape, Properties: props})
		}
	}
	if d.shape == "polygon" && kit.Button(c, text("finishPolygon"), icons.Check, false).Disabled(len(d.points) < 3).Clicked() {
		var points []string
		for _, point := range d.points {
			points = append(points, fmt.Sprintf("%f,%f", point[0], point[1]))
		}
		d.add(maskShape{Shape: "polygon", Properties: []maskProperty{{Name: "left", Value: "0"}, {Name: "top", Value: "0"}, {Name: "points", Value: strings.Join(points, " ")}}})
		d.points = nil
	}
	ui.Field(c, text("maskGroup"), func() { kit.TextInput(c, &d.group).Placeholder(text("autoGroup")).FillWidth() }).FillWidth()
	if ui.Checkbox(c, &d.OccludeInactive, text("hideOtherMasks")).Changed() {
		d.dirty = true
	}
	remove := -1
	for groupIndex := range d.Occlusions {
		groupIndex := groupIndex
		group := &d.Occlusions[groupIndex]
		ui.Column(c.Key(groupIndex)).FillWidth().Gap(8).Children(func() {
			ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
				ordinal := float64(group.Ordinal)
				kit.NumberField(c, &ordinal, 1, 999, 1, "", text("maskGroup"))
				if int(ordinal) != group.Ordinal {
					d.remember()
					group.Ordinal = int(ordinal)
				}
				if kit.GhostButton(c, text("remove"), icons.Trash2, false).Clicked() {
					remove = groupIndex
				}
			})
			for shapeIndex := range group.Shapes {
				shape := &group.Shapes[shapeIndex]
				ui.Row(c.Key(shapeIndex)).FillWidth().Wrap().Gap(8).Children(func() {
					for propertyIndex := range shape.Properties {
						p := &shape.Properties[propertyIndex]
						ui.Field(c.Key(p.Name), human(p.Name), func() {
							if kit.TextInput(c, &p.Value).Width(150).Changed() {
								d.dirty = true
							}
						}).Basis(160).Grow(1)
					}
				})
			}
		})
	}
	if remove >= 0 {
		d.remember()
		d.Occlusions = append(d.Occlusions[:remove:remove], d.Occlusions[remove+1:]...)
	}
	ui.Field(c, text("imageHeader"), func() {
		if ui.TextArea(c, &d.Header).Lines(1, 4).Changed() {
			d.dirty = true
		}
	}).FillWidth()
	ui.Field(c, text("imageBack"), func() {
		if ui.TextArea(c, &d.BackExtra).Lines(2, 6).Changed() {
			d.dirty = true
		}
	}).FillWidth()
	ui.Field(c, text("tags"), func() {
		if kit.TextInput(c, &d.tagText).FillWidth().Changed() {
			d.dirty = true
		}
	}).FillWidth()
	if kit.Button(c, text("saveMasks"), icons.Save, false).Disabled(len(d.Occlusions) == 0).Clicked() {
		params := map[string]any{"path": d.Path, "noteId": d.NoteID, "deckId": b.deckID, "occlusions": d.Occlusions, "occludeInactive": d.OccludeInactive, "header": d.Header, "backExtra": d.BackExtra, "tags": strings.Fields(d.tagText)}
		b.act("winone.image.save", params, func(json.RawMessage) { b.image = nil; b.tab = 1; b.search(false) })
	}
}

package onebot

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	_ "golang.org/x/image/webp"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// The crop uses native Image/Slider/Button controls, matching Cindy's portrait
// picker geometry. Only the committed PNG goes to the existing avatar store.
type portraitDialog struct {
	parent       *page
	image        image.Image
	bitmap       *ui.Bitmap
	zoom         float64
	x, y         float32
	stage        ui.Handle
	busy, closed bool
	error        string
	save         func(string, func(error))
	onClose      func()
}

func newPortraitDialog(parent *page, encoded string, save func(string, func(error)), onClose func()) (*portraitDialog, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%s", nt("avatarFailed"))
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s", nt("avatarFailed"))
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, err
	}
	return &portraitDialog{parent: parent, image: source, bitmap: bitmap, zoom: 1, save: save, onClose: onClose}, nil
}
func (p *portraitDialog) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.bitmap = nil
	p.image = nil
	if p.onClose != nil {
		p.onClose()
	}
}
func (p *portraitDialog) clamp(stage float32) {
	bounds := p.image.Bounds()
	factor := float64(stage) / float64(min(bounds.Dx(), bounds.Dy())) * p.zoom
	maxX, maxY := float32((float64(bounds.Dx())*factor-float64(stage))/2), float32((float64(bounds.Dy())*factor-float64(stage))/2)
	p.x = max(-maxX, min(maxX, p.x))
	p.y = max(-maxY, min(maxY, p.y))
}
func cropPortrait(source image.Image, zoom float64, x, y, stage float32) ([]byte, error) {
	bounds := source.Bounds()
	if bounds.Empty() || stage <= 0 || zoom < 1 || math.IsNaN(zoom) || math.IsInf(zoom, 0) {
		return nil, fmt.Errorf("%s", nt("avatarFailed"))
	}
	cover := float64(stage) / float64(min(bounds.Dx(), bounds.Dy())) * zoom
	scale := cover * 512 / float64(stage)
	tx := (float64(stage)-float64(bounds.Dx())*cover)/2 + float64(x)
	ty := (float64(stage)-float64(bounds.Dy())*cover)/2 + float64(y)
	out := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	xdraw.CatmullRom.Transform(out, f64.Aff3{scale, 0, tx*512/float64(stage) - float64(bounds.Min.X)*scale, 0, scale, ty*512/float64(stage) - float64(bounds.Min.Y)*scale}, source, bounds, xdraw.Src, nil)
	var data bytes.Buffer
	err := png.Encode(&data, out)
	return data.Bytes(), err
}
func (p *portraitDialog) View(c *ui.Context) {
	if p.closed {
		return
	}
	open := true
	stageSize := p.stage.Bounds(c).W
	if stageSize <= 0 {
		stageSize = 280
	}
	p.clamp(stageSize)
	if kit.Dialog(c, &open, kit.DialogOptions{Title: bt("settings.userProfile.edit.crop.title"), Message: bt("settings.userProfile.edit.crop.description"), Width: 500, Confirm: bt("settings.userProfile.edit.crop.done"), Cancel: nt("cancel"), Busy: p.busy, Error: p.error, Body: func() {
		stage := ui.Box(c).Width(280).MaxWidthPercent(100).AspectRatio(1).AlignSelf(ui.Center).Clip().Focusable().Bind(&p.stage).Label(bt("settings.userProfile.edit.crop.stageLabel"))
		stage.Children(func() {
			ui.Image(c, p.bitmap).Fill().Fit(ui.Cover).Scale(float32(p.zoom), float32(p.zoom)).Translate(p.x, p.y)
		})
		if dx, dy, dragged := stage.Dragged(); dragged {
			p.x += dx
			p.y += dy
			p.clamp(stageSize)
		}
		if stage.Shortcut(0, ui.KeyLeft) {
			p.x -= 10
		}
		if stage.Shortcut(0, ui.KeyRight) {
			p.x += 10
		}
		if stage.Shortcut(0, ui.KeyUp) {
			p.y -= 10
		}
		if stage.Shortcut(0, ui.KeyDown) {
			p.y += 10
		}
		ui.Slider(c, &p.zoom, 1, 8).FillWidth().Label(bt("settings.userProfile.edit.crop.zoomLabel"))
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if kit.IconButton(c, icons.ZoomOut, bt("settings.userProfile.edit.crop.zoomOut")).Clicked() {
				p.zoom = max(1, p.zoom-.25)
			}
			if kit.IconButton(c, icons.ZoomIn, bt("settings.userProfile.edit.crop.zoomIn")).Clicked() {
				p.zoom = min(8, p.zoom+.25)
			}
			kit.Caption(c, bt("settings.userProfile.edit.crop.zoomValue", "percent", int(p.zoom*100)))
		})
	}}) {
		p.clamp(stageSize)
		data, err := cropPortrait(p.image, p.zoom, p.x, p.y, stageSize)
		if err != nil {
			p.error = err.Error()
			return
		}
		p.busy = true
		p.save(base64.StdEncoding.EncodeToString(data), func(err error) {
			if p.closed {
				return
			}
			p.busy = false
			if err != nil {
				p.error = err.Error()
				return
			}
			p.Close()
		})
	}
	if !open && !p.busy {
		p.Close()
	}
}
func (d *profileSettings) choosePortrait() {
	core.Go(d.parent.env, func() (string, error) {
		return d.parent.env.Dialogs.PickFile(bt("bots.profile.changeAvatar"), core.Filter{Name: nt("images"), Extensions: []string{"png", "jpg", "jpeg", "webp"}})
	}, func(path string, err error) {
		if d.closed {
			return
		}
		if err != nil {
			d.error = err.Error()
			return
		}
		if path != "" {
			d.portraitSource(path)
		}
	})
}
func (d *profileSettings) portraitSource(path string) {
	oneBotCall(d.parent, "avatar.source", map[string]string{"path": path}, func(value struct {
		Base64 string `json:"base64"`
	}, err error) {
		if d.closed {
			return
		}
		if err != nil {
			d.error = err.Error()
			return
		}
		portrait, err := newPortraitDialog(d.parent, value.Base64, func(encoded string, done func(error)) {
			d.saving = true
			oneBotCall(d.parent, "bots.chooseAvatar", map[string]string{"botId": d.botID, "avatarImageBase64": encoded}, func(value struct {
				Canceled bool    `json:"canceled"`
				Profile  Profile `json:"profile"`
			}, err error) {
				if d.closed {
					return
				}
				d.saving = false
				if err != nil {
					done(err)
					return
				}
				if value.Canceled {
					done(nil)
					return
				}
				d.draft = rebaseProfile(d.draft, d.baseline, value.Profile)
				d.baseline = copyValue(value.Profile)
				d.parent.mergeProfile(value.Profile)
				d.parent.avatars.load(d.parent, []Profile{value.Profile})
				done(nil)
			})
		}, func() { d.portrait = nil })
		if err != nil {
			d.error = err.Error()
			return
		}
		d.portrait = portrait
	})
}

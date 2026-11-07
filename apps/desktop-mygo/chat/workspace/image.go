package workspace

import (
	"context"
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// ImagePane uses MyGo's real bitmap/GIF decoder and native image surface.
// Preview bytes come only from a Host-issued scoped URL; zoom never creates
// horizontal scrolling, and a magnified image can be panned within its clip.
type ImagePane struct {
	env             *core.Env
	preview         FilePreview
	ctx             context.Context
	cancel          context.CancelFunc
	image           *ui.Bitmap
	video           *ui.VideoPlayer
	loading, closed bool
	err             string
	zoom            float32
	rotation        float32
	x, y            float32
	visible         bool
}

func NewImagePane(env *core.Env, preview FilePreview) *ImagePane {
	ctx, cancel := context.WithCancel(context.Background())
	p := &ImagePane{env: env, preview: preview, ctx: ctx, cancel: cancel, loading: true, zoom: 1, visible: true}
	type result struct {
		data  []byte
		image *ui.Bitmap
	}
	core.Go(env, func() (result, error) {
		// Match Cindy's original image byte-action limit and the shared
		// Host/Markdown path; a valid preview must not fail only in lightbox.
		data, err := FetchPreview(ctx, preview.URL, 100<<20)
		if err != nil {
			return result{}, err
		}
		image, err := ui.DecodeBitmap(data)
		return result{data, image}, err
	}, func(value result, err error) {
		if p.closed {
			return
		}
		p.loading = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.image = value.image
		if preview.MIME == "image/gif" {
			decoder, err := ui.NewGIFDecoder(value.data)
			if err != nil {
				p.err = err.Error()
				return
			}
			p.video, err = ui.NewVideoPlayer(decoder)
			if err != nil {
				p.err = err.Error()
				return
			}
			p.video.SetLoop(true)
		}
	})
	return p
}
func (p *ImagePane) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.cancel()
	if p.video != nil {
		p.video.Close()
	}
	p.image = nil
	p.video = nil
}
func (p *ImagePane) SetVisible(visible bool) {
	p.visible = visible
	if p.video != nil && !visible {
		p.video.Pause()
	}
}
func (p *ImagePane) View(c *ui.Context) {
	if p.closed || !p.visible {
		return
	}
	ui.Column(c).Fill().MinWidth(0).Gap(6).Children(func() {
		ui.Row(c).FillWidth().Gap(6).Padding(8).Wrap().Children(func() {
			kit.Help(c, fmt.Sprintf("%.0f%%", p.zoom*100))
			if kit.IconButton(c, icons.ZoomIn, i18n.T("chat.zoomIn")).Clicked() {
				p.zoom = min(10, p.zoom*1.25)
			}
			if kit.IconButton(c, icons.ZoomOut, i18n.T("chat.zoomOut")).Clicked() {
				p.zoom = max(.1, p.zoom/1.25)
			}
			if kit.IconButton(c, icons.Maximize, i18n.T("chat.fitImage")).Clicked() {
				p.zoom, p.rotation, p.x, p.y = 1, 0, 0, 0
			}
			if kit.IconButton(c, icons.RotateCw, i18n.T("chat.rotateImage")).Clicked() {
				p.rotation = float32(math.Mod(float64(p.rotation+90), 360))
			}
			if p.video != nil {
				label := wt("pauseImage")
				if !p.video.Playing() {
					label = wt("playImage")
				}
				if kit.Button(c, label, icons.Play, false).Clicked() {
					if p.video.Playing() {
						p.video.Pause()
					} else {
						p.video.Play()
					}
				}
			}
		})
		kit.InlineError(c, p.err)
		if p.loading {
			kit.Loader(c)
		}
		stage := ui.Box(c).Grow(1).FillWidth().MinHeight(0).Clip()
		stage.Children(func() {
			if p.image == nil {
				return
			}
			image := ui.Image(c, p.image)
			if p.video != nil {
				image = ui.Video(c, p.video)
			}
			image.Fill().Fit(ui.Contain).Scale(p.zoom, p.zoom).Rotate(p.rotation).Translate(p.x, p.y).Label(p.preview.Name)
		})
		if dx, dy, dragged := stage.Dragged(); dragged {
			p.x += dx
			p.y += dy
		}
	})
}

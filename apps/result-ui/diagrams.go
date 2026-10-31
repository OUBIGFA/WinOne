package resultui

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

type diagramEntry struct {
	bitmap       *ui.Bitmap
	state, error string
	zoom         float32
	sourceOpen   bool
	bytes        int
	used         uint64
}

func (p *Panel) renderDiagram(c *ui.Context, t *ui.Theme, source string, ready bool) {
	if !ready {
		ui.Spinner(c).Size(16, 16).AlignSelf(ui.Start).Label(p.T("正在接收图表…", "Receiving diagram…"))
		return
	}
	if p.diagrams == nil {
		p.diagrams = map[string]*diagramEntry{}
	}
	key := fmt.Sprintf("%x/%t", sha256.Sum256([]byte(source)), t.Dark)
	entry := p.diagrams[key]
	if entry == nil {
		if len(p.diagrams) >= 64 {
			ui.Text(c, p.T("图表数量较多，请复制源码查看。", "Too many diagrams. Copy the source to view it separately.")).FontSize(em(t, sizeLabel)).TextColor(t.TextMuted)
			ui.Row(c).FillWidth().Children(func() {
				ui.Spacer(c)
				if iconButton(c, "copy", p.T("复制图表源码", "Copy diagram source"), false).Clicked() && p.Actions.Copy != nil {
					p.Actions.Copy(source)
				}
			})
			p.diagramSource(c, t, source)
			return
		}
		entry = &diagramEntry{zoom: 1}
		p.diagrams[key] = entry
	}
	request := func() {
		if p.Actions.RenderDiagram == nil {
			entry.state = "failed"
			entry.error = p.T("图表组件不可用，请修复安装。", "Diagram component unavailable. Repair the installation.")
			return
		}
		entry.state = "loading"
		p.Actions.RenderDiagram(source, t.Dark, func(bitmap *ui.Bitmap, err error) {
			if err != nil {
				entry.state = "failed"
				entry.error = err.Error()
				return
			}
			w, h := bitmap.Size()
			entry.bytes = w * h * 4
			entry.bitmap = bitmap
			entry.state = "ready"
			p.diagramClock++
			entry.used = p.diagramClock
			// Keep bitmap caches bounded. An evicted chart offers an explicit reload;
			// scrolling never creates an automatic eviction/re-render loop.
			total := 0
			for _, item := range p.diagrams {
				if item.bitmap != nil {
					total += item.bytes
				}
			}
			for total > 16<<20 {
				var oldest *diagramEntry
				for _, item := range p.diagrams {
					if item != entry && item.bitmap != nil && (oldest == nil || item.used < oldest.used) {
						oldest = item
					}
				}
				if oldest == nil {
					break
				}
				total -= oldest.bytes
				oldest.bitmap = nil
				oldest.state = "deferred"
			}
		})
	}
	if entry.state == "" {
		request()
	}
	// The diagram never scrolls sideways (WinOne forbids horizontal
	// scrolling): it shows at its own size or the reading column's width,
	// whichever is smaller, and zooms within that room.
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		var base, limit float32
		if entry.bitmap != nil {
			w, _ := entry.bitmap.Size()
			base = min(p.readingWidth(c), float32(w)/2) // the renderer draws at 2x
			limit = min(3, p.readingWidth(c)/max(base, 1))
			entry.zoom = min(entry.zoom, limit)
		}
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(4).Children(func() {
			ui.Text(c, "Mermaid").FontSize(em(t, sizeMeta)).TextColor(t.TextMuted).Grow(1)
			if entry.bitmap != nil {
				if iconButton(c, "minus", p.T("缩小图表", "Zoom out"), entry.zoom <= .4).Clicked() {
					entry.zoom = max(.4, entry.zoom-.25)
				}
				if iconButton(c, "plus", p.T("放大图表", "Zoom in"), entry.zoom >= limit).Clicked() {
					entry.zoom = min(limit, entry.zoom+.25)
				}
			}
			if iconButton(c, "code", p.T("显示或隐藏源码", "Show or hide source"), false).Clicked() {
				entry.sourceOpen = !entry.sourceOpen
			}
			if entry.sourceOpen && iconButton(c, "copy", p.T("复制图表源码", "Copy diagram source"), false).Clicked() && p.Actions.Copy != nil {
				p.Actions.Copy(source)
			}
		})
		switch entry.state {
		case "loading":
			ui.Spinner(c).Size(16, 16).AlignSelf(ui.Start).Label(p.T("正在渲染图表…", "Rendering diagram…"))
		case "ready":
			w, h := entry.bitmap.Size()
			width := base * entry.zoom
			ui.Image(c, entry.bitmap).Size(width, float32(h)*width/float32(w)).AlignSelf(ui.Start).Label(p.T("Mermaid 图表", "Mermaid diagram"))
		case "failed":
			p.selectable(c, ui.Column(c).FillWidth(), func() {
				ui.Text(c, entry.error).FillWidth().FontSize(em(t, sizeLabel)).TextColor(t.Danger)
			})
			if ui.Button(c, p.T("重试图表", "Retry diagram")).AlignSelf(ui.Start).Clicked() {
				request()
			}
		case "deferred":
			if ui.Button(c, p.T("显示图表", "Show diagram")).AlignSelf(ui.Start).Clicked() {
				request()
			}
		}
		if entry.sourceOpen {
			p.diagramSource(c, t, source)
		}
	})
}

// readingWidth is the reading column's width in this window.
func (p *Panel) readingWidth(c *ui.Context) float32 {
	width, _ := c.Size()
	return max(40, min(width-2*gutter, readingMaxWidth))
}

// diagramSource shows a diagram's source as code blocks show code: in the
// code font, wrapped rather than scrolled, selectable.
func (p *Panel) diagramSource(c *ui.Context, t *ui.Theme, source string) {
	p.selectable(c, ui.Column(c).FillWidth().Padding(8, 12).Radius(8).Background(t.Text.Alpha(0.035)).Border(1, t.Border), func() {
		ui.Text(c, strings.TrimSuffix(source, "\n")).FillWidth().Font(monoFont).FontSize(em(t, sizeMeta)).LineHeight(1.5)
	})
}

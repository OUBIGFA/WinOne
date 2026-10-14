package resultui

import (
	"crypto/sha256"
	"fmt"
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
			ui.Text(c, p.T("图表数量较多，请复制源码查看。", "Too many diagrams. Copy the source to view it separately."))
			p.codeBlock(c, t, "mermaid", source)
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
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Row(c).Gap(4).Children(func() {
			ui.Text(c, "Mermaid").TextColor(t.TextMuted).Grow(1)
			if entry.bitmap != nil {
				if iconButton(c, "minus", p.T("缩小图表", "Zoom out")).Clicked() {
					entry.zoom = max(.4, entry.zoom-.25)
				}
				if iconButton(c, "plus", p.T("放大图表", "Zoom in")).Clicked() {
					entry.zoom = min(3, entry.zoom+.25)
				}
			}
			if iconButton(c, "code", p.T("显示或隐藏源码", "Show or hide source")).Clicked() {
				entry.sourceOpen = !entry.sourceOpen
			}
			if entry.sourceOpen && iconButton(c, "copy", p.T("复制图表源码", "Copy diagram source")).Clicked() && p.Actions.Copy != nil {
				p.Actions.Copy(source)
			}
		})
		switch entry.state {
		case "loading":
			ui.Spinner(c).Size(16, 16).AlignSelf(ui.Start).Label(p.T("正在渲染图表…", "Rendering diagram…"))
		case "ready":
			width, _ := c.Size()
			w, h := entry.bitmap.Size()
			fit := min(1, min(width-48, 840)/(float32(w)/2))
			scale := fit * entry.zoom / 2
			ui.ScrollBoth(c).FillWidth().Height(min(440, float32(h)*scale+8)).Padding(4).Children(func() {
				ui.Image(c, entry.bitmap).Size(float32(w)*scale, float32(h)*scale).Label(p.T("Mermaid 图表", "Mermaid diagram"))
			})
		case "failed":
			ui.Text(c, entry.error).TextColor(t.Danger).Selectable()
			if ui.Button(c, p.T("重试图表", "Retry diagram")).AlignSelf(ui.Start).Clicked() {
				request()
			}
		case "deferred":
			if ui.Button(c, p.T("显示图表", "Show diagram")).AlignSelf(ui.Start).Clicked() {
				request()
			}
		}
		if entry.sourceOpen {
			height := min(260, float32(20*(1+countLines(source))))
			ui.ScrollBoth(c).FillWidth().Height(height).Padding(8).Background(t.SurfaceHover).Children(func() { ui.Text(c, source).Font("Consolas").NoWrap().Selectable() })
		}
	})
}
func countLines(value string) int {
	n := 0
	for _, r := range value {
		if r == '\n' {
			n++
		}
	}
	return n
}

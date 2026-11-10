package flashchat

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Media are downloaded by the same encrypted transport as file offers. Only
// decoded raster frames and playback state cross into the native view.
func (p *completePage) nativePreview(c *ui.Context, message nativeMessage) {
	if !p.previewRequested[message.ID] && p.view != nil {
		p.previewRequested[message.ID] = true
		scope := "room:" + p.selected
		core.Do(p.env, func() error {
			_, err := p.invoke(scope, "preview", map[string]any{"id": message.ID, "magnet": message.Magnet})
			return err
		}, func(err error) {
			if err != nil && !p.closed {
				p.previewErrors[message.ID] = err.Error()
			}
		})
	}
	if err := p.previewErrors[message.ID]; err != "" {
		kit.InlineError(c, err)
		if kit.GhostButton(c, tr("retry"), nil, false).Clicked() {
			delete(p.previewRequested, message.ID)
			delete(p.previewErrors, message.ID)
		}
		return
	}
	items := p.snapshot.Previews[message.ID]
	if len(items) == 0 {
		kit.Caption(c, nt("mediaLoading"))
	}
	for _, item := range items {
		ui.Column(c.Key(item.ID)).FillWidth().MinWidth(0).Gap(6).Children(func() {
			kit.Caption(c, item.Name)
			if frame := p.frames[item.ID]; frame != nil {
				button := ui.ButtonBase(c).FillWidth().MinWidth(0).Label(nt("enlarge") + " · " + item.Name).Children(func() { ui.Image(c, frame).FillWidth().Height(220).Fit(ui.Contain) })
				if button.Clicked() {
					p.previewID, p.panel = item.ID, "preview"
				}
			}
			if item.Kind == "audio" || item.Kind == "video" {
				label, icon := nt("play"), icons.Play
				if item.Playing {
					label, icon = nt("pause"), icons.Pause
				}
				if kit.Button(c, label, icon, false).Disabled(p.busy).Clicked() {
					p.command("room:"+p.selected, "playback", map[string]any{"id": item.ID, "enabled": !item.Playing}, nil)
				}
				if item.Duration > 0 {
					position := item.Current
					if ui.Slider(c, &position, 0, item.Duration).FillWidth().Label(nt("position")).Changed() {
						p.backgroundCommand("room:"+p.selected, "playback", map[string]any{"id": item.ID, "position": position})
					}
					kit.Caption(c, fmt.Sprintf("%d:%02d / %d:%02d", int(item.Current)/60, int(item.Current)%60, int(item.Duration)/60, int(item.Duration)%60))
				}
			}
		})
	}
}

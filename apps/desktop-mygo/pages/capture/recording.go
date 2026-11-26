package capture

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

func captureText(zh, en string) string {
	if i18n.Locale() == i18n.English {
		return en
	}
	return zh
}

func (p *page) recordingSettings(c *ui.Context) {
	d := &p.draft.Value
	section := func(gif bool, ask *bool, folder *string) {
		title := captureText("视频录制", "Video recording")
		if gif {
			title = captureText("GIF 录制", "GIF recording")
		}
		kit.Group(c, icons.Monitor, title, "", func() {
			kit.Toggle(c, ask, captureText("每次询问保存地址", "Ask where to save each time"),
				captureText("确认保存地址后，点击工具栏的开始按钮录制。", "Confirm the destination, then select Start on the toolbar."))
			display := *folder
			if display == "" {
				display = d.OutputDirectory
			}
			var choose ui.Element
			zone := kit.Stack(c, captureText("默认保存目录", "Default save folder"), "", func() {
				choose = kit.PathField(c, display, "", captureText("选择目录", "Choose folder"), p.busy())
			})
			setFolder := func(path string) {
				if gif {
					p.draft.Value.GifOutputDirectory = path
				} else {
					p.draft.Value.VideoOutputDirectory = path
				}
			}
			if choose.Clicked() {
				p.choosing, p.err = true, ""
				core.Go(p.env, func() (string, error) { return p.env.Dialogs.PickFolder(title) }, func(path string, err error) {
					p.choosing = false
					if err != nil {
						p.err = err.Error()
					} else if path != "" && p.draft != nil {
						setFolder(path)
					}
				})
			}
			widgets.DropArea(c, p.env, zone, widgets.DropFolders, false, p.busy(), func(paths []string, err error) {
				if err != nil {
					p.err = err.Error()
				} else if len(paths) > 0 {
					setFolder(paths[0])
				}
			})
			if *folder != "" && kit.Button(c, captureText("使用截图保存目录", "Use screenshot folder"), nil, false).Clicked() {
				*folder = ""
			}
		})
	}
	section(false, &d.VideoAskWhereToSave, &d.VideoOutputDirectory)
	section(true, &d.GifAskWhereToSave, &d.GifOutputDirectory)
}

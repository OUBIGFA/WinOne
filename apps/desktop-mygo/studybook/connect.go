package studybook

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"strconv"
	"strings"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type connectSettings struct {
	Enabled, Running bool
	Port             int
	Origins          []string
	Key, Error       string
}

func (b *Book) connectDraft() string {
	if b.connect == nil {
		return ""
	}
	raw, _ := json.Marshal([]any{b.connect.Enabled, b.connectPort, b.connectOrigins})
	return string(raw)
}
func (b *Book) acceptConnect(value connectSettings) {
	b.connect = &value
	b.connectPort = strconv.Itoa(value.Port)
	b.connectOrigins = strings.Join(value.Origins, "\n")
	copy := value
	copy.Origins = append([]string{}, value.Origins...)
	b.connectSaved = &copy
	b.connectBaseline = b.connectDraft()
}

func (b *Book) connectTools(c *ui.Context) {
	kit.Heading(c, text("connectTitle"))
	kit.Help(c, text("connectHint"))
	if b.connect == nil {
		if kit.Button(c, text("connectSettings"), icons.Settings, false).Clicked() {
			b.busy = true
			core.Go(b.env, func() (connectSettings, error) {
				var value connectSettings
				err := b.env.Host.CallInto("learning.connect.get", nil, &value)
				return value, err
			}, func(value connectSettings, err error) {
				if b.closed {
					return
				}
				b.busy = false
				if err != nil {
					b.err = err.Error()
					return
				}
				b.acceptConnect(value)
			})
		}
		return
	}
	ui.Checkbox(c, &b.connect.Enabled, text("connectEnable"))
	ui.Field(c, text("port"), func() { ui.TextInput(c, &b.connectPort).FillWidth() }).FillWidth()
	ui.Field(c, text("origins"), func() { ui.TextArea(c, &b.connectOrigins).Lines(2, 5).FillWidth() }).FillWidth()
	if b.connect.Key != "" {
		ui.Field(c, text("apiKey"), func() { ui.Text(c, b.connect.Key).Selectable() }).FillWidth()
	}
	if b.connect.Error != "" {
		kit.Notice(c, kit.Failure, "", b.connect.Error, nil)
	}
	if b.connect.Running {
		kit.Help(c, text("connectRunning"))
	} else {
		kit.Help(c, text("connectStopped"))
	}
	if kit.Button(c, text("saveConnect"), icons.Save, false).Clicked() {
		port, err := strconv.Atoi(b.connectPort)
		if err != nil {
			b.err = text("invalidPort")
			return
		}
		origins := []string{}
		for _, line := range strings.Split(b.connectOrigins, "\n") {
			if value := strings.TrimSpace(line); value != "" {
				origins = append(origins, value)
			}
		}
		enabled := b.connect.Enabled
		b.busy = true
		core.Go(b.env, func() (connectSettings, error) {
			var value connectSettings
			err := b.env.Host.CallInto("learning.connect.save", map[string]any{"enabled": enabled, "port": port, "origins": origins}, &value)
			return value, err
		}, func(value connectSettings, err error) {
			if b.closed {
				return
			}
			b.busy = false
			if err != nil {
				b.err = err.Error()
				return
			}
			b.acceptConnect(value)
			b.notice = text("completed")
		})
	}
}

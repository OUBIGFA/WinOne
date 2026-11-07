package onebot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/audioio"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type dictationRange struct {
	id         string
	start, end int
	touched    bool
}
type dictationSession struct {
	parent                             *page
	ctx                                context.Context
	cancel                             context.CancelFunc
	mic                                *audioio.Input
	micCancel                          context.CancelFunc
	runID, phase, text, partial, error string
	stopFence                          *float64
	busy, closed                       bool
	ranges                             map[string]*dictationRange
	accept                             func(string) error
}

func (p *page) startDictation(accept func(string) error) {
	if p.dictation != nil && !p.dictation.closed {
		return
	}
	if p.voice != nil && !p.voice.closed {
		p.error = nt("voiceBusy")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &dictationSession{parent: p, ctx: ctx, cancel: cancel, busy: true, phase: "starting", ranges: map[string]*dictationRange{}, accept: accept}
	p.dictation = d
	type started struct{ runID, deviceID string }
	core.Go(p.env, func() (started, error) {
		var r started
		var settings struct {
			Settings struct {
				MicrophoneDeviceID *string `json:"microphoneDeviceId"`
				MuteSystemAudio    bool    `json:"muteSystemAudio"`
				Language           string  `json:"language"`
			} `json:"settings"`
		}
		if err := p.call("voice.input.data", nil, &settings); err != nil {
			return r, err
		}
		if settings.Settings.MicrophoneDeviceID != nil {
			r.deviceID = *settings.Settings.MicrophoneDeviceID
		}
		if settings.Settings.MuteSystemAudio {
			if err := p.call("voice.input.mute", nil, nil); err != nil {
				return r, err
			}
		}
		var result struct {
			RunID string `json:"runId"`
		}
		if err := p.call("voice.input.start", map[string]string{"sourceLanguage": settings.Settings.Language}, &result); err != nil {
			_ = p.call("voice.input.restore", nil, nil)
			return r, err
		}
		r.runID = result.RunID
		return r, nil
	}, func(r started, err error) {
		if d.closed {
			return
		}
		d.busy = false
		if err != nil {
			d.error = err.Error()
			d.phase = "error"
			return
		}
		d.runID = r.runID
		ctx, cancel := context.WithCancel(d.ctx)
		d.micCancel = cancel
		core.Go(p.env, func() (*audioio.Input, error) {
			return audioio.StartInput(ctx, r.deviceID, func(bytes []byte) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				return p.call("voice.input.audio", map[string]string{"runId": r.runID, "pcm": base64.StdEncoding.EncodeToString(bytes)}, nil)
			}, func(err error) {
				p.env.Post(func() {
					if !d.closed {
						d.error = nt("microphoneFailed") + " · " + err.Error()
						d.phase = "error"
					}
				})
			})
		}, func(input *audioio.Input, err error) {
			if d.closed || ctx.Err() != nil {
				if input != nil {
					input.Stop()
				}
				return
			}
			if err != nil {
				d.error = nt("microphoneFailed") + " · " + err.Error()
				d.phase = "error"
				return
			}
			d.mic = input
			d.phase = "listening"
		})
	})
}
func (d *dictationSession) receive(raw json.RawMessage) {
	if d.closed {
		return
	}
	var event struct {
		Type, RunID, Text, Message, State string
		Segment                           struct{ ID string }
		Range                             struct {
			ID                     string
			StartOffset, EndOffset int
		}
	}
	if json.Unmarshal(raw, &event) != nil || event.RunID != d.runID {
		return
	}
	switch event.Type {
	case "state":
		d.phase = event.State
	case "error":
		d.error = event.Message
		d.phase = "error"
	case "draft":
		d.partial = event.Text
	case "submitted":
		d.partial = ""
		if _, seen := d.ranges[event.Segment.ID]; seen {
			return
		}
		separator := ""
		if d.text != "" && !strings.HasSuffix(d.text, "\n") {
			separator = "\n"
		}
		start := len(utf16.Encode([]rune(d.text + separator)))
		d.text += separator + event.Text
		d.ranges[event.Segment.ID] = &dictationRange{id: event.Segment.ID, start: start, end: len(utf16.Encode([]rune(d.text)))}
	case "refined":
		rangeValue := d.ranges[event.Range.ID]
		if rangeValue == nil || rangeValue.touched {
			return
		}
		units := utf16.Encode([]rune(d.text))
		if rangeValue.start < 0 || rangeValue.end > len(units) || rangeValue.start > rangeValue.end {
			return
		}
		changed := utf16.Encode([]rune(event.Text))
		next := append(append(append([]uint16(nil), units[:rangeValue.start]...), changed...), units[rangeValue.end:]...)
		delta := len(changed) - (rangeValue.end - rangeValue.start)
		rangeValue.end = rangeValue.start + len(changed)
		d.text = string(utf16.Decode(next))
		for _, other := range d.ranges {
			if other != rangeValue && other.start >= rangeValue.end-delta {
				other.start += delta
				other.end += delta
			}
		}
	}
}
func (d *dictationSession) finish() {
	if d.closed || d.busy {
		return
	}
	d.busy = true
	if d.micCancel != nil {
		d.micCancel()
	}
	id := d.runID
	core.Go(d.parent.env, func() (float64, error) {
		if d.mic != nil {
			<-d.mic.Done()
		}
		if err := d.parent.call("voice.input.stop", map[string]string{"runId": id}, nil); err != nil {
			return 0, err
		}
		var fence struct {
			Cursor float64 `json:"cursor"`
		}
		err := d.parent.call("events.barrier", nil, &fence)
		return fence.Cursor, err
	}, func(fence float64, err error) {
		if d.closed {
			return
		}
		d.busy = false
		if err != nil {
			d.error = err.Error()
			return
		}
		d.stopFence = &fence
		d.phase = "finalizing"
		d.finishFence()
	})
}
func (d *dictationSession) finishFence() {
	if d.stopFence != nil && d.parent.cursor >= *d.stopFence && d.parent.cursor != 9007199254740991 {
		d.phase = "finished"
	}
}
func (d *dictationSession) Close() {
	if d.closed {
		return
	}
	d.closed = true
	d.cancel()
	if d.micCancel != nil {
		d.micCancel()
	}
	if d.mic != nil {
		d.mic.Stop()
	}
	id := d.runID
	core.Do(d.parent.env, func() error { return d.parent.call("voice.input.cancel", map[string]string{"runId": id}, nil) }, func(err error) {
		if err != nil && !d.parent.closed {
			d.parent.error = err.Error()
		}
	})
}
func (p *page) dictationView(c *ui.Context) {
	d := p.dictation
	if d == nil || d.closed {
		return
	}
	open := true
	kit.Dialog(c, &open, kit.DialogOptions{Title: nt("dictation"), Width: 720, Cancel: nt("cancel"), Error: d.error, Body: func() {
		kit.Status(c, nt("dictationPhase", "phase", d.phase))
		if d.partial != "" {
			kit.Help(c, d.partial)
		}
		if ui.TextArea(c, &d.text).FillWidth().Height(260).Label(nt("dictationText")).Changed() {
			for _, rangeValue := range d.ranges {
				rangeValue.touched = true
			}
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if kit.Button(c, nt("finishDictation"), icons.Square, d.busy).Disabled(d.busy || d.phase == "finished").Clicked() {
				d.finish()
			}
			if kit.PrimaryButton(c, nt("insertDraft"), icons.ArrowDownToLine, false).Disabled(d.busy || d.phase != "finished").Clicked() {
				if err := d.accept(d.text); err != nil {
					d.error = err.Error()
					return
				}
				text := d.text
				core.Do(p.env, func() error { return p.call("voice.input.recordHistory", map[string]string{"text": text}, nil) }, func(err error) {
					if err != nil {
						p.error = err.Error()
					}
				})
				d.Close()
				p.dictation = nil
			}
			if kit.Button(c, nt("copy"), icons.Copy, false).Clicked() {
				c.Services().WriteClipboard(d.text)
			}
		})
	}})
	if !open {
		d.Close()
		p.dictation = nil
	}
}

// All settings originate in the same native Host store. Device IDs are mapped
// to WinMM only here; a legacy unavailable browser ID is retained and explained.
type microphonePane struct {
	parent                *profileSettings
	devices               []audioio.InputDevice
	deviceID, baseline    string
	loading, busy, closed bool
	error                 string
}

func newMicrophonePane(d *profileSettings) *microphonePane {
	p := &microphonePane{parent: d, loading: true}
	core.Go(d.parent.env, func() ([]audioio.InputDevice, error) { return audioio.Inputs() }, func(devices []audioio.InputDevice, err error) {
		if p.closed {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.devices = devices
	})
	oneBotCall(d.parent, "voice.input.data", nil, func(value struct {
		Settings struct {
			MicrophoneDeviceID *string `json:"microphoneDeviceId"`
		} `json:"settings"`
	}, err error) {
		if p.closed {
			return
		}
		if err != nil {
			p.error = err.Error()
			return
		}
		if value.Settings.MicrophoneDeviceID != nil {
			p.deviceID = *value.Settings.MicrophoneDeviceID
		}
		p.baseline = p.deviceID
	})
	return p
}
func (p *microphonePane) Dirty() bool { return p.deviceID != p.baseline }
func (p *microphonePane) Busy() bool  { return p.busy }
func (p *microphonePane) Close()      { p.closed = true }
func (p *microphonePane) Discard()    { p.deviceID = p.baseline }
func (p *microphonePane) View(c *ui.Context) {
	if p.loading {
		kit.Loader(c)
	}
	options := []kit.SelectOption{}
	for _, device := range p.devices {
		label := device.Name
		if device.ID == "" {
			label = nt("defaultMicrophone")
		}
		options = append(options, kit.SelectOption{Value: device.ID, Label: label})
	}
	ui.Field(c, nt("microphone"), func() { kit.Select(c, &p.deviceID, options) })
	kit.InlineError(c, p.error)
	if kit.PrimaryButton(c, nt("save"), icons.Save, p.busy).Disabled(!p.Dirty() || p.busy).Clicked() {
		p.busy = true
		id := p.deviceID
		var value any
		if id != "" {
			value = id
		}
		oneBotCall(p.parent.parent, "voice.input.settings", map[string]any{"patch": map[string]any{"microphoneDeviceId": value}}, func(_ json.RawMessage, err error) {
			if p.closed {
				return
			}
			p.busy = false
			if err != nil {
				p.error = err.Error()
				return
			}
			p.baseline = id
		})
	}
	if kit.Button(c, nt("microphonePermissions"), icons.Settings2, false).Clicked() {
		oneBotCall(p.parent.parent, "voice.input.microphoneSettings", nil, func(_ any, err error) {
			if err != nil {
				p.error = err.Error()
			}
		})
	}
}

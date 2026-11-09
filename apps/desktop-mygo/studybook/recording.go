package studybook

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"github.com/egoist/mygo/ui"
	"sync"
	"winone/desktopmygo/audioio"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type voiceRecording struct {
	mu      sync.Mutex
	input   *audioio.Input
	done    <-chan struct{}
	pcm     []byte
	cancel  context.CancelFunc
	playing bool
}

func (b *Book) stopRecording() {
	if b.recording == nil {
		return
	}
	r := b.recording
	if r.input != nil {
		r.input.Stop()
		r.input = nil
	}
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	r.playing = false
}

func (b *Book) recordingActive() bool {
	return b.recording != nil && (b.recording.input != nil || b.recording.playing)
}
func (b *Book) recordControls(c *ui.Context) {
	if b.recording == nil {
		b.recording = &voiceRecording{}
	}
	r := b.recording
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		label := text("recordVoice")
		if r.input != nil {
			label = text("stopRecording")
		}
		if kit.GhostButton(c, label, icons.Mic, false).Clicked() {
			if r.input != nil {
				r.input.Stop()
				r.input = nil
			} else {
				b.stopRecording()
				r = &voiceRecording{}
				b.recording = r
				ctx, cancel := context.WithCancel(context.Background())
				r.cancel = cancel
				input, err := audioio.StartInput(ctx, "", func(chunk []byte) error {
					r.mu.Lock()
					defer r.mu.Unlock()
					if len(r.pcm)+len(chunk) > 64<<20 {
						return errors.New(text("mediaTooLarge"))
					}
					r.pcm = append(r.pcm, chunk...)
					return nil
				}, func(err error) {
					b.env.Post(func() {
						if !b.closed && ctx.Err() == nil {
							b.err = err.Error()
							b.stopRecording()
						}
					})
				})
				if err != nil {
					cancel()
					r.cancel = nil
					b.err = err.Error()
				} else {
					r.input = input
					r.done = input.Done()
				}
			}
		}
		r.mu.Lock()
		has := len(r.pcm) > 0
		r.mu.Unlock()
		if kit.GhostButton(c, text("replayVoice"), icons.Play, r.playing).Disabled(!has || r.input != nil).Clicked() {
			b.stopRecording()
			ctx, cancel := context.WithCancel(context.Background())
			r.cancel = cancel
			r.playing = true
			core.Do(b.env, func() error {
				if r.done != nil {
					select {
					case <-r.done:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				r.mu.Lock()
				data := append([]byte(nil), r.pcm...)
				r.mu.Unlock()
				player, err := audioio.OpenOutput(ctx, 16000)
				if err != nil {
					return err
				}
				defer player.Close()
				return player.Write(data)
			}, func(err error) {
				if b.closed || ctx.Err() != nil {
					return
				}
				r.playing = false
				r.cancel = nil
				cancel()
				if err != nil {
					b.err = err.Error()
				}
			})
		}
		if kit.GhostButton(c, text("stopAudio"), icons.Square, false).Disabled(r.input == nil && !r.playing).Clicked() {
			b.stopRecording()
		}
	})
}

// wavPCM uses the same 16 kHz mono PCM format as WinOne's native audio input.
func wavPCM(pcm []byte) []byte {
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(36+len(pcm)))
	out.WriteString("WAVEfmt ")
	_ = binary.Write(&out, binary.LittleEndian, uint32(16))
	for _, value := range []uint16{1, 1} {
		_ = binary.Write(&out, binary.LittleEndian, value)
	}
	_ = binary.Write(&out, binary.LittleEndian, uint32(16000))
	_ = binary.Write(&out, binary.LittleEndian, uint32(32000))
	_ = binary.Write(&out, binary.LittleEndian, uint16(2))
	_ = binary.Write(&out, binary.LittleEndian, uint16(16))
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(pcm)))
	out.Write(pcm)
	return out.Bytes()
}

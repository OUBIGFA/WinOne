package workspace

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type mediaState struct {
	ID                               uint64 `json:"id"`
	Opened, Playing, Ended, Muted    bool
	Position, Duration, Volume, Rate float64
	VideoWidth, VideoHeight          int
	Frame, Error                     string
}
type mediaClient struct {
	mu       sync.Mutex
	command  *exec.Cmd
	input    io.WriteCloser
	lines    chan []byte
	done     chan error
	cancel   context.CancelFunc
	sequence uint64
	once     sync.Once
}

func startMediaClient(ctx context.Context, path string) (*mediaClient, error) {
	ctx, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(ctx, path)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	input, err := command.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		cancel()
		return nil, err
	}
	p := &mediaClient{command: command, input: input, lines: make(chan []byte, 1), done: make(chan error, 1), cancel: cancel}
	command.Stderr = io.Discard
	if err = command.Start(); err != nil {
		input.Close()
		cancel()
		return nil, err
	}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		defer close(p.lines)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 64<<10), 32<<20)
		for scanner.Scan() {
			select {
			case p.lines <- append([]byte(nil), scanner.Bytes()...):
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { err := command.Wait(); <-readDone; p.done <- err }()
	select {
	case raw := <-p.lines:
		var ready struct{ Ready bool }
		if json.Unmarshal(raw, &ready) != nil || !ready.Ready {
			p.Close()
			return nil, errors.New(wt("mediaUnavailable"))
		}
	case <-ctx.Done():
		p.Close()
		return nil, ctx.Err()
	case <-time.After(15 * time.Second):
		p.Close()
		return nil, errors.New(wt("mediaUnavailable"))
	}
	return p, nil
}
func (p *mediaClient) call(ctx context.Context, method string, values map[string]any) (mediaState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if values == nil {
		values = map[string]any{}
	}
	p.sequence++
	values["id"], values["method"] = p.sequence, method
	if err := json.NewEncoder(p.input).Encode(values); err != nil {
		return mediaState{}, errors.New(wt("mediaUnavailable"))
	}
	select {
	case raw, ok := <-p.lines:
		if !ok {
			return mediaState{}, errors.New(wt("mediaUnavailable"))
		}
		var state mediaState
		if json.Unmarshal(raw, &state) != nil || state.ID != p.sequence {
			return state, errors.New(wt("mediaUnavailable"))
		}
		if state.Error != "" {
			return state, fmt.Errorf("%s (%s)", wt("mediaUnavailable"), state.Error)
		}
		return state, nil
	case <-ctx.Done():
		return mediaState{}, ctx.Err()
	}
}
func (p *mediaClient) Close() error {
	var failure error
	p.once.Do(func() {
		p.cancel()
		p.input.Close()
		if p.command.Process != nil {
			_ = p.command.Process.Kill()
		}
		select {
		case err := <-p.done:
			if _, killed := err.(*exec.ExitError); !killed {
				failure = err
			}
		case <-time.After(5 * time.Second):
			failure = errors.New(wt("mediaDidNotExit"))
		}
	})
	return failure
}

// MediaPane uses Windows' existing decoder in a lazy, owned helper. Playback,
// seek, sound and rate are real engine operations; no external player or web
// document substitutes for the native controls.
type mediaCommand struct {
	method string
	values map[string]any
}
type MediaPane struct {
	env                                               *core.Env
	preview                                           FilePreview
	client                                            *mediaClient
	ctx                                               context.Context
	cancel                                            context.CancelFunc
	state                                             mediaState
	bitmap                                            *ui.Bitmap
	stage                                             ui.Handle
	width, height                                     int
	pending                                           []mediaCommand
	loading, busy, closed, visible, polling, expanded bool
	timer                                             *time.Timer
	error                                             string
}

func NewMediaPane(env *core.Env, preview FilePreview) (*MediaPane, error) {
	u, err := url.Parse(preview.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil {
		return nil, errors.New(i18n.T("chat.fileReadFailed"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &MediaPane{env: env, preview: preview, ctx: ctx, cancel: cancel, loading: true, visible: true, width: 640, height: 480}
	path := os.Getenv("WINONE_MEDIA_WORKER")
	if path == "" {
		executable, err := os.Executable()
		if err != nil {
			cancel()
			return nil, err
		}
		path = filepath.Join(filepath.Dir(executable), "WinOne.Media.exe")
	}
	core.Go(env, func() (*mediaClient, error) {
		client, err := startMediaClient(ctx, path)
		if err != nil {
			return nil, err
		}
		if _, err = client.call(ctx, "open", map[string]any{"url": preview.URL}); err != nil {
			client.Close()
			return nil, err
		}
		return client, nil
	}, func(client *mediaClient, err error) {
		if p.closed {
			if client != nil {
				core.Do(env, client.Close, func(error) {})
			}
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.client = client
		p.poll()
	})
	return p, nil
}
func (p *MediaPane) Busy() bool { return p.busy }
func (p *MediaPane) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.cancel()
	if p.timer != nil {
		p.timer.Stop()
	}
	if p.client != nil {
		client := p.client
		p.client = nil
		core.Do(p.env, client.Close, func(err error) {
			if err != nil && p.env.Toast != nil {
				p.env.Toast(err.Error())
			}
		})
	}
	p.bitmap = nil
}
func (p *MediaPane) SetVisible(visible bool) {
	if p.visible == visible {
		return
	}
	p.visible = visible
	if !visible {
		if p.timer != nil {
			p.timer.Stop()
		}
		if p.client != nil {
			p.run("pause", nil)
		}
	} else if p.client != nil {
		p.poll()
	}
}
func (p *MediaPane) run(method string, values map[string]any) {
	if p.closed || p.client == nil {
		return
	}
	if p.busy {
		if method == "volume" || method == "seek" || method == "rate" {
			for i := range p.pending {
				if p.pending[i].method == method {
					p.pending[i].values = values
					return
				}
			}
		}
		p.pending = append(p.pending, mediaCommand{method, values})
		return
	}
	p.busy = true
	client := p.client
	core.Go(p.env, func() (mediaState, error) { return client.call(p.ctx, method, values) }, func(state mediaState, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
		} else {
			p.state = state
			p.error = ""
		}
		if len(p.pending) > 0 {
			next := p.pending[0]
			p.pending = p.pending[1:]
			p.run(next.method, next.values)
		} else {
			p.poll()
		}
	})
}
func (p *MediaPane) poll() {
	if p.closed || !p.visible || p.polling || p.client == nil {
		return
	}
	p.polling = true
	client := p.client
	video := p.preview.Kind == "video"
	width, height := p.width, p.height
	core.Go(p.env, func() (mediaState, error) {
		return client.call(p.ctx, "status", map[string]any{"frame": video, "width": width, "height": height})
	}, func(state mediaState, err error) {
		if p.closed {
			return
		}
		p.polling = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.state = state
		p.error = ""
		if state.Frame != "" {
			data, decodeErr := base64.StdEncoding.DecodeString(state.Frame)
			if decodeErr == nil {
				p.bitmap, decodeErr = ui.DecodeBitmap(data)
			}
			if decodeErr != nil {
				p.error = i18n.T("chat.fileReadFailed")
			}
		}
		delay := time.Second
		if state.Playing {
			delay = time.Second / 30
		} else if !state.Opened {
			delay = 100 * time.Millisecond
		}
		p.timer = time.AfterFunc(delay, func() { p.env.Post(p.poll) })
	})
}
func (p *MediaPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.loading {
		kit.Loader(c)
	}
	kit.InlineError(c, p.error)
	p.content(c)
	if p.expanded {
		open := true
		kit.Dialog(c, &open, kit.DialogOptions{Title: p.preview.Name, Width: 1000, Cancel: i18n.T("chat.close"), Body: func() { ui.Column(c).FillWidth().Height(560).Children(func() { p.content(c) }) }})
		if !open {
			p.expanded = false
		}
	}
}
func (p *MediaPane) content(c *ui.Context) {
	bounds := p.stage.Bounds(c)
	if bounds.W > 0 && bounds.H > 0 {
		p.width, p.height = min(1024, int(bounds.W)), min(1024, int(bounds.H))
	}
	ui.Column(c).Fill().MinWidth(0).Gap(8).Children(func() {
		if p.preview.Kind == "video" {
			ui.Box(c).FillWidth().Grow(1).MinHeight(120).Clip().Bind(&p.stage).Children(func() {
				if p.bitmap != nil {
					ui.Image(c, p.bitmap).Fill().Fit(ui.Contain).Label(p.preview.Name)
				}
			})
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			label := wt("mediaPlay")
			if p.state.Playing {
				label = wt("mediaPause")
			}
			if kit.Button(c, label, icons.Play, false).Disabled(p.client == nil || p.busy).Clicked() {
				method := "play"
				if p.state.Playing {
					method = "pause"
				}
				p.run(method, nil)
			}
			if kit.IconButton(c, icons.Maximize, i18n.T("chat.fitImage")).Clicked() {
				p.expanded = !p.expanded
			}
			if kit.IconButton(c, icons.Volume2, wt("mediaMute")).Clicked() {
				p.run("mute", map[string]any{"muted": !p.state.Muted})
			}
			kit.Caption(c, fmt.Sprintf("%s / %s", mediaTime(p.state.Position), mediaTime(p.state.Duration)))
		})
		if p.state.Duration > 0 {
			position := p.state.Position
			if ui.Slider(c, &position, 0, p.state.Duration).FillWidth().Label(wt("mediaSeek")).Changed() {
				p.run("seek", map[string]any{"position": position})
			}
		}
		volume := p.state.Volume
		if ui.Slider(c, &volume, 0, 1).FillWidth().Label(wt("mediaVolume")).Changed() {
			p.run("volume", map[string]any{"volume": volume})
		}
		rate := fmt.Sprint(p.state.Rate)
		if rate == "0" {
			rate = "1"
		}
		options := []kit.SelectOption{}
		for _, value := range []string{"0.5", "0.75", "1", "1.25", "1.5", "2"} {
			options = append(options, kit.SelectOption{Value: value, Label: value + "×"})
		}
		if changed, _ := kit.Select(c, &rate, options); changed {
			var speed float64
			_, _ = fmt.Sscan(rate, &speed)
			p.run("rate", map[string]any{"rate": speed})
		}
	})
}
func mediaTime(seconds float64) string {
	value := int(seconds)
	return fmt.Sprintf("%d:%02d", value/60, value%60)
}

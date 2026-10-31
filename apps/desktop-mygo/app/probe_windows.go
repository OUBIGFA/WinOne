//go:build windows

package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/shell"
	"winone/internal/catalog"
	internalhost "winone/internal/host"
	"winone/internal/settings"
	"winone/internal/startupprobe"
	"winone/internal/transport"
)

// probeOptions admits only the installer's nonce and an existing scratch
// directory outside this installation, including after resolving junctions.
func probeOptions(args []string, executable string) (string, error) {
	if len(args) != 4 || args[0] != "--check-ui" || args[2] != "--probe-root" || args[1] == "" || len(args[1]) > 128 || strings.ContainsRune(args[1], 0) {
		return "", errors.New("启动检查参数无效")
	}
	if !filepath.IsAbs(args[3]) {
		return "", errors.New("启动检查需要绝对临时路径")
	}
	scratch, err := filepath.EvalSymlinks(args[3])
	if err != nil {
		return "", fmt.Errorf("无法读取启动检查临时目录：%w", err)
	}
	info, err := os.Stat(scratch)
	if err != nil || !info.IsDir() {
		return "", errors.New("启动检查临时路径不是目录")
	}
	installation, err := filepath.EvalSymlinks(filepath.Dir(executable))
	if err != nil {
		return "", err
	}
	inside := strings.EqualFold(scratch, installation) || strings.HasPrefix(strings.ToLower(scratch), strings.ToLower(strings.TrimRight(installation, string(filepath.Separator)))+string(filepath.Separator))
	if inside {
		return "", errors.New("启动检查临时目录必须在程序目录外")
	}
	return scratch, nil
}

// nativeProbeHost supplies read-only catalog data to the production shell on a
// private pipe. No user Host, settings, workers, account or clipboard is used.
// Any newly introduced startup operation fails the probe instead of being run.
type nativeProbeHost struct {
	snapshot                  internalhost.Snapshot
	modules                   map[string]any
	seenSnapshot, seenModules atomic.Bool
	failure                   chan error
}

func newNativeProbeHost() *nativeProbeHost {
	h := &nativeProbeHost{failure: make(chan error, 1)}
	h.snapshot.Settings = settings.Settings{SchemaVersion: 1, Theme: "light", Language: "zh-CN"}
	h.snapshot.Locale = "zh-CN"
	entries := make([]any, 0, len(catalog.Builtins()))
	for _, m := range catalog.Builtins() {
		h.snapshot.Modules = append(h.snapshot.Modules, internalhost.Module{ID: m.ID, Name: m.Name.Chinese, State: "disabled"})
		entries = append(entries, map[string]any{"manifest": m, "enabled": false, "runtime": map[string]any{"id": m.ID, "state": "disabled"}})
	}
	h.modules = map[string]any{"revision": 0, "modules": entries}
	return h
}

func (h *nativeProbeHost) Handle(r internalhost.Request) internalhost.Response {
	var value any
	switch r.Method {
	case "snapshot":
		h.seenSnapshot.Store(true)
		value = h.snapshot
	case "modules.snapshot":
		h.seenModules.Store(true)
		value = h.modules
	case "interconnect.pending", "directory-links.pending":
		value = core.Intent{}
	default:
		err := fmt.Errorf("启动检查不允许调用 %s", r.Method)
		select {
		case h.failure <- err:
		default:
		}
		return internalhost.Response{ID: r.ID, Error: &internalhost.RPCError{Code: "probe", Message: err.Error()}}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return internalhost.Response{ID: r.ID, Error: &internalhost.RPCError{Code: "probe", Message: err.Error()}}
	}
	return internalhost.Response{ID: r.ID, Result: data}
}

// runProbe opens and renders the actual MyGo shell under an isolated profile.
// It retains the existing installer handshake; no browser identity is claimed
// for a native window. UIReady is emitted only after real pipe reads, their UI
// callbacks, a native paint and capture, and normal window/process-loop exit.
func runProbe(args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	scratch, err := probeOptions(args, executable)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := startupprobe.Check(ctx, filepath.Join(filepath.Dir(executable), "WinOne.Host.exe")); err != nil {
		return fmt.Errorf("后台启动自检失败：%w", err)
	}
	directory, err := os.MkdirTemp(scratch, "mygo-desktop-probe-")
	if err != nil {
		return err
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-Desktop-Probe-%d-%s`, os.Getpid(), rand.Text())
	listener, err := transport.Listen(pipe)
	if err != nil {
		return err
	}
	h := newNativeProbeHost()
	served := make(chan struct{})
	go func() { _ = transport.Serve(ctx, listener, h); close(served) }()
	defer func() { cancel(); listener.Close(); <-served }()
	client, err := host.NewClient(pipe)
	if err != nil {
		return err
	}
	defer client.Cancel()
	w := &window{client: client}
	var work, posted atomic.Int64
	var workers sync.WaitGroup
	env := &core.Env{Host: client, Dialogs: dialogs{w: w}}
	env.Spawn = func(fn func()) {
		work.Add(1)
		workers.Add(1)
		go func() {
			defer workers.Done()
			fn()
			work.Add(-1)
			w.win.Invalidate()
		}()
	}
	env.Post = func(fn func()) {
		posted.Add(1)
		w.win.Update(func() { defer posted.Add(-1); fn() })
	}
	w.shell = shell.New(env, w, shell.Options{Directory: directory, EnsureHost: func() (internalhost.Snapshot, error) {
		var snapshot internalhost.Snapshot
		err := client.CallInto("snapshot", nil, &snapshot)
		return snapshot, err
	}})
	mygo.App.SetName("WinOne Startup Probe")
	mygo.App.SetPath(mygo.PathUserData, directory)
	var painted, queued, completed bool
	var probeErr error
	mygo.App.WhenReady(func() {
		content := ui.View(func(c *ui.Context) {
			ui.Column(c).Fill().Children(func() { w.shell.View(c) }).Draw(func(_ *ui.Painter, r ui.Rect) {
				painted = r.W > 0 && r.H > 0
			})
			select {
			case probeErr = <-h.failure:
			default:
			}
			ready := painted && h.seenSnapshot.Load() && h.seenModules.Load() && work.Load() == 0 && posted.Load() == 0
			if !queued && w.win != nil && (probeErr != nil || ready) {
				queued = true
				// Update queues after this frame, so the current production
				// content finishes painting before it is captured and closed.
				w.win.Update(func() {
					if probeErr == nil {
						var data []byte
						data, probeErr = w.win.CapturePage()
						if probeErr == nil {
							config, decodeErr := png.DecodeConfig(bytes.NewReader(data))
							if decodeErr != nil || config.Width == 0 || config.Height == 0 {
								probeErr = errors.New("原生窗口没有生成有效图像")
							} else {
								probeErr = os.WriteFile(filepath.Join(directory, "desktop.png"), data, 0600)
							}
						}
					}
					completed = probeErr == nil
					w.win.Close()
				})
			} else if !queued {
				c.After(10 * time.Millisecond)
			}
		})
		opts := settingsWindowOptions(content)
		opts.SkipTaskbar = true
		w.win = mygo.NewWindow(opts)
		w.win.ShowInactive()
		w.shell.Start()
	})
	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			mygo.App.Quit()
		case <-finished:
		}
	}()
	err = mygo.App.Run()
	close(finished)
	client.Cancel()
	cancel()
	workers.Wait()
	if err != nil {
		return err
	}
	if probeErr != nil {
		return probeErr
	}
	if !completed {
		return errors.New("原生窗口启动检查未完成")
	}
	return json.NewEncoder(os.Stdout).Encode(startupprobe.Report{Protocol: 1, Nonce: args[1], PID: os.Getpid(), Executable: executable, Modules: catalog.Builtins(), UIReady: true})
}

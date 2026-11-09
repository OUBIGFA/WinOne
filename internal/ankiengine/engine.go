// Package ankiengine owns the bundled Anki worker and serializes collection
// operations over private pipes. It never connects to a user's external Anki.
package ankiengine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Options struct{ Python, Worker, Library, Source, Profile string }
type Event struct {
	Type  string          `json:"type"`
	Page  string          `json:"page,omitempty"`
	Query string          `json:"query,omitempty"`
	Data  json.RawMessage `json:"-"`
}
type reply struct {
	ID     uint64            `json:"id"`
	Result json.RawMessage   `json:"result"`
	Events []json.RawMessage `json:"events"`
	Error  *struct {
		Message string `json:"message"`
		Kind    string `json:"kind"`
	} `json:"error"`
}

type Engine struct {
	diagnostic  *diagnosticBuffer
	options     Options
	lane        chan struct{}
	mu          sync.Mutex
	writeMu     sync.Mutex
	activeID    uint64
	activeOwner uint32
	cmd         *exec.Cmd
	input       io.WriteCloser
	output      *bufio.Reader
	done        chan error
	next        uint64
	closed      bool
	lastUsed    time.Time
	idleOnce    sync.Once
	idleStop    chan struct{}
}

func New(options Options) *Engine {
	return &Engine{options: options, lane: make(chan struct{}, 1), idleStop: make(chan struct{})}
}

// Touch keeps an actively displayed review alive without a worker request.
func (e *Engine) Touch() { e.mu.Lock(); e.lastUsed = time.Now(); e.mu.Unlock() }

func (e *Engine) watchIdle() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.idleStop:
			return
		case <-ticker.C:
			e.releaseIdle()
		}
	}
}

// releaseIdle rechecks activity after the worker reply, so a newly focused
// page or queued write cannot lose its collection between the two checks.
func (e *Engine) releaseIdle() bool {
	e.mu.Lock()
	observed := e.lastUsed
	due := e.cmd != nil && !e.closed && time.Since(observed) >= 30*time.Second
	e.mu.Unlock()
	if !due {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	var status struct{ Idle bool }
	_, err := e.Call(ctx, "winone.idle", map[string]any{}, &status)
	cancel()
	if err != nil || !status.Idle {
		return false
	}
	select {
	case e.lane <- struct{}{}:
		defer func() { <-e.lane }()
		e.mu.Lock()
		unchanged := e.lastUsed.Equal(observed)
		e.mu.Unlock()
		if unchanged {
			return e.stop(false) == nil
		}
	default:
	}
	return false
}

// Available verifies the bundled files without starting a collection worker.
// Merely opening a dictionary result must not prewarm a separate application.
func (e *Engine) Available() error {
	for _, path := range []string{e.options.Python, e.options.Worker, filepath.Join(e.options.Library, "anki", "collection.py"), filepath.Join(e.options.Source, "services.py")} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("内置 Anki 运行文件不完整，请修复 WinOne 安装：%w", err)
		}
	}
	return nil
}

// BundleOptions uses the already-shipped isolated Python runtime, with Anki's
// own dependency directory first. Neither runtime is installed on the system.
func BundleOptions(bundle, data string) Options {
	return Options{Python: filepath.Join(bundle, "capture", "Ocr", "python", "python.exe"),
		Worker: filepath.Join(bundle, "shared", "anki", "worker.py"), Library: filepath.Join(bundle, "shared", "anki", "lib"),
		Source: filepath.Join(bundle, "shared", "anki", "connect"), Profile: filepath.Join(data, "learning", "anki")}
}

func (e *Engine) start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("英语学习服务已关闭")
	}
	if e.cmd != nil {
		select {
		case <-e.done:
			_ = e.input.Close()
			e.cmd, e.input, e.output, e.done = nil, nil, nil, nil
		default:
			return nil
		}
	}
	if err := e.Available(); err != nil {
		return err
	}
	if err := os.MkdirAll(e.options.Profile, 0700); err != nil {
		return err
	}
	cmd := exec.Command(e.options.Python, "-B", "-u", e.options.Worker, "--profile", e.options.Profile, "--library", e.options.Library, "--source", e.options.Source)
	hideWindow(cmd)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONDONTWRITEBYTECODE=1")
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return err
	}
	// Retain a bounded in-memory startup diagnostic so an import failure is not
	// reduced to EOF. It is never written to a persistent log.
	diagnostic := &diagnosticBuffer{}
	cmd.Stderr = diagnostic
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return err
	}
	e.cmd, e.input, e.output, e.done = cmd, input, bufio.NewReader(output), make(chan error, 1)
	e.diagnostic = diagnostic
	done := e.done
	go func() { done <- cmd.Wait() }()
	return nil
}

// Call never interrupts a write transaction halfway through. A caller may
// cancel waiting; the lane remains owned until the real response is drained.
func (e *Engine) Call(ctx context.Context, method string, params any, out any) ([]json.RawMessage, error) {
	return e.CallOwned(ctx, 0, method, params, out)
}

// CallOwned binds native editor leases to the verified caller process. External
// AnkiConnect requests use owner zero and cannot impersonate a settings process.
func (e *Engine) CallOwned(ctx context.Context, owner uint32, method string, params any, out any) ([]json.RawMessage, error) {
	if method != "winone.idle" {
		e.Touch()
	}
	select {
	case e.lane <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		<-e.lane
		return nil, err
	}
	if err := e.start(); err != nil {
		<-e.lane
		return nil, err
	}
	e.idleOnce.Do(func() { go e.watchIdle() })
	e.mu.Lock()
	input, output, diagnostic := e.input, e.output, e.diagnostic
	e.mu.Unlock()
	type completion struct {
		value reply
		err   error
	}
	finished := make(chan completion, 1)
	go func() {
		defer func() { <-e.lane }()
		e.next++
		id := e.next
		e.mu.Lock()
		e.activeID = id
		e.activeOwner = owner
		e.mu.Unlock()
		defer func() {
			e.mu.Lock()
			if e.activeID == id {
				e.activeID = 0
				e.activeOwner = 0
			}
			e.mu.Unlock()
		}()
		body, err := json.Marshal(map[string]any{"id": id, "owner": owner, "method": method, "params": params})
		if err == nil {
			e.writeMu.Lock()
			_, err = input.Write(append(body, '\n'))
			e.writeMu.Unlock()
		}
		var value reply
		if err == nil {
			line, readErr := output.ReadBytes('\n')
			err = readErr
			if err == nil {
				err = json.Unmarshal(line, &value)
			}
			if err == nil && value.ID != id {
				err = errors.New("内置 Anki 返回了不匹配的请求编号")
			}
		}
		if err != nil {
			if detail := diagnostic.String(); detail != "" {
				err = fmt.Errorf("%w: %s", err, detail)
			}
			e.stop(false)
		}
		finished <- completion{value, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-finished:
		if result.err != nil {
			return nil, result.err
		}
		if result.value.Error != nil {
			return result.value.Events, errors.New(result.value.Error.Message)
		}
		if out != nil {
			if err := json.Unmarshal(result.value.Result, out); err != nil {
				return result.value.Events, err
			}
		}
		return result.value.Events, nil
	}
}

// CancelCurrent sends only an abort signal for the caller's own active work.
// Anki performs its supported cancellation and transaction rollback itself.
func (e *Engine) CancelCurrent(owner uint32) (bool, error) {
	e.mu.Lock()
	id, input := e.activeID, e.input
	allowed := id != 0 && e.activeOwner == owner && input != nil
	e.mu.Unlock()
	if !allowed {
		return false, nil
	}
	body, _ := json.Marshal(map[string]any{"method": "cancel", "target": id})
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	_, err := input.Write(append(body, '\n'))
	return err == nil, err
}

type diagnosticBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *diagnosticBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	size := len(data)
	b.data = append(b.data, data...)
	if len(b.data) > 32768 {
		b.data = append([]byte(nil), b.data[len(b.data)-32768:]...)
	}
	return size, nil
}
func (b *diagnosticBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }

func (e *Engine) stop(closeService bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if closeService {
		e.closed = true
	}
	if e.cmd == nil {
		return nil
	}
	_ = e.input.Close()
	var err error
	select {
	case err = <-e.done:
	case <-time.After(15 * time.Second):
		_ = e.cmd.Process.Kill()
		err = <-e.done
	}
	e.cmd, e.input, e.output, e.done = nil, nil, nil, nil
	if err != nil {
		return fmt.Errorf("Anki 关闭失败：%w: %s", err, e.diagnostic.String())
	}
	return nil
}

func (e *Engine) Close() error {
	e.mu.Lock()
	if !e.closed {
		close(e.idleStop)
	}
	e.closed = true
	e.mu.Unlock()
	select {
	case e.lane <- struct{}{}:
	case <-time.After(5 * time.Second):
		e.stop(true)
		e.lane <- struct{}{}
	}
	defer func() { <-e.lane }()
	return e.stop(true)
}

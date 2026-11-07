package workspace

import (
	"context"
	"encoding/base64"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"winone/desktopmygo/core"
)

type terminalPoll struct {
	Data   []byte  `json:"data"`
	Offset uint64  `json:"offset"`
	Reset  bool    `json:"reset"`
	Exit   *uint32 `json:"exit"`
	Error  string  `json:"error"`
}
type terminalInfo struct {
	ID, Shell, Directory string
	PID                  uint32
}

// terminalConnection attaches the native emulator to the existing Host ConPTY.
// The official MyGo terminal already queues input and coalesces resize calls;
// this adapter does no shell emulation, process creation, or input replay.
type terminalConnection struct {
	env                            *core.Env
	prefix, sessionID, tabID       string
	ctx                            context.Context
	cancel                         context.CancelFunc
	visible                        atomic.Bool
	writeMu                        sync.Mutex
	failureMu                      sync.Mutex
	failure                        error
	onError                        func(error)
	onExit                         func(int)
	errorOnce, exitOnce, closeOnce sync.Once
	closeErr                       error
	offset                         uint64
	pending                        []byte
	exit                           *uint32
}

func newTerminalConnection(env *core.Env, prefix, sessionID, tabID, shell string, onError func(error), onExit func(int)) (*terminalConnection, terminalInfo, error) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &terminalConnection{env: env, prefix: prefix, sessionID: sessionID, tabID: tabID, ctx: ctx, cancel: cancel, onError: onError, onExit: onExit}
	c.visible.Store(true)
	var info terminalInfo
	if err := c.call("create", map[string]any{"shell": shell, "cols": 80, "rows": 24}, &info); err != nil {
		cancel()
		return nil, info, err
	}
	return c, info, nil
}
func (c *terminalConnection) call(method string, params map[string]any, out any) error {
	if params == nil {
		params = map[string]any{}
	}
	params["sessionId"], params["tabId"] = c.sessionID, c.tabID
	return c.env.Host.CallInto(c.prefix+"terminal."+method, params, out)
}
func (c *terminalConnection) fail(err error) {
	c.failureMu.Lock()
	if c.failure == nil {
		c.failure = err
	}
	c.failureMu.Unlock()
	c.errorOnce.Do(func() {
		if c.onError != nil {
			c.onError(err)
		}
	})
}
func (c *terminalConnection) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	for {
		if c.ctx.Err() != nil {
			return 0, io.EOF
		}
		if len(c.pending) > 0 {
			n := copy(buffer, c.pending)
			c.pending = c.pending[n:]
			return n, nil
		}
		if c.exit != nil {
			c.exitOnce.Do(func() {
				if c.onExit != nil {
					c.onExit(int(*c.exit))
				}
			})
			return 0, io.EOF
		}
		var result terminalPoll
		if err := c.call("poll", map[string]any{"offset": c.offset}, &result); err != nil {
			if c.ctx.Err() != nil {
				return 0, io.EOF
			}
			c.fail(err)
			return 0, err
		}
		c.offset = result.Offset
		c.exit = result.Exit
		if result.Reset {
			c.pending = append(c.pending, []byte("\x1bc")...)
		}
		c.pending = append(c.pending, result.Data...)
		if result.Error != "" {
			c.fail(core.Errorf(result.Error))
		}
		if len(c.pending) > 0 || c.exit != nil {
			continue
		}
		delay := 100 * time.Millisecond
		if !c.visible.Load() {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-c.ctx.Done():
			timer.Stop()
			return 0, io.EOF
		case <-timer.C:
		}
	}
}
func (c *terminalConnection) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.ctx.Err() != nil {
		return 0, io.ErrClosedPipe
	}
	c.failureMu.Lock()
	failure := c.failure
	c.failureMu.Unlock()
	if failure != nil {
		return 0, failure
	}
	// JSON strings cannot preserve a UTF-8 sequence split across VT writes.
	// Encode exact bytes; the Host retains the legacy text route for old clients.
	err := c.call("write", map[string]any{"dataBase64": base64.StdEncoding.EncodeToString(data)}, nil)
	if err != nil {
		c.fail(err)
		return 0, err
	}
	return len(data), nil
}
func (c *terminalConnection) Resize(cols, rows int) error {
	if c.ctx.Err() != nil {
		return io.ErrClosedPipe
	}
	err := c.call("resize", map[string]any{"cols": min(1000, max(2, cols)), "rows": min(500, max(2, rows))}, nil)
	if err != nil {
		c.fail(err)
	}
	return err
}
func (c *terminalConnection) Close() error {
	c.closeOnce.Do(func() { c.cancel(); c.closeErr = c.call("dispose", nil, nil) })
	return c.closeErr
}

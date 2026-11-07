//go:build windows

package pdfium

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Client owns exactly one native PDF helper. Calls serialize through its
// private stdio protocol; cancellation kills the owned process to interrupt
// native decoding. No idle helper is retained after closing the PDF tab.
type diagnosticBuffer struct {
	mu    sync.Mutex
	value bytes.Buffer
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	size := len(p)
	remaining := (32 << 10) - b.value.Len()
	if remaining > 0 {
		b.value.Write(p[:min(remaining, len(p))])
	}
	return size, nil
}
func (b *diagnosticBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.value.String() }

type Client struct {
	cancel     context.CancelFunc
	mu         sync.Mutex
	diagnostic diagnosticBuffer
	command    *exec.Cmd
	input      io.WriteCloser
	lines      chan []byte
	done       chan error
	sequence   uint64
	closeOnce  sync.Once
	closeErr   error
	stopOnce   sync.Once
}

func Start(ctx context.Context, executable, library string) (*Client, error) {
	ctx, cancelRead := context.WithCancel(ctx)
	success := false
	defer func() {
		if !success {
			cancelRead()
		}
	}()
	command := exec.CommandContext(ctx, executable, "--library", library)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	c := &Client{cancel: cancelRead, command: command, input: input, lines: make(chan []byte, 1), done: make(chan error, 1)}
	command.Stderr = &c.diagnostic
	if err = command.Start(); err != nil {
		input.Close()
		return nil, err
	}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		defer close(c.lines)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 64<<10), 64<<20)
		for scanner.Scan() {
			select {
			case c.lines <- append([]byte(nil), scanner.Bytes()...):
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { err := command.Wait(); <-readDone; c.done <- err }()
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	select {
	case raw := <-c.lines:
		var ready struct{ Ready bool }
		if json.Unmarshal(raw, &ready) != nil || !ready.Ready {
			c.stop()
			return nil, errors.New("PDF helper could not initialize")
		}
	case <-readyCtx.Done():
		c.stop()
		return nil, readyCtx.Err()
	}
	success = true
	return c, nil
}
func (c *Client) Call(ctx context.Context, request Request) (Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sequence++
	request.ID = c.sequence
	if err := json.NewEncoder(c.input).Encode(request); err != nil {
		return Response{}, errors.New("PDF helper is unavailable")
	}
	select {
	case raw, ok := <-c.lines:
		if !ok {
			return Response{}, fmt.Errorf("PDF helper stopped while decoding: %s", strings.TrimSpace(c.diagnostic.String()))
		}
		var response Response
		if err := json.Unmarshal(raw, &response); err != nil || response.ID != request.ID {
			c.stop()
			return Response{}, errors.New("invalid PDF helper response")
		}
		if response.Error != "" {
			return response, errors.New(response.Error)
		}
		return response, nil
	case <-ctx.Done():
		c.stop()
		return Response{}, ctx.Err()
	}
}
func (c *Client) stop() {
	c.stopOnce.Do(func() {
		// CommandContext is the sole process terminator. Do not close stdin
		// before its cancellation runs: native EOF can win that race, after
		// which Windows rejects TerminateProcess while Wait has not reaped it.
		// Cmd.Wait owns closing StdinPipe once the process has exited.
		c.cancel()
	})
}
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.stop()
		select {
		case err := <-c.done:
			if _, ok := err.(*exec.ExitError); !ok {
				c.closeErr = err
			}
		case <-time.After(5 * time.Second):
			c.closeErr = errors.New("PDF helper did not exit")
		}
	})
	return c.closeErr
}

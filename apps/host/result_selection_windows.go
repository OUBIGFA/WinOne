//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"
	"unicode/utf8"
	"winone/internal/configjson"
	"winone/internal/host"
	"winone/internal/resultwindow"
	"winone/internal/selection"
)

type resultSelectionRead struct {
	session string
	pid     uint32
	reply   chan selection.Selection
}

// Read the owned result's selected text over its authenticated local channel.
// The request ID binds a reply to one selection gesture, not the next window.
func (s *resultWindows) readSelection(ctx context.Context, event selection.Event) (selection.Selection, bool, error) {
	if s == nil {
		return selection.Selection{}, false, nil
	}
	s.mu.Lock()
	var entry *resultSession
	for _, candidate := range s.sessions {
		if candidate.attached && candidate.window == strconv.FormatUint(uint64(event.Window), 10) && candidate.process != nil && uint32(candidate.process.cmd.Process.Pid) == event.ProcessID {
			entry = candidate
			break
		}
	}
	if entry == nil {
		s.mu.Unlock()
		return selection.Selection{}, false, nil
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		s.mu.Unlock()
		return selection.Selection{}, true, err
	}
	id := hex.EncodeToString(nonce[:])
	read := &resultSelectionRead{entry.id, event.ProcessID, make(chan selection.Selection, 1)}
	if s.selectionReads == nil {
		s.selectionReads = map[string]*resultSelectionRead{}
	}
	s.selectionReads[id] = read
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.selectionReads, id); s.mu.Unlock() }()
	if err := entry.process.send(resultwindow.Command{Kind: "read-selection", ID: entry.id, Request: id}); err != nil {
		return selection.Selection{}, true, err
	}
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	select {
	case value := <-read.reply:
		return value, true, nil
	case <-ctx.Done():
		return selection.Selection{}, true, ctx.Err()
	case <-entry.cancelled:
		return selection.Selection{}, true, context.Canceled
	case <-entry.process.done:
		return selection.Selection{}, true, errors.New("结果窗口已关闭")
	}
}

func (s *resultWindows) acceptSelection(r host.Request) host.Response {
	var input struct {
		ID      string `json:"id"`
		Request string `json:"request"`
		Text    string `json:"text"`
	}
	fail := func() host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "result_selection", Message: "结果选区已失效"}}
	}
	if configjson.DecodeObject(r.Params, &input) != nil || len(input.Text) > 65536 || !utf8.ValidString(input.Text) {
		return fail()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	read := s.selectionReads[input.Request]
	if read == nil || read.session != input.ID || read.pid != r.PeerPID || s.sessions[input.ID] == nil {
		return fail()
	}
	value := selection.Selection{Status: "empty"}
	if input.Text != "" {
		value.Status = "selected"
		value.Text = input.Text
	}
	select {
	case read.reply <- value:
	default:
		return fail()
	}
	return host.Response{ID: r.ID, Result: json.RawMessage(`{"accepted":true}`)}
}

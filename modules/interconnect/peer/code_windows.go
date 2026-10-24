package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Code transport belongs to the same persistent share. Only croc's existing
// options are forwarded; paths always come from the published file pool.
type codeSpecification struct {
	Code      string          `json:"code"`
	Options   json.RawMessage `json:"options"`
	Socks5    string          `json:"socks5"`
	HTTPProxy string          `json:"httpProxy"`
}

func codeFailure(err error) json.RawMessage {
	data, _ := json.Marshal(map[string]any{"status": "failed", "error": err.Error()})
	return data
}

// Caller owns shareMu or is the initial setup goroutine. This is intentionally
// independent of Tailcat: a public relay outage cannot revoke the phone link.
func (t *task) restartCode(ctx context.Context) {
	if t.spec.Transfer == nil {
		return
	}
	t.mu.Lock()
	old := t.codeTransfer
	t.codeTransfer = nil
	t.mu.Unlock()
	if old != nil {
		if err := old.close(); err != nil {
			t.setCodeFailure(err)
			return
		}
	}
	raw, err := t.lan.call(ctx, "snapshot", nil)
	if err != nil {
		t.setCodeFailure(err)
		return
	}
	var pool struct {
		Items []struct {
			ID      string
			Kind    string
			Content string
		}
	}
	if err = json.Unmarshal(raw, &pool); err != nil {
		t.setCodeFailure(err)
		return
	}
	paths := []string{}
	texts := []string{}
	for _, item := range pool.Items {
		if item.Kind == "text" {
			texts = append(texts, item.Content)
			continue
		}
		raw, err = t.lan.call(ctx, "item_path", item.ID)
		if err != nil {
			t.setCodeFailure(err)
			return
		}
		var path string
		if err = json.Unmarshal(raw, &path); err != nil {
			t.setCodeFailure(err)
			return
		}
		t.mu.Lock()
		if mapped, ok := t.codeSources[path]; ok {
			path = mapped
		}
		t.mu.Unlock()
		paths = append(paths, path)
	}
	if len(paths) == 0 && len(texts) == 0 {
		t.mu.Lock()
		t.state.Transfer = json.RawMessage(`{"status":"cancelled"}`)
		t.mu.Unlock()
		return
	}
	directory := filepath.Join(t.spec.DataDir, "code", rand.Text())
	text := strings.Join(texts, string(rune(10))+string(rune(10)))
	// Croc's text mode replaces file selections. Mixed pools therefore package
	// their text as a regular file, keeping every published item in the send.
	if len(paths) > 0 && text != "" {
		if err = os.MkdirAll(directory, 0700); err != nil {
			t.setCodeFailure(err)
			return
		}
		path := filepath.Join(directory, "Shared text.txt")
		if err = os.WriteFile(path, []byte(text), 0600); err != nil {
			t.setCodeFailure(err)
			return
		}
		paths = append(paths, path)
		text = ""
	}
	p, err := childProcess("WinOne.Interconnect.Transfer.exe")
	if err != nil {
		t.setCodeFailure(err)
		return
	}
	spec := t.spec.Transfer
	options := spec.Options
	if len(options) == 0 {
		options = json.RawMessage(`{}`)
	}
	_, err = p.call(ctx, "start", map[string]any{"mode": "send", "paths": paths, "text": text, "dataDir": directory, "code": spec.Code, "options": options, "socks5": spec.Socks5, "httpProxy": spec.HTTPProxy})
	if err != nil {
		p.close()
		t.setCodeFailure(err)
		return
	}
	t.mu.Lock()
	t.codeTransfer = p
	t.state.Transfer = json.RawMessage(`{"status":"running"}`)
	t.mu.Unlock()
}

func (t *task) setCodeFailure(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.Transfer = codeFailure(err)
}

func (t *task) shareCommand(method string, params json.RawMessage) error {
	t.mu.Lock()
	if t.lan == nil || !t.state.Ready || t.ctx.Err() != nil {
		t.mu.Unlock()
		return errors.New("共享未运行")
	}
	t.connections.Add(1)
	t.mu.Unlock()
	defer t.connections.Done()
	t.shareMu.Lock()
	defer t.shareMu.Unlock()
	switch method {
	case "code_restart":
		t.restartCode(t.ctx)
		return nil
	case "code_stop", "code_answer":
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.codeTransfer == nil {
			return errors.New("口令传输未运行")
		}
		if method == "code_answer" {
			_, err := t.codeTransfer.call(t.ctx, "answer", params)
			return err
		}
		err := t.codeTransfer.close()
		t.codeTransfer = nil
		t.state.Transfer = json.RawMessage(`{"status":"cancelled"}`)
		if err != nil {
			t.state.Transfer = codeFailure(err)
		}
		return err
	case "add_text":
		var text string
		if err := decode(params, &text); err != nil {
			return err
		}
		if _, err := t.lan.call(t.ctx, "add_text", text); err != nil {
			return err
		}
		t.restartCode(t.ctx)
		return nil
	case "save_as":
		_, err := t.lan.call(t.ctx, "save_as", params)
		return err
	default:
		return fmt.Errorf("未知分享操作: %s", method)
	}
}

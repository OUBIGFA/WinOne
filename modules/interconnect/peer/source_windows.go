package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/tailscale/tailcat"
	"tailscale.com/types/key"
)

// Children inherit the Host's kill-on-close Windows Job. This adapter speaks
// only the same private stdio commands as the Host, never a network protocol.
type child struct {
	mu    sync.Mutex
	cmd   *exec.Cmd
	input io.WriteCloser
	lines chan []byte
	done  chan struct{}
	seq   int
	err   error
}

func childProcess(name string) (*child, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(exe), name))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Stderr = io.Discard
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	p := &child{cmd: cmd, input: input, lines: make(chan []byte, 1), done: make(chan struct{})}
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 8<<20)
		for scanner.Scan() {
			select {
			case p.lines <- append([]byte(nil), scanner.Bytes()...):
			default:
				p.cmd.Process.Kill()
			}
		}
		p.err = errors.Join(scanner.Err(), cmd.Wait())
		close(p.done)
	}()
	return p, nil
}
func (p *child) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.seq++
	id := fmt.Sprint(p.seq)
	frame, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if len(frame) > 2<<20 {
		return nil, errors.New("共享命令超过 2 MiB")
	}
	stop := context.AfterFunc(ctx, func() { p.cmd.Process.Kill() })
	defer stop()
	if _, err = p.input.Write(append(frame, 10)); err != nil {
		return nil, err
	}
	var raw []byte
	select {
	case raw = <-p.lines:
	case <-p.done:
		select {
		case raw = <-p.lines:
		default:
			return nil, fmt.Errorf("共享子进程退出: %v", p.err)
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var response struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err = decode(raw, &response); err != nil || response.ID != id {
		p.cmd.Process.Kill()
		return nil, errors.New("共享子进程响应损坏")
	}
	if response.Error != "" {
		return nil, errors.New(response.Error)
	}
	return response.Result, nil
}
func (p *child) close() error {
	select {
	case <-p.done:
		return p.err
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := p.call(ctx, "stop", nil)
	p.input.Close()
	select {
	case <-p.done:
	case <-ctx.Done():
		p.cmd.Process.Kill()
		<-p.done
	}
	return errors.Join(err, p.err)
}

type storedIdentity struct {
	Key     key.NodePrivate
	PSK     tailcat.PresharedKey
	Address tailcat.Addr `json:",omitempty"`
}

func (t *task) identity() error {
	if !filepath.IsAbs(t.spec.DataDir) {
		return errors.New("共享数据目录无效")
	}
	if err := os.MkdirAll(t.spec.DataDir, 0700); err != nil {
		return err
	}
	file := filepath.Join(t.spec.DataDir, "identity.json")
	data, err := os.ReadFile(file)
	var value storedIdentity
	if errors.Is(err, os.ErrNotExist) {
		value = storedIdentity{Key: key.NewNode(), PSK: tailcat.NewPresharedKey()}
		data, err = json.Marshal(value)
		if err != nil {
			return err
		}
		var output *os.File
		output, err = os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = output.Write(data)
		err = errors.Join(err, output.Sync(), output.Close())
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if err = json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("共享身份文件损坏: %w", err)
	}
	if value.Key.IsZero() || value.PSK.IsZero() {
		return errors.New("共享身份文件不完整")
	}
	t.spec.Key = value.Key
	t.spec.PSK = value.PSK
	t.savedAddress = value.Address
	return nil
}

// Persist the actual bootstrap region as part of the upstream address. Keeping
// keys alone is insufficient: a later latency-based region choice changes it.
func (t *task) saveAddress(address tailcat.Addr) error {
	if t.savedAddress != "" && t.savedAddress != address {
		return errors.New("恢复的分享地址与原记录不一致")
	}
	data, err := json.Marshal(storedIdentity{Key: t.spec.Key, PSK: t.spec.PSK, Address: address})
	if err != nil {
		return err
	}
	file := filepath.Join(t.spec.DataDir, "identity.json")
	pending := file + ".pending"
	output, err := os.OpenFile(pending, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = output.Write(data)
	if err = errors.Join(err, output.Sync(), output.Close()); err != nil {
		return err
	}
	return os.Rename(pending, file)
}
func (t *task) prepareFiles(ctx context.Context, paths []string) ([]string, error) {
	p, err := childProcess("WinOne.Interconnect.Transfer.exe")
	if err != nil {
		return nil, err
	}
	defer p.close()
	directory := filepath.Join(t.spec.DataDir, "prepared", rand.Text())
	if _, err = p.call(ctx, "start", map[string]any{"mode": "prepare_share", "paths": paths, "dataDir": directory}); err != nil {
		return nil, err
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		raw, err := p.call(ctx, "snapshot", nil)
		if err != nil {
			return nil, err
		}
		var state struct {
			Status string
			Error  string
			Detail string
			Result struct{ Paths []string }
		}
		if err = json.Unmarshal(raw, &state); err != nil {
			return nil, err
		}
		if state.Status == "completed" {
			return state.Result.Paths, nil
		}
		if state.Status != "running" {
			return nil, fmt.Errorf("准备共享内容失败: %s", state.Error)
		}
		t.mu.Lock()
		t.state.Detail = state.Detail
		t.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
func (t *task) prepareSource(ctx context.Context) error {
	p, err := childProcess("WinOne.Interconnect.Lan.exe")
	if err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			p.close()
		}
	}()
	raw, err := p.call(ctx, "start", map[string]any{"bind": "127.0.0.1", "port": 0, "dataDir": filepath.Join(t.spec.DataDir, "lan"), "downloadDir": filepath.Join(t.spec.DataDir, "received")})
	if err != nil {
		return err
	}
	var info struct{ URL string }
	if err = json.Unmarshal(raw, &info); err != nil {
		return err
	}
	if !t.spec.Resume && len(t.spec.Paths) > 0 {
		paths, err := t.prepareFiles(ctx, t.spec.Paths)
		if err != nil {
			return err
		}
		if _, err = p.call(ctx, "add_files", paths); err != nil {
			return err
		}
	}
	t.spec.ShareURL = info.URL
	t.mu.Lock()
	t.lan = p
	t.state.Detail = ""
	t.mu.Unlock()
	success = true
	return nil
}
func (t *task) changeShare(method string, params []byte) error {
	t.mu.Lock()
	if t.lan == nil || !t.state.Ready || t.state.Updating || t.ctx.Err() != nil {
		t.mu.Unlock()
		return errors.New("共享尚未就绪或正在更新")
	}
	var paths []string
	var id string
	var err error
	if method == "add" {
		err = decode(params, &paths)
	} else {
		err = decode(params, &id)
	}
	if err != nil {
		t.mu.Unlock()
		return err
	}
	t.state.Updating = true
	t.connections.Add(1)
	t.mu.Unlock()
	go func() {
		defer t.connections.Done()
		t.shareMu.Lock()
		defer t.shareMu.Unlock()
		var err error
		if method == "add" {
			var prepared []string
			prepared, err = t.prepareFiles(t.ctx, paths)
			if err == nil {
				_, err = t.lan.call(t.ctx, "add_files", prepared)
			}
		} else {
			_, err = t.lan.call(t.ctx, "remove", id)
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		t.state.Updating = false
		t.state.Detail = ""
		if err != nil {
			t.state.Error = err.Error()
		} else {
			t.state.Error = ""
		}
	}()
	return nil
}

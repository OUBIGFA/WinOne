//go:build windows

// Package interconnect owns WinOne's adapter processes, never transfer protocols.
package interconnect

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
	"winone/internal/configjson"
	"winone/internal/settings"
)

type Preferences struct {
	Bind        string `json:"bind"`
	Port        uint16 `json:"port"`
	DownloadDir string `json:"downloadDir"`
}
type Manager struct {
	mu               sync.Mutex
	executable, data string
	preferences      Preferences
	process          *worker
	transfers        *Transfers
	general          General
}

func New(executable, data string) (*Manager, error) {
	if !filepath.IsAbs(executable) || !filepath.IsAbs(data) {
		return nil, errors.New("互联助手运行路径无效")
	}
	m := &Manager{executable: executable, data: data, preferences: Preferences{Bind: "0.0.0.0", Port: 5421, DownloadDir: filepath.Join(data, "received")}}
	content, err := os.ReadFile(filepath.Join(data, "preferences.json"))
	if err == nil {
		if err = configjson.DecodeObject(content, &m.preferences); err != nil {
			return nil, err
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	m.transfers, err = NewTransfers(filepath.Join(filepath.Dir(executable), "WinOne.Interconnect.Transfer.exe"), filepath.Join(filepath.Dir(data), "transfer"))
	if err != nil {
		return nil, err
	}
	if err = m.readGeneral(); err != nil {
		return nil, err
	}
	return m, nil
}

// Calls are serialized by the private stdio protocol. A timed-out operation
// retires its worker, so a late reply cannot satisfy a later command.
func (m *Manager) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if strings.HasPrefix(method, "transfer_") || strings.HasPrefix(method, "share_") {
		if method == "transfer_start" {
			var spec map[string]any
			if err := configDecode(params, &spec); err != nil {
				return nil, err
			}
			if mode := spec["mode"]; mode == "receive" || mode == "store_receive" {
				if value, _ := spec["outputDir"].(string); value == "" {
					spec["outputDir"] = m.General().DownloadDir
				}
			}
			var err error
			params, err = json.Marshal(spec)
			if err != nil {
				return nil, err
			}
		}
		return m.transfers.Call(ctx, method, params)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if method == "stop" {
		return json.RawMessage("null"), m.stopLocked()
	}
	if method == "start" {
		if m.process != nil {
			return nil, errors.New("局域网共享已经运行")
		}
		var preferences Preferences
		if err := configjson.DecodeObject(params, &preferences); err != nil {
			return nil, err
		}
		address, err := netip.ParseAddr(preferences.Bind)
		if err != nil || !address.Is4() || !filepath.IsAbs(preferences.DownloadDir) {
			return nil, errors.New("请选择 IPv4 地址和有效的接收目录")
		}
		p, err := startWorker(m.executable)
		if err != nil {
			return nil, err
		}
		options := map[string]any{"bind": preferences.Bind, "port": preferences.Port, "dataDir": m.data, "downloadDir": preferences.DownloadDir}
		result, err := p.call(ctx, "start", options)
		if err != nil {
			p.close()
			return nil, err
		}
		data, err := json.Marshal(preferences)
		if err == nil {
			err = settings.WriteAtomic(filepath.Join(m.data, "preferences.json"), data)
		}
		if err != nil {
			p.close()
			return nil, err
		}
		m.preferences = preferences
		m.process = p
		return result, nil
	}
	if method == "get" {
		var snapshot json.RawMessage = json.RawMessage("null")
		var addresses json.RawMessage
		p := m.process
		if p == nil {
			var err error
			p, err = startWorker(m.executable)
			if err != nil {
				return nil, err
			}
			defer p.close()
		}
		var err error
		addresses, err = p.call(ctx, "addresses", nil)
		if err == nil && m.process != nil {
			snapshot, err = p.call(ctx, "snapshot", nil)
		}
		if err != nil {
			if m.process != nil {
				m.stopLocked()
			}
			return nil, err
		}
		return json.Marshal(map[string]any{"settings": m.preferences, "running": m.process != nil, "addresses": addresses, "snapshot": snapshot})
	}
	switch method {
	case "add_files", "add_text", "remove", "item_path", "download_name", "save_as":
	default:
		return nil, errors.New("不支持的互联助手操作")
	}
	if m.process == nil {
		return nil, errors.New("请先启动局域网共享")
	}
	result, err := m.process.call(ctx, method, params)
	if err != nil && (ctx.Err() != nil || m.process.retired()) {
		m.stopLocked()
	}
	return result, err
}

func (m *Manager) stopLocked() error {
	if m.process == nil {
		return nil
	}
	p := m.process
	m.process = nil
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := p.call(ctx, "stop", nil)
	return errors.Join(err, p.close())
}
func (m *Manager) Close() error {
	m.mu.Lock()
	err := m.stopLocked()
	m.mu.Unlock()
	return errors.Join(err, m.transfers.Close())
}

type worker struct {
	cmd        *exec.Cmd
	input      io.WriteCloser
	job        windows.Handle
	responses  chan json.RawMessage
	done       chan struct{}
	halt       chan struct{}
	retireOnce sync.Once
	err        error
	sequence   uint64
}

func startWorker(executable string, args ...string) (*worker, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	cmd := exec.Command(executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Dir = filepath.Dir(executable)
	input, err := cmd.StdinPipe()
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		windows.CloseHandle(job)
		return nil, err
	}
	// Transferred text, paths and capabilities must not enter ordinary logs.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		windows.CloseHandle(job)
		return nil, err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, handle)
		windows.CloseHandle(handle)
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		input.Close()
		output.Close()
		windows.CloseHandle(job)
		return nil, err
	}
	p := &worker{cmd: cmd, input: input, job: job, responses: make(chan json.RawMessage, 1), done: make(chan struct{}), halt: make(chan struct{})}
	// No command is sent until kill-on-close ownership is established.
	go func() {
		reader := bufio.NewScanner(output)
		reader.Buffer(make([]byte, 4096), 8*1024*1024)
		for reader.Scan() {
			select {
			case p.responses <- append(json.RawMessage(nil), reader.Bytes()...):
			case <-p.halt:
			}
		}
		if reader.Err() != nil {
			p.retire()
		}
		p.err = errors.Join(reader.Err(), cmd.Wait())
		close(p.done)
	}()
	return p, nil
}
func (p *worker) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.sequence++
	id := fmt.Sprint(p.sequence)
	var frame bytes.Buffer
	if err := json.NewEncoder(&frame).Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	if frame.Len() > 2*1024*1024 {
		return nil, errors.New("共享请求超过 2 MiB")
	}
	// Anonymous-pipe writes can block when the worker stops reading. Retiring
	// the job unblocks both sides, including a pending write, before returning.
	written := make(chan error, 1)
	go func() { _, err := p.input.Write(frame.Bytes()); written <- err }()
	select {
	case err := <-written:
		if err != nil {
			p.retire()
			return nil, err
		}
	case <-ctx.Done():
		p.retire()
		<-written
		return nil, ctx.Err()
	}
	decode := func(data []byte) (json.RawMessage, error) {
		var reply struct {
			ID     string          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(data, &reply); err != nil {
			p.retire()
			return nil, err
		}
		if reply.ID != id {
			p.retire()
			return nil, errors.New("共享进程响应不匹配")
		}
		if reply.Error != "" {
			return nil, errors.New(reply.Error)
		}
		return reply.Result, nil
	}
	select {
	case data := <-p.responses:
		return decode(data)
	case <-p.done:
		select {
		case data := <-p.responses:
			return decode(data)
		default:
		}
		p.retire()
		return nil, fmt.Errorf("共享进程已退出: %v", p.err)
	case <-ctx.Done():
		// Retire the entire instance before any subsequent command can be submitted.
		p.retire()
		return nil, ctx.Err()
	}
}
func (p *worker) retire() {
	p.retireOnce.Do(func() { close(p.halt); windows.TerminateJobObject(p.job, 1) })
}
func (p *worker) retired() bool {
	select {
	case <-p.halt:
		return true
	default:
		return false
	}
}
func (p *worker) close() error {
	p.input.Close()
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		p.retire()
		<-p.done
	}
	windows.CloseHandle(p.job)
	return p.err
}

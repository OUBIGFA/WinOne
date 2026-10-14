//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/diagnostics"
	"winone/internal/host"
	"winone/internal/protocol"
	"winone/internal/resultwindow"
	"winone/internal/transport"
)

type resultProcess struct {
	cmd        *exec.Cmd
	input      net.Conn
	done       chan struct{}
	writes     chan struct{}
	writeOnce  sync.Once
	writeError error
}

func (p *resultProcess) send(command resultwindow.Command) error {
	data, err := json.Marshal(command)
	if err != nil {
		return err
	}
	// Bound both queue admission and I/O. A renderer stuck in a native exit
	// callback must not hold the Host's actions or shutdown forever.
	deadline := time.Now().Add(2 * time.Second)
	p.writeOnce.Do(func() { p.writes = make(chan struct{}, 1) })
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case p.writes <- struct{}{}:
		defer func() { <-p.writes }()
	case <-p.done:
		return errors.New("结果宿主已退出，请重试")
	case <-timer.C:
		return errors.New("结果界面响应超时，请关闭结果窗口后重试")
	}
	if p.writeError != nil {
		return p.writeError
	}
	if err = p.input.SetWriteDeadline(deadline); err == nil {
		err = protocol.WriteFrame(p.input, data)
	}
	if err != nil {
		// A failed write may have delivered only part of the frame. Never
		// append another command to that stream or report it as successful.
		p.writeError = fmt.Errorf("结果界面未响应或连接已断开，请关闭结果窗口后重试：%w", err)
		p.input.Close()
		return p.writeError
	}
	return nil
}

type resultSession struct {
	id, token string
	source    string
	creator   uint32
	image     []byte
	process   *resultProcess
	ready     chan struct{}
	attached  bool
	window    string
	cancelled chan struct{}
	closing   bool
}
type resultWindows struct {
	mu                    sync.Mutex
	ctx                   context.Context
	executable, pipe      string
	logDirectory          string
	appearance            func() (string, string)
	process               *resultProcess
	sessions              map[string]*resultSession
	ai                    *aiService
	validateWindow        func(string, uint32) error
	quickShow             sync.Mutex
	quickAssistantAllowed bool
	selectionAllowed      bool
	selectionReads        map[string]*resultSelectionRead
	claimComposerHint     func() (bool, error)
}

func registerResultServices(service *host.Service, ctx context.Context, directory, executable, pipe string, appearance func() (string, string)) *resultWindows {
	engine := newAIService(ctx, directory, executable)
	bindAIService(service, engine)
	windows := &resultWindows{ctx: ctx, executable: filepath.Join(filepath.Dir(executable), "shared", "ui", "WinOne.Results.exe"), pipe: pipe, logDirectory: directory, appearance: appearance, sessions: map[string]*resultSession{}, ai: engine, validateWindow: visibleResultWindow}
	windows.claimComposerHint = claimResultComposerHint
	engine.resultSessions = windows
	service.RegisterModule("results", func() host.Module { return host.Module{ID: "results", State: "ready"} }, windows.handle)
	return windows
}

func (s *resultWindows) handle(request host.Request) host.Response {
	if request.Method == "selected" {
		return s.acceptSelection(request)
	}
	respond := func(value any, err error) host.Response {
		if err != nil {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "result_window", Message: err.Error()}}
		}
		data, _ := json.Marshal(value)
		return host.Response{ID: request.ID, Result: data}
	}
	var input struct {
		State  json.RawMessage    `json:"state,omitempty"`
		ID     string             `json:"id"`
		Token  string             `json:"token,omitempty"`
		Mode   string             `json:"mode,omitempty"`
		Image  *ai.ImageReference `json:"image,omitempty"`
		Window string             `json:"window,omitempty"`
	}
	if err := configjson.DecodeObject(request.Params, &input); err != nil {
		return respond(nil, err)
	}
	switch request.Method {
	case "composerHint.claim":
		if err := s.authorize(input.ID, request.PeerPID); err != nil {
			return respond(nil, err)
		}
		if s.claimComposerHint == nil {
			return respond(nil, errors.New("首次使用提示暂不可用"))
		}
		show, err := s.claimComposerHint()
		return respond(map[string]bool{"show": show}, err)
	case "status":
		s.mu.Lock()
		defer s.mu.Unlock()
		entry := s.sessions[input.ID]
		if entry == nil || entry.creator != request.PeerPID {
			return respond(nil, errors.New("结果窗口不存在或不属于本次操作"))
		}
		pid := 0
		if entry.process != nil {
			pid = entry.process.cmd.Process.Pid
		}
		return respond(map[string]any{"id": input.ID, "processId": pid, "ready": entry.attached, "window": entry.window}, nil)
	case "open":
		if input.Image == nil || (input.Mode != "ocr" && input.Mode != "translate") {
			return respond(nil, errors.New("结果窗口请求无效"))
		}
		err := s.open(input.ID, input.Mode, *input.Image, request.PeerPID)
		if err != nil {
			return respond(nil, err)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		entry := s.sessions[input.ID]
		if entry == nil || !entry.attached || entry.process == nil {
			return respond(nil, errors.New("结果窗口已关闭，请重试"))
		}
		// The foreground capture caller performs the final focus transfer.
		return respond(map[string]any{"id": input.ID, "opened": true, "window": entry.window, "processId": entry.process.cmd.Process.Pid}, nil)
	case "ready":
		s.mu.Lock()
		defer s.mu.Unlock()
		entry := s.sessions[input.ID]
		if entry == nil || entry.process == nil || entry.token != input.Token || entry.process.cmd.Process.Pid != int(request.PeerPID) {
			return respond(nil, errors.New("结果窗口身份不匹配"))
		}
		if !entry.attached {
			if s.validateWindow != nil {
				if err := s.validateWindow(input.Window, request.PeerPID); err != nil {
					return respond(nil, err)
				}
			}
			entry.window = input.Window
			entry.attached = true
			close(entry.ready)
		}
		return respond(map[string]bool{"ready": true}, nil)
	case "closed":
		s.mu.Lock()
		entry := s.sessions[input.ID]
		allowed := entry != nil && entry.process != nil && entry.process.cmd.Process.Pid == int(request.PeerPID)
		s.mu.Unlock()
		if !allowed {
			return respond(nil, errors.New("结果窗口身份不匹配"))
		}
		s.remove(input.ID)
		return respond(map[string]bool{"closed": true}, nil)
	case "cancel":
		s.mu.Lock()
		entry := s.sessions[input.ID]
		if entry != nil && entry.creator != request.PeerPID {
			s.mu.Unlock()
			return respond(nil, errors.New("结果窗口不属于本次截图"))
		}
		var process *resultProcess
		if entry != nil {
			process = entry.process
			delete(s.sessions, input.ID)
			close(entry.cancelled)
		}
		s.mu.Unlock()
		if process != nil {
			s.ai.cancelResultSession(input.ID, uint32(process.cmd.Process.Pid))
			if err := process.send(resultwindow.Command{Kind: "close", ID: input.ID}); err != nil {
				return respond(nil, err)
			}
		}
		return respond(map[string]bool{"cancelled": true}, nil)
	default:
		return respond(nil, errors.New("不支持的结果窗口操作"))
	}
}

func (s *resultWindows) startLocked() (*resultProcess, error) {
	if s.process != nil {
		select {
		case <-s.process.done:
			s.process = nil
		default:
			return s.process, nil
		}
	}
	cmd := exec.Command(s.executable)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()
	reader, input, err := transport.ChildStdin(ctx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	cmd.Stdin = reader
	cmd.WaitDelay = 2 * time.Second
	// No result text or images are written to diagnostic streams or arguments.
	cmd.Stdout = io.Discard
	logOutput, err := diagnostics.NewModuleWriter(s.logDirectory, "results")
	if err != nil {
		input.Close()
		return nil, err
	}
	cmd.Stderr = logOutput
	if err = cmd.Start(); err != nil {
		input.Close()
		logOutput.Close()
		return nil, fmt.Errorf("结果界面无法启动，请修复安装：%w", err)
	}
	p := &resultProcess{cmd: cmd, input: input, done: make(chan struct{})}
	s.process = p
	go func() {
		if err := errors.Join(cmd.Wait(), logOutput.Close()); err != nil {
			fmt.Fprintln(os.Stderr, "Result UI exited:", err)
		}
		input.Close()
		s.mu.Lock()
		var ids []string
		for id, entry := range s.sessions {
			if entry.process == p {
				ids = append(ids, id)
				delete(s.sessions, id)
				close(entry.cancelled)
			}
		}
		if s.process == p {
			s.process = nil
		}
		close(p.done)
		s.mu.Unlock()
		for _, id := range ids {
			s.ai.cancelResultSession(id, uint32(p.cmd.Process.Pid))
		}
	}()
	return p, nil
}

func (s *resultWindows) open(id, mode string, image ai.ImageReference, creator uint32) error {
	return s.openSource(id, mode, &image, "", [2]int32{}, creator, nil)
}

func (s *resultWindows) openSelection(text string, anchor [2]int32, actions []resultwindow.SelectionAction, quick ...resultwindow.QuickOptions) error {
	if strings.TrimSpace(text) == "" || len(text) > 65536 || !utf8.ValidString(text) {
		return errors.New("选中文字无效或过长")
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return err
	}
	return s.openSource(hex.EncodeToString(bytes[:]), "translate", nil, text, anchor, uint32(os.Getpid()), actions)
}

func (s *resultWindows) dismissSelection(point [2]int32, force bool, window uintptr) error {
	s.mu.Lock()
	p := s.process
	s.mu.Unlock()
	if p != nil {
		return p.send(resultwindow.Command{Kind: "dismiss-selection", Point: point, Force: force, Window: uint64(window)})
	}
	return nil
}

func (s *resultWindows) openSource(id, mode string, image *ai.ImageReference, text string, anchor [2]int32, creator uint32, actions []resultwindow.SelectionAction, quick ...resultwindow.QuickOptions) error {
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 16 {
		return errors.New("结果窗口标识无效")
	}
	s.mu.Lock()
	if image == nil && mode != "chat" && !s.selectionAllowed {
		s.mu.Unlock()
		return errors.New("划词助手已关闭")
	}
	if mode == "chat" && !s.quickAssistantAllowed {
		s.mu.Unlock()
		return errors.New("快捷助手已关闭")
	}
	if _, exists := s.sessions[id]; exists {
		s.mu.Unlock()
		return errors.New("结果窗口请求已提交")
	}
	if len(s.sessions) >= 8 {
		s.mu.Unlock()
		return errors.New("结果窗口较多，请先关闭不需要的窗口")
	}
	// Reserve a slot before copying a potentially large image.
	entry := &resultSession{id: id, creator: creator, ready: make(chan struct{}), cancelled: make(chan struct{})}
	if image == nil {
		entry.source = "selection"
		if mode == "chat" {
			entry.source = "quick-assistant"
		}
	}
	s.sessions[id] = entry
	s.mu.Unlock()
	defer func() {
		if err != nil {
			s.remove(id)
		}
	}()
	if image != nil {
		entry.image, err = ai.ReadImage(*image, creator)
		if err != nil {
			return err
		}
	}
	var token [16]byte
	if _, err = rand.Read(token[:]); err != nil {
		return err
	}
	entry.token = hex.EncodeToString(token[:])
	theme, language := s.appearance()
	command := resultwindow.Command{Kind: "open", Initial: &resultwindow.Initial{ID: id, Token: entry.token, Pipe: s.pipe, HostPID: uint32(os.Getpid()), Mode: mode, Theme: theme, Language: language}}
	if len(quick) > 0 {
		command.Initial.Quick = quick[0]
	}
	if image == nil {
		command.Initial.Source = entry.source
		command.Initial.Text = text
		command.Initial.Toolbar = mode != "chat"
		command.Initial.Anchor = anchor
		command.Initial.Actions = actions
	}
	s.mu.Lock()
	if s.sessions[id] != entry || entry.closing {
		s.mu.Unlock()
		err = errors.New("结果窗口已取消")
		return err
	}
	entry.process, err = s.startLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	// Do not keep the session lock across a transport write. Status, close and
	// module stop must remain available when the renderer stops consuming input.
	select {
	case <-entry.cancelled:
		return errors.New("结果窗口已取消")
	default:
	}
	if err = entry.process.send(command); err != nil {
		return err
	}
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	select {
	case <-entry.ready:
		return nil
	case <-entry.process.done:
		err = errors.New("结果界面未能显示，请重试")
	case <-s.ctx.Done():
		err = s.ctx.Err()
	case <-entry.cancelled:
		err = errors.New("结果窗口已取消")
	case <-timer.C:
		err = errors.New("结果界面打开超时，请重试")
	}
	_ = entry.process.send(resultwindow.Command{Kind: "close", ID: id})
	return err
}

func (s *resultWindows) allowSelection(allowed bool) {
	s.mu.Lock()
	s.selectionAllowed = allowed
	s.mu.Unlock()
}
func (s *resultWindows) closeSelection() error { return s.closeSource("selection") }
func (s *resultWindows) closeSource(source string) error {
	s.mu.Lock()
	entries := []*resultSession{}
	for _, entry := range s.sessions {
		if entry.source == source {
			// Stop admission before dropping the lock: an open that has not
			// started its renderer must not race past this close operation.
			entry.closing = true
			entries = append(entries, entry)
		}
	}
	s.mu.Unlock()
	for _, entry := range entries {
		if entry.process != nil {
			if err := entry.process.send(resultwindow.Command{Kind: "close", ID: entry.id}); err != nil {
				select {
				case <-entry.process.done:
				default:
					return err
				}
			}
		}
		if entry.process == nil {
			s.remove(entry.id)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for _, entry := range entries {
		if entry.process == nil {
			continue
		}
		// Keep the session addressable until its close acknowledgement, so
		// the quick assistant can retain its draft before the HWND disappears.
		timer := time.NewTimer(max(0, time.Until(deadline)))
		select {
		case <-entry.cancelled:
		case <-entry.process.done:
		case <-timer.C:
			return errors.New("助手窗口尚未关闭，请检查窗口中的提示后重试")
		}
		timer.Stop()
		s.remove(entry.id)
	}
	return nil
}

func (s *resultWindows) authorize(id string, pid uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.sessions[id]
	if entry == nil || entry.process == nil || !entry.attached || entry.process.cmd.Process.Pid != int(pid) {
		return errors.New("结果窗口已关闭或不属于当前进程")
	}
	return nil
}
func (s *resultWindows) image(id string, pid uint32) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.sessions[id]
	if entry == nil || entry.process == nil || !entry.attached || entry.process.cmd.Process.Pid != int(pid) {
		return nil, errors.New("结果图片已释放")
	}
	return entry.image, nil
}
func (s *resultWindows) remove(id string) {
	s.mu.Lock()
	entry := s.sessions[id]
	delete(s.sessions, id)
	if entry != nil {
		entry.image = nil
	}
	if entry != nil && entry.cancelled != nil {
		close(entry.cancelled)
	}
	s.mu.Unlock()
	if entry != nil && entry.process != nil {
		s.ai.cancelResultSession(id, uint32(entry.process.cmd.Process.Pid))
	}
}
func (s *resultWindows) active() bool { s.mu.Lock(); defer s.mu.Unlock(); return len(s.sessions) > 0 }
func (s *resultWindows) stop() error {
	s.mu.Lock()
	p := s.process
	s.mu.Unlock()
	if p == nil {
		return nil
	}
	if err := p.send(resultwindow.Command{Kind: "quit"}); err != nil {
		select {
		case <-p.done:
			return nil
		default:
			return errors.New("结果窗口暂时无法关闭，请先手动关闭")
		}
	}
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("请先完成或关闭结果窗口，再退出 WinOne")
	}
}
func (s *resultWindows) dispose() error {
	s.mu.Lock()
	p := s.process
	s.mu.Unlock()
	if p == nil {
		return nil
	}
	p.input.Close()
	select {
	case <-p.done:
		return nil
	case <-time.After(3 * time.Second):
		killErr := p.cmd.Process.Kill()
		select {
		case <-p.done:
			return nil
		case <-time.After(3 * time.Second):
			return errors.Join(errors.New("结果宿主尚未退出，请检查占用后重试"), killErr)
		}
	}
}

func visibleResultWindow(value string, pid uint32) error {
	handle, err := strconv.ParseUint(value, 10, 64)
	if err != nil || handle == 0 {
		return errors.New("结果窗口句柄无效")
	}
	api := windows.NewLazySystemDLL("user32.dll")
	var actual uint32
	api.NewProc("GetWindowThreadProcessId").Call(uintptr(handle), uintptr(unsafe.Pointer(&actual)))
	visible, _, _ := api.NewProc("IsWindowVisible").Call(uintptr(handle))
	if actual != pid || visible == 0 {
		return errors.New("结果窗口尚未显示")
	}
	return nil
}

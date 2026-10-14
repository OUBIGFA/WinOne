//go:build windows

package chat

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unsafe"
)

type TerminalInfo struct {
	ID        string `json:"id"`
	Shell     string `json:"shell"`
	Directory string `json:"directory"`
	PID       uint32 `json:"pid"`
}

var terminalAttributeUpdate = windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")
type TerminalPoll struct {
	Data   []byte  `json:"data"`
	Offset uint64  `json:"offset"`
	Reset  bool    `json:"reset"`
	Exit   *uint32 `json:"exit,omitempty"`
	Error  string  `json:"error,omitempty"`
}
type TerminalManager struct {
	mu       sync.Mutex
	sessions map[string]*terminalSession
	closed   bool
}
type terminalSession struct {
	mu                    sync.Mutex
	owner                 uint32
	session               string
	info                  TerminalInfo
	input, output         *os.File
	console, job, process windows.Handle
	data                  []byte
	base                  uint64
	exit                  *uint32
	failure               string
	once                  sync.Once
}

// NewTerminalManager owns real Windows pseudo consoles. A job is assigned
// before the suspended shell can start, so child processes cannot escape the
// tab/window lifetime. Hiding a tab does not dispose its terminal.
func NewTerminalManager(ctx context.Context) *TerminalManager {
	m := &TerminalManager{sessions: map[string]*terminalSession{}}
	go func() { <-ctx.Done(); m.Close() }()
	return m
}
func terminalSize(cols, rows int) (windows.Coord, error) {
	if cols < 2 || cols > 1000 || rows < 2 || rows > 500 {
		return windows.Coord{}, errors.New("终端尺寸无效")
	}
	return windows.Coord{X: int16(cols), Y: int16(rows)}, nil
}
func terminalShell(shell string) (string, string, error) {
	if shell == "" {
		shell = "powershell"
	}
	var args string
	switch shell {
	case "powershell", "pwsh":
		args = " -NoLogo -NoProfile"
	case "cmd":
		args = " /D"
	default:
		return "", "", errors.New("不支持的终端 Shell")
	}
	path, err := exec.LookPath(shell + ".exe")
	if err != nil {
		return "", "", fmt.Errorf("未找到 %s，请选择已安装的 Shell", shell)
	}
	return path, "\"" + path + "\"" + args, nil
}
func (m *TerminalManager) Create(owner uint32, id, session, directory, shell string, cols, rows int) (TerminalInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return TerminalInfo{}, errors.New("终端服务已关闭")
	}
	if id == "" || len(id) > 200 || strings.ContainsRune(id, 0) {
		return TerminalInfo{}, errors.New("终端标识无效")
	}
	if prior := m.sessions[id]; prior != nil {
		if prior.owner != owner || prior.session != session {
			return TerminalInfo{}, errors.New("终端属于其他会话")
		}
		return prior.info, nil
	}
	directory, err := existingDirectory(directory)
	if err != nil {
		return TerminalInfo{}, err
	}
	size, err := terminalSize(cols, rows)
	if err != nil {
		return TerminalInfo{}, err
	}
	executable, command, err := terminalShell(shell)
	if err != nil {
		return TerminalInfo{}, err
	}
	if shell == "" {
		shell = "powershell"
	}
	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	if err = windows.CreatePipe(&inputRead, &inputWrite, nil, 0); err != nil {
		return TerminalInfo{}, err
	}
	defer windows.CloseHandle(inputRead)
	if err = windows.CreatePipe(&outputRead, &outputWrite, nil, 0); err != nil {
		windows.CloseHandle(inputWrite)
		return TerminalInfo{}, err
	}
	defer windows.CloseHandle(outputWrite)
	s := &terminalSession{owner: owner, session: session, info: TerminalInfo{ID: id, Shell: shell, Directory: directory}, input: os.NewFile(uintptr(inputWrite), "terminal-input"), output: os.NewFile(uintptr(outputRead), "terminal-output")}
	success := false
	defer func() {
		if !success {
			s.stop()
		}
	}()
	if err = windows.CreatePseudoConsole(size, inputRead, outputWrite, 0, &s.console); err != nil {
		return TerminalInfo{}, fmt.Errorf("创建终端失败：%w", err)
	}
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return TerminalInfo{}, err
	}
	defer attributes.Delete()
	// This specific Win32 attribute takes the opaque HPCON value, not a pointer
	// to Go memory. Pass its integer handle at the syscall boundary rather than
	// manufacturing an unsafe.Pointer that the GC/vet could treat as memory.
	updated, _, updateError := terminalAttributeUpdate.Call(uintptr(unsafe.Pointer(attributes.List())), 0, windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(s.console), unsafe.Sizeof(s.console), 0, 0)
	if updated == 0 { return TerminalInfo{}, updateError }
	s.job, err = windows.CreateJobObject(nil, nil)
	if err != nil {
		return TerminalInfo{}, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(s.job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return TerminalInfo{}, err
	}
	// Explicit null standard handles let ConPTY provide console handles. Without
	// STARTF_USESTDHANDLES Windows can inherit the Host/test runner's redirected
	// handles even with inheritHandles=false, bypassing the pseudo console.
	startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})), Flags: windows.STARTF_USESTDHANDLES}, ProcThreadAttributeList: attributes.List()}
	var proc windows.ProcessInformation
	executable16, _ := windows.UTF16PtrFromString(executable)
	command16, _ := windows.UTF16PtrFromString(command)
	directory16, _ := windows.UTF16PtrFromString(directory)
	if err = windows.CreateProcess(executable16, command16, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_SUSPENDED, nil, directory16, &startup.StartupInfo, &proc); err != nil {
		return TerminalInfo{}, fmt.Errorf("启动终端失败：%w", err)
	}
	s.process = proc.Process
	s.info.PID = proc.ProcessId
	defer windows.CloseHandle(proc.Thread)
	if err = windows.AssignProcessToJobObject(s.job, proc.Process); err != nil {
		windows.TerminateProcess(proc.Process, 1)
		windows.CloseHandle(proc.Process)
		s.process = 0
		return TerminalInfo{}, err
	}
	// Drain ConPTY output before resuming: its synchronous pipes can otherwise
	// block startup while the console writes its initial terminal capabilities.
	go s.read()
	if _, err = windows.ResumeThread(proc.Thread); err != nil {
		return TerminalInfo{}, err
	}
	go func() {
		windows.WaitForSingleObject(proc.Process, windows.INFINITE)
		var code uint32
		err := windows.GetExitCodeProcess(proc.Process, &code)
		s.mu.Lock()
		s.exit = &code
		if err != nil {
			s.failure = err.Error()
		}
		s.mu.Unlock()
		windows.CloseHandle(proc.Process)
	}()
	m.sessions[id] = s
	success = true
	return s.info, nil
}
func (s *terminalSession) read() {
	buffer := make([]byte, 32<<10)
	for {
		n, err := s.output.Read(buffer)
		if n > 0 {
			s.mu.Lock()
			s.data = append(s.data, buffer[:n]...)
			if len(s.data) > 8<<20 {
				discard := len(s.data) - (8 << 20)
				s.base += uint64(discard)
				s.data = append([]byte(nil), s.data[discard:]...)
			}
			s.mu.Unlock()
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				s.mu.Lock()
				s.failure = err.Error()
				s.mu.Unlock()
			}
			return
		}
	}
}
func (m *TerminalManager) get(owner uint32, id string) (*terminalSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil || s.owner != owner {
		return nil, errors.New("终端不存在或不属于当前窗口")
	}
	return s, nil
}
func (m *TerminalManager) Poll(owner uint32, id string, offset uint64) (TerminalPoll, error) {
	s, err := m.get(owner, id)
	if err != nil {
		return TerminalPoll{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	end := s.base + uint64(len(s.data))
	reset := offset < s.base || offset > end
	if reset {
		offset = s.base
	}
	return TerminalPoll{Data: append([]byte{}, s.data[offset-s.base:]...), Offset: end, Reset: reset, Exit: s.exit, Error: s.failure}, nil
}
func (m *TerminalManager) Write(owner uint32, id string, data []byte) error {
	if len(data) > 1<<20 {
		return errors.New("终端输入过长")
	}
	s, err := m.get(owner, id)
	if err != nil {
		return err
	}
	_, err = s.input.Write(data)
	return err
}
func (m *TerminalManager) Resize(owner uint32, id string, cols, rows int) error {
	size, err := terminalSize(cols, rows)
	if err != nil {
		return err
	}
	s, err := m.get(owner, id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.console == 0 {
		return errors.New("终端已退出")
	}
	return windows.ResizePseudoConsole(s.console, size)
}
func (m *TerminalManager) Dispose(owner uint32, id string) error {
	m.mu.Lock()
	s := m.sessions[id]
	if s == nil {
		m.mu.Unlock()
		return nil
	}
	if s.owner != owner {
		m.mu.Unlock()
		return errors.New("终端属于其他窗口")
	}
	delete(m.sessions, id)
	m.mu.Unlock()
	s.stop()
	return nil
}
func (s *terminalSession) stop() {
	s.once.Do(func() {
		if s.job != 0 {
			windows.TerminateJobObject(s.job, 1)
			windows.CloseHandle(s.job)
		}
		s.input.Close()
		s.mu.Lock()
		console := s.console
		s.console = 0
		s.mu.Unlock()
		if console != 0 {
			windows.ClosePseudoConsole(console)
		}
		s.output.Close()
	})
}
func (m *TerminalManager) Detach(owner uint32) {
	m.mu.Lock()
	var removed []*terminalSession
	for id, s := range m.sessions {
		if s.owner == owner {
			delete(m.sessions, id)
			removed = append(removed, s)
		}
	}
	m.mu.Unlock()
	for _, s := range removed {
		s.stop()
	}
}
func (m *TerminalManager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	var all []*terminalSession
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.sessions = map[string]*terminalSession{}
	m.mu.Unlock()
	for _, s := range all {
		s.stop()
	}
}

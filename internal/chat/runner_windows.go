//go:build windows

package chat

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
	"winone/internal/transport"
)

type runnerReply struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	OK       bool            `json:"ok"`
	Value    json.RawMessage `json:"value"`
	Error    string          `json:"error"`
	Protocol int             `json:"protocol"`
}

// Rejection is an acknowledged non-execution, unlike a timeout after sending.
// Callers may safely release their running state while retaining the draft.
type CommandRejection struct{ Message string }

func (e *CommandRejection) Error() string { return e.Message }

type rememberedCall struct {
	digest [32]byte
	reply  *runnerReply
}
type runnerDiagnostics struct {
	mu   sync.Mutex
	text []byte
}

func (d *runnerDiagnostics) Write(value []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.text) < 4096 {
		d.text = append(d.text, value[:min(len(value), 4096-len(d.text))]...)
	}
	return len(value), nil
}
func (d *runnerDiagnostics) String() string { d.mu.Lock(); defer d.mu.Unlock(); return string(d.text) }

type Runner struct {
	cmd         *exec.Cmd
	input       net.Conn
	job         windows.Handle
	jobOnce     sync.Once
	ready, done chan struct{}
	writes      chan struct{}
	mu          sync.Mutex
	pending     map[string]chan runnerReply
	remembered  map[string]*rememberedCall
	order       []string
	err         error
	sequence    atomic.Uint64
	prefix      string
}

// loopbackDirectEnvironment keeps worker loopback traffic off the user's proxy.
// Bun's fetch applies HTTP(S)_PROXY to 127.0.0.1 too, unlike Go's
// ProxyFromEnvironment. Worker calls to the Host bridges and the AI gateway
// carry private bearer keys and only exist on this machine, so loopback hosts
// join NO_PROXY. Model providers and IM channels keep the inherited proxy.
// Windows treats variable names case-insensitively, so any NO_PROXY spelling is
// merged into one entry instead of producing an ambiguous duplicate.
func loopbackDirectEnvironment(environment []string) []string {
	const loopback = "localhost,127.0.0.1,::1"
	result := make([]string, 0, len(environment)+1)
	bypass := ""
	for _, entry := range environment {
		if name, value, found := strings.Cut(entry, "="); found && strings.EqualFold(name, "NO_PROXY") {
			bypass = value
			continue
		}
		result = append(result, entry)
	}
	if bypass != "" {
		bypass += ","
	}
	return append(result, "NO_PROXY="+bypass+loopback)
}

// StartRunner uses the same Windows job ownership as the existing AI workers.
// No init or tool command is sent until the process is assigned to the job.
// Closing it also ends normal descendant commands, even after a worker crash.
func StartRunner(ctx context.Context, bun, script, directory string, onEvent func(json.RawMessage)) (*Runner, error) {
	if !filepath.IsAbs(bun) || !filepath.IsAbs(script) || !filepath.IsAbs(directory) {
		return nil, errors.New("Pi 运行路径无效")
	}
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
	pipeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	reader, input, err := transport.ChildStdin(pipeCtx)
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	defer reader.Close()
	cmd := exec.Command(bun, script)
	cmd.Dir = directory
	cmd.Env = loopbackDirectEnvironment(os.Environ())
	cmd.Stdin = reader
	diagnostics := &runnerDiagnostics{}
	cmd.Stderr = diagnostics
	cmd.WaitDelay = time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	// Own both pipe ends so Cmd.Wait cannot close the reader before the final
	// reply is consumed. A dead parent also cannot leave us waiting forever on
	// stdout inherited by one of its descendants: its job is closed by Wait.
	output, childOutput, err := os.Pipe()
	if err != nil {
		input.Close()
		windows.CloseHandle(job)
		return nil, err
	}
	defer childOutput.Close()
	cmd.Stdout = childOutput
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		windows.CloseHandle(job)
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
		windows.CloseHandle(process)
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		input.Close()
		output.Close()
		windows.CloseHandle(job)
		return nil, err
	}
	r := &Runner{cmd: cmd, input: input, job: job, ready: make(chan struct{}), done: make(chan struct{}), writes: make(chan struct{}, 1), pending: map[string]chan runnerReply{}, remembered: map[string]*rememberedCall{}, prefix: rand.Text()}
	exited := make(chan error, 1)
	go func() { err := cmd.Wait(); r.kill(); input.Close(); exited <- err }()
	go func() {
		defer output.Close()
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		ready := false
		for scanner.Scan() {
			raw := append(json.RawMessage(nil), scanner.Bytes()...)
			var value runnerReply
			if err := json.Unmarshal(raw, &value); err != nil {
				r.setError(errors.New("Pi 返回了无效协议数据"))
				r.kill()
				break
			}
			if value.Type == "ready" {
				if ready || value.Protocol != 1 {
					r.setError(errors.New("Pi 协议版本不匹配"))
					r.kill()
					break
				}
				ready = true
				close(r.ready)
				continue
			}
			if !ready {
				r.setError(errors.New("Pi 未完成启动握手"))
				r.kill()
				break
			}
			if value.Type == "response" {
				r.mu.Lock()
				waiter := r.pending[value.ID]
				if prior := r.remembered[value.ID]; prior != nil {
					copy := value
					prior.reply = &copy
				}
				delete(r.pending, value.ID)
				r.mu.Unlock()
				if waiter != nil {
					waiter <- value
				}
			} else if onEvent != nil {
				onEvent(raw)
			}
		}
		if err := scanner.Err(); err != nil {
			r.setError(fmt.Errorf("读取 Pi 输出：%w", err))
			r.kill()
		}
		if err := <-exited; err != nil {
			r.setError(fmt.Errorf("Pi 进程退出：%w；%s", err, diagnostics.String()))
		}
		input.Close()
		r.kill()
		close(r.done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			r.kill()
		case <-r.done:
		}
	}()
	return r, nil
}
func (r *Runner) setError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err == nil {
		r.err = err
	}
}
func (r *Runner) failure() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	return errors.New("Pi 会话进程已结束")
}
func (r *Runner) kill()                 { r.jobOnce.Do(func() { windows.CloseHandle(r.job) }) }
func (r *Runner) Done() <-chan struct{} { return r.done }
func (r *Runner) PID() int              { return r.cmd.Process.Pid }
func (r *Runner) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return r.call(ctx, fmt.Sprintf("%s-%d", r.prefix, r.sequence.Add(1)), method, params, false)
}
func (r *Runner) CallOnce(ctx context.Context, id, method string, params any) (json.RawMessage, error) {
	return r.call(ctx, id, method, params, true)
}
func (r *Runner) call(ctx context.Context, id, method string, params any, remember bool) (json.RawMessage, error) {
	select {
	case <-r.ready:
	case <-r.done:
		return nil, r.failure()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if id == "" || len(id) > 128 {
		return nil, errors.New("Chat 请求标识无效")
	}
	data, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if len(data) > 1024*1024 {
		return nil, errors.New("Chat 请求过大")
	}
	waiter := make(chan runnerReply, 1)
	digest := sha256.Sum256(data)
	r.mu.Lock()
	if old := r.remembered[id]; old != nil {
		if old.digest != digest {
			r.mu.Unlock()
			return nil, errors.New("请求标识被用于不同操作")
		}
		if old.reply == nil {
			r.mu.Unlock()
			return nil, errors.New("之前的请求仍在处理，请读取会话状态")
		}
		if old.reply.OK {
			previous := *old.reply
			r.mu.Unlock()
			return decodeRunnerReply(previous)
		}
		// A definite rejection is not an accepted operation. An explicit retry
		// with the same payload may succeed after its precondition is repaired.
		// Ambiguous timeouts above remain remembered and are never resent here.
		delete(r.remembered, id)
		for index, saved := range r.order {
			if saved == id {
				r.order = append(r.order[:index], r.order[index+1:]...)
				break
			}
		}
	}
	if len(r.pending) >= 32 {
		r.mu.Unlock()
		return nil, errors.New("Chat 请求较多，请稍后重试")
	}
	r.pending[id] = waiter
	if remember {
		r.remembered[id] = &rememberedCall{digest: digest}
		r.order = append(r.order, id)
		if len(r.order) > 64 {
			delete(r.remembered, r.order[0])
			r.order = r.order[1:]
		}
	}
	r.mu.Unlock()
	sent := false
	defer func() {
		r.mu.Lock()
		if r.pending[id] == waiter {
			delete(r.pending, id)
		}
		if !sent {
			delete(r.remembered, id)
		}
		r.mu.Unlock()
	}()
	select {
	case r.writes <- struct{}{}:
	case <-r.done:
		select {
		case response := <-waiter:
			return decodeRunnerReply(response)
		default:
			return nil, r.failure()
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	deadline := time.Now().Add(2 * time.Second)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	if err = r.input.SetWriteDeadline(deadline); err == nil {
		var written int
		written, err = r.input.Write(append(data, '\n'))
		if err == nil && written != len(data)+1 {
			err = io.ErrShortWrite
		}
	}
	sent = err == nil
	<-r.writes
	if err != nil {
		r.setError(fmt.Errorf("Pi 通信失败：%w", err))
		r.kill()
		return nil, err
	}
	select {
	case response := <-waiter:
		return decodeRunnerReply(response)
	case <-r.done:
		select {
		case response := <-waiter:
			return decodeRunnerReply(response)
		default:
			return nil, r.failure()
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func decodeRunnerReply(value runnerReply) (json.RawMessage, error) {
	if !value.OK {
		return nil, &CommandRejection{Message: value.Error}
	}
	return value.Value, nil
}
func (r *Runner) Close(ctx context.Context) error {
	select {
	case <-r.done:
		return nil
	default:
	}
	// EOF is Pi's normal shutdown contract. Its SDK aborts active work and
	// finishes session writes; the job is the bounded fallback, not success.
	r.input.Close()
	select {
	case <-r.done:
		r.mu.Lock()
		err := r.err
		r.mu.Unlock()
		return err
	case <-ctx.Done():
		r.kill()
	}
	select {
	case <-r.done:
		return fmt.Errorf("Pi 超过正常退出期限：%w", ctx.Err())
	case <-time.After(3 * time.Second):
		return errors.New("Pi 进程尚未退出")
	}
}

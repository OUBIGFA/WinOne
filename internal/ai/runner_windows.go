//go:build windows

package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
	"unsafe"
)

// The initial command is withheld until the worker is in a kill-on-close job.
// Account CLIs therefore share the request's lifetime, including cancellation.
type aiProcess struct {
	cmd      *exec.Cmd
	input    io.WriteCloser
	output   io.ReadCloser
	job      windows.Handle
	once     sync.Once
	finished chan struct{}
}

func (p *aiProcess) closeJob()     { p.once.Do(func() { windows.CloseHandle(p.job) }) }
func (p *aiProcess) finish() error { p.closeJob(); err := p.cmd.Wait(); close(p.finished); return err }
func (t Translator) start(ctx context.Context) (*aiProcess, error) {
	if !filepath.IsAbs(t.Executable) || !filepath.IsAbs(t.DataDirectory) {
		return nil, errors.New("AI 运行路径无效")
	}
	for _, dir := range []string{"home", "config", "cache", "home/AppData/Local", "home/AppData/Roaming"} {
		if err := os.MkdirAll(filepath.Join(t.DataDirectory, dir), 0700); err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, t.Executable)
	cmd.Env = isolatedEnvironment(t.DataDirectory)
	usagePath := t.UsagePath
	if usagePath == "" {
		usagePath = filepath.Join(t.DataDirectory, "config", "magpie", "usage.jsonl")
	}
	cmd.Env = append(cmd.Env, "WINONE_AI_USAGE_PATH="+usagePath)
	cmd.Dir = filepath.Dir(t.Executable)
	return startOwnedProcess(ctx, cmd)
}

// Both magpie and local OCR are short-lived workers. Assign their job before
// sending any input, so closing a result window also stops runtime helpers.
func startOwnedProcess(ctx context.Context, cmd *exec.Cmd) (*aiProcess, error) {
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
	hideConsole(cmd)
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
		windows.CloseHandle(job)
		return nil, err
	}
	p := &aiProcess{cmd: cmd, input: input, output: output, job: job, finished: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			p.closeJob()
		case <-p.finished:
		}
	}()
	return p, nil
}

func (t Translator) invoke(ctx context.Context, input any) (json.RawMessage, error) {
	return t.invokeBounded(ctx, input, 1024*1024)
}

// A local CSV export may exceed the normal message bound. Its caller keeps the
// payload in the Host and returns only a file receipt to the desktop.
func (t Translator) invokeBounded(ctx context.Context, input any, maximum int64) (json.RawMessage, error) {
	input = t.withRuntimeSettings(input)
	process, err := t.start(ctx)
	if err != nil {
		return nil, err
	}
	if err = json.NewEncoder(process.input).Encode(input); err != nil {
		process.closeJob()
		process.finish()
		return nil, err
	}
	process.input.Close()
	output, readErr := io.ReadAll(io.LimitReader(process.output, maximum+1))
	if readErr != nil || int64(len(output)) > maximum {
		process.closeJob()
		process.finish()
		return nil, errors.New("AI 响应无效或过大")
	}
	err = process.finish()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return json.RawMessage(output), err
}

func (t Translator) Accounts(ctx context.Context, operation, agent string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output, err := t.invoke(ctx, map[string]string{"operation": operation, "agent": agent, "agentBackend": t.AccountBackend})
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, errors.New("账号读取超时，请检查网络后重试")
	}
	if ctx.Err() != nil {
		return nil, errors.New("账号读取已取消")
	}
	var result struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(output, &result) != nil {
		return nil, errors.New("账号服务无法启动，请修复安装")
	}
	if err != nil || result.Error != "" {
		return nil, errors.New("账号暂不可用，请检查登录状态或客户端安装")
	}
	return output, nil
}

func (t Translator) SignIn(ctx context.Context, agent string, controls <-chan string, progress func(json.RawMessage)) error {
	return t.SignInMethod(ctx, agent, nil, controls, progress)
}

// A selected method is an index from this provider's own plugin, never a
// package name or executable supplied by the settings window.
func (t Translator) SignInMethod(ctx context.Context, agent string, method *int, controls <-chan string, progress func(json.RawMessage)) error {
	process, err := t.start(ctx)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(process.input)
	if err = encoder.Encode(map[string]any{"operation": "accounts.signin", "agent": agent, "agentBackend": t.AccountBackend, "method": method}); err != nil {
		process.closeJob()
		process.finish()
		return err
	}
	writingDone := make(chan struct{})
	go func() {
		defer close(writingDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-process.finished:
				return
			case callback, ok := <-controls:
				if !ok {
					return
				}
				if encoder.Encode(map[string]string{"callback": callback}) != nil {
					return
				}
			}
		}
	}()
	scanner := bufio.NewScanner(process.output)
	scanner.Buffer(make([]byte, 4096), 256*1024)
	for scanner.Scan() {
		raw := append(json.RawMessage(nil), scanner.Bytes()...)
		if !json.Valid(raw) {
			process.closeJob()
			break
		}
		progress(raw)
	}
	process.input.Close()
	err = process.finish()
	<-writingDone
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if scanner.Err() != nil {
		return scanner.Err()
	}
	return err
}

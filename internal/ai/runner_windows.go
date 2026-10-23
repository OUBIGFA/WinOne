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
	cmd            *exec.Cmd
	input          io.WriteCloser
	output         io.ReadCloser
	job            windows.Handle
	once           sync.Once
	finished       chan struct{}
	pluginLifetime context.Context
	loginComplete  bool
}

func (p *aiProcess) closeJob() { p.once.Do(func() { windows.CloseHandle(p.job) }) }
func (p *aiProcess) finish() error {
	// Magpie closes its plugin pipe on normal exit. Let plugins release their
	// helpers themselves; their detached login browser belongs to the Host,
	// not this one RPC. Errors/cancellation retain the kill-on-close policy.
	if p.pluginLifetime == nil {
		p.closeJob()
	}
	err := p.cmd.Wait()
	if err == nil && p.loginComplete {
		err = closePluginLoginBrowsers(p.job)
	}
	p.once.Do(func() {
		if err == nil && p.pluginLifetime != nil && p.pluginLifetime.Err() == nil {
			retainPluginJob(p.pluginLifetime, p.job)
		} else {
			windows.CloseHandle(p.job)
		}
	})
	close(p.finished)
	return err
}

// A retained job consumes no worker process. Release its handle as soon as
// the plugin's windows close, and kill remaining descendants on Host exit.
// Win32 JOBOBJECT_BASIC_ACCOUNTING_INFORMATION (not exposed by x/sys).
// LARGE_INTEGER fields are 64-bit, followed by four DWORD counters.
type jobBasicAccounting struct {
	TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
	TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
}

func retainPluginJob(ctx context.Context, job windows.Handle) {
	go func() {
		defer windows.CloseHandle(job)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			var info jobBasicAccounting
			err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)
			if err != nil || info.ActiveProcesses == 0 {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
func (t Translator) start(ctx context.Context) (*aiProcess, error) {
	return t.startMode(ctx, false)
}
func (t Translator) startMode(ctx context.Context, signingIn bool) (*aiProcess, error) {
	if !filepath.IsAbs(t.Executable) || !filepath.IsAbs(t.DataDirectory) {
		return nil, errors.New("AI 运行路径无效")
	}
	for _, dir := range []string{"home", "config", "cache", "home/AppData/Local", "home/AppData/Roaming"} {
		if err := os.MkdirAll(filepath.Join(t.DataDirectory, dir), 0700); err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, t.Executable)
	releaseToken, err := limitWorkerPrivilege(cmd)
	if err != nil {
		return nil, err
	}
	defer releaseToken()
	cmd.Env = isolatedEnvironment(t.DataDirectory)
	if t.AccountBackend == "plugin" && !signingIn {
		cmd.Env = append(cmd.Env, "WINONE_AI_BACKGROUND_DESKTOP=1")
	}
	// Plugins may create native Windows AppContainers. Their OS identity must
	// match the logged-in user; only the plugin child receives this profile.
	cmd.Env = append(cmd.Env, "WINONE_AI_OS_USERPROFILE="+os.Getenv("USERPROFILE"))
	if t.PrivacyService != nil && !t.TransportOnly {
		address, key, err := t.PrivacyService(ctx)
		if err != nil {
			return nil, err
		}
		if address != "" {
			cmd.Env = append(cmd.Env, "WINONE_AI_PRIVACY_URL="+address, "WINONE_AI_PRIVACY_KEY="+key)
		}
	}
	// Share user-installed plugin code, while isolatedEnvironment keeps auth
	// and other account files private to the selected provider profile.
	pluginRoot := t.RootDataDirectory
	if pluginRoot == "" {
		pluginRoot = t.DataDirectory
	}
	if err := preparePluginCodeDirectory(filepath.Join(pluginRoot, "third-party", "plugins")); err != nil {
		return nil, err
	}
	cmd.Env = append(cmd.Env, "WINONE_AI_PLUGINS_HOME="+filepath.Join(pluginRoot, "third-party"))
	if t.TransportOnly {
		cmd.Env = append(cmd.Env, "WINONE_AI_TRANSPORT_ONLY=1")
	}
	usagePath := t.UsagePath
	if usagePath == "" {
		usagePath = filepath.Join(t.DataDirectory, "config", "magpie", "usage.jsonl")
	}
	cmd.Env = append(cmd.Env, "WINONE_AI_USAGE_PATH="+usagePath)
	cmd.Dir = filepath.Dir(t.Executable)
	process, err := startOwnedProcess(ctx, cmd)
	if err == nil && t.AccountBackend == "plugin" {
		process.pluginLifetime = t.PluginLifetime
	}
	return process, err
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
	process, err := t.startMode(ctx, true)
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
		var event struct{ SignIn struct{ State string } }
		if json.Unmarshal(raw, &event) == nil && event.SignIn.State == "done" {
			process.loginComplete = true
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

//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"net/url"
	"strings"
	"sync"
	"time"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/host"
)

type aiSignInState struct {
	Agent         string `json:"agent"`
	URL           string `json:"url"`
	Code          string `json:"code,omitempty"`
	State         string `json:"state"`
	User          string `json:"user,omitempty"`
	Error         string `json:"error,omitempty"`
	PasteCallback bool   `json:"pasteCallback,omitempty"`
	PasteCode     bool   `json:"pasteCode,omitempty"`
	PasteKey      bool   `json:"pasteKey,omitempty"`
	KeysURL       string `json:"keysURL,omitempty"`
	Instructions  string `json:"instructions,omitempty"`
	// A plugin's CLI can open OAuth itself; preserve that fact across the Host
	// boundary so the account page does not open a duplicate browser tab.
	Opened bool            `json:"opened,omitempty"`
	Prompt *aiSignInPrompt `json:"prompt,omitempty"`
}
type aiSignInPrompt struct {
	Type        string `json:"type"`
	Key         string `json:"key"`
	Message     string `json:"message"`
	Placeholder string `json:"placeholder,omitempty"`
	Options     []struct {
		Label string `json:"label"`
		Value string `json:"value"`
		Hint  string `json:"hint,omitempty"`
	} `json:"options,omitempty"`
}
type aiSignInJob struct {
	ID           string        `json:"id"`
	Status       aiSignInState `json:"status"`
	InputError   string        `json:"inputError,omitempty"`
	InputPending bool          `json:"inputPending,omitempty"`
	owner        uint32
	cancel       context.CancelFunc
	input        chan string
	terminal     *aiSignInState
}

type aiSignInProgress struct {
	SignIn     *aiSignInState `json:"signin"`
	InputError string         `json:"inputError"`
	Error      string         `json:"error"`
}

// A submitted callback stays busy until it fails or the authorization moves
// to a new step. An unrelated waiting-state update is not an acknowledgement.
func (job *aiSignInJob) observe(event aiSignInProgress) {
	if event.SignIn != nil {
		if event.SignIn.State == "input" || event.SignIn.State != job.Status.State {
			job.InputPending = false
		}
		job.Status = *event.SignIn
		if event.SignIn.State == "done" {
			job.terminal = event.SignIn
			job.Status.State = "finishing"
		}
	}
	if event.InputError != "" {
		job.InputError = event.InputError
		job.InputPending = false
	}
	if event.Error != "" {
		job.Status.State = "failed"
		job.Status.Error = event.Error
		job.InputPending = false
	}
}

func validSignInInput(status aiSignInState, value string) bool {
	if len(value) > 16384 {
		return false
	}
	if status.State == "input" && status.Prompt != nil {
		switch status.Prompt.Type {
		case "text":
			return true
		case "password":
			return strings.TrimSpace(value) != ""
		case "select":
			for _, option := range status.Prompt.Options {
				if option.Value == value {
					return true
				}
			}
		}
		return false
	}
	return status.State == "waiting" && (status.PasteCallback || status.PasteCode || status.PasteKey) && strings.TrimSpace(value) != ""
}

func (s *aiService) handleAccounts(r host.Request) host.Response {
	result := func(value any) host.Response {
		data, _ := json.Marshal(value)
		return host.Response{ID: r.ID, Result: data}
	}
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai_account", Message: err.Error()}}
	}
	var params struct {
		Agent    string           `json:"agent"`
		ID       string           `json:"id"`
		Callback string           `json:"callback"`
		Keys     bool             `json:"keys"`
		Provider string           `json:"provider,omitempty"`
		Method   *int             `json:"method,omitempty"`
		Draft    *ai.AccountDraft `json:"draft,omitempty"`
	}
	if err := configjson.DecodeObject(r.Params, &params); err != nil {
		return fail(err)
	}
	if params.Method != nil && (*params.Method < 0 || *params.Method > 127) {
		return fail(errors.New("登录方式无效"))
	}
	runtime, err := s.magpieTranslator()
	if err != nil {
		return fail(err)
	}
	gate := s.accountGate
	var balanceProvider *ai.Provider
	var balanceKeys ai.KeyPlan
	if params.Draft != nil && params.Provider == "" {
		return fail(errors.New("账号连接标识无效"))
	}
	if (r.Method == "accounts.quota" || r.Method == "accounts.checkin") && (params.Provider == "" || params.Draft != nil) {
		return fail(errors.New("请先保存需要读取额度的账号"))
	}
	if params.Provider != "" && (r.Method == "accounts.list" || r.Method == "accounts.models" || r.Method == "accounts.quota" || r.Method == "accounts.checkin" || r.Method == "accounts.signin.start") {
		if r.Method == "accounts.quota" {
			p, plan, err := s.store.ProviderKeysForModels(params.Provider)
			if err != nil {
				return fail(err)
			}
			if p.AuthMode == "api-key" {
				if params.Agent != "" {
					return fail(errors.New("余额供应商与账号不匹配"))
				}
				balanceProvider, balanceKeys = &p, plan
			}
		}
		if balanceProvider == nil {
			p, err := s.accountProvider(params.Provider, params.Agent, params.Draft)
			if err != nil {
				return fail(err)
			}
			runtime = runtime.ForProvider(p.ID)
			runtime.AccountBackend = p.AgentBackend
			gate = s.accountGateFor(p.ID)
		} else {
			gate = s.accountGateFor(balanceProvider.ID)
		}
	}
	switch r.Method {
	case "accounts.catalog", "accounts.list", "accounts.models", "accounts.quota", "accounts.checkin":
		// Even a quota read can refresh credentials or write quota history.
		// Keep one owner per profile, but let a short read wait for another
		// operation instead of treating ordinary contention as a broken login.
		readCtx, cancel := context.WithTimeout(s.ctx, 25*time.Second)
		defer cancel()
		if readCtx.Err() != nil {
			return fail(errors.New("账号读取已取消"))
		}
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		case <-readCtx.Done():
			if errors.Is(readCtx.Err(), context.DeadlineExceeded) {
				return fail(errors.New("等待账号操作完成超时，请完成当前请求后刷新"))
			}
			return fail(errors.New("账号读取已取消"))
		}
		var output json.RawMessage
		if balanceProvider != nil {
			output, err = runtime.KeyBalances(readCtx, *balanceProvider, balanceKeys)
		} else {
			output, err = runtime.Accounts(readCtx, r.Method, params.Agent)
		}
		if err != nil {
			return fail(err)
		}
		output, err = ai.WithAccountIcons(r.Method, params.Agent, output)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: r.ID, Result: output}
	case "accounts.signin.start":
		if params.Agent == "" || len(params.Agent) > 80 {
			return fail(errors.New("请选择智能体"))
		}
		select {
		case s.accountGate <- struct{}{}:
		default:
			return fail(errors.New("请先完成当前账号操作"))
		}
		if gate != s.accountGate {
			select {
			case gate <- struct{}{}:
			default:
				<-s.accountGate
				return fail(errors.New("所选账号正在使用，请稍后登录"))
			}
		}
		releaseGates := func() {
			<-s.accountGate
			if gate != s.accountGate {
				<-gate
			}
		}
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			releaseGates()
			return fail(err)
		}
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
		job := &aiSignInJob{ID: hex.EncodeToString(id[:]), Status: aiSignInState{Agent: params.Agent, State: "starting"}, owner: r.PeerPID, cancel: cancel, input: make(chan string, 1)}
		s.mu.Lock()
		s.signin = job
		initial := *job
		s.mu.Unlock()
		go func() {
			defer cancel()
			var released sync.Once
			release := func() { released.Do(releaseGates) }
			defer release()
			if job.owner != 0 {
				owner, err := windows.OpenProcess(windows.SYNCHRONIZE, false, job.owner)
				if err != nil {
					cancel()
				} else {
					go func() {
						defer windows.CloseHandle(owner)
						for ctx.Err() == nil {
							state, err := windows.WaitForSingleObject(owner, 200)
							if err != nil || state == windows.WAIT_OBJECT_0 {
								cancel()
								return
							}
						}
					}()
				}
			}
			err := runtime.SignInMethod(ctx, params.Agent, params.Method, job.input, func(raw json.RawMessage) {
				var event aiSignInProgress
				if json.Unmarshal(raw, &event) != nil {
					return
				}
				s.mu.Lock()
				defer s.mu.Unlock()
				if s.signin != job || ctx.Err() != nil {
					return
				}
				job.observe(event)
			})
			release()
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.signin != job {
				return
			}
			job.InputPending = false
			if job.terminal != nil {
				job.Status = *job.terminal
			} else if ctx.Err() != nil {
				job.Status.State = "canceled"
				job.InputPending = false
			} else if err != nil || job.Status.State != "done" && job.Status.State != "failed" && job.Status.State != "canceled" {
				job.Status.State = "failed"
				if job.Status.Error == "" {
					job.Status.Error = "登录未完成，请重试"
				}
			}
		}()
		return result(initial)
	case "accounts.signin.get", "accounts.signin.cancel", "accounts.signin.submit", "accounts.signin.open":
		s.mu.Lock()
		defer s.mu.Unlock()
		job := s.signin
		if job == nil || job.ID != params.ID || job.owner != r.PeerPID {
			return fail(errors.New("登录任务不存在"))
		}
		switch r.Method {
		case "accounts.signin.cancel":
			if job.terminal == nil && job.Status.State != "done" {
				job.cancel()
				job.Status.State = "canceled"
			}
		case "accounts.signin.submit":
			if !validSignInInput(job.Status, params.Callback) {
				return fail(errors.New("当前登录步骤不接受此输入，请按页面提示操作"))
			}
			if job.InputPending {
				return fail(errors.New("正在处理上一次输入"))
			}
			select {
			case job.input <- params.Callback:
				job.InputError = ""
				job.InputPending = true
			default:
				return fail(errors.New("正在处理上一次输入"))
			}
		case "accounts.signin.open":
			addressValue := job.Status.URL
			if params.Keys || addressValue == "" {
				addressValue = job.Status.KeysURL
			}
			target, err := url.Parse(addressValue)
			if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil {
				return fail(errors.New("登录地址尚未就绪"))
			}
			verb, _ := windows.UTF16PtrFromString("open")
			address, _ := windows.UTF16PtrFromString(target.String())
			if err = windows.ShellExecute(0, verb, address, nil, nil, windows.SW_SHOWNORMAL); err != nil {
				return fail(err)
			}
		}
		return result(job)
	default:
		return fail(errors.New("不支持的账号操作"))
	}
}

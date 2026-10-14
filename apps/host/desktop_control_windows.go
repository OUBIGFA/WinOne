//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"path/filepath"
	"time"
	"winone/internal/adminstartup"
	"winone/internal/configjson"
	"winone/internal/host"
	"winone/internal/logintask"
	"winone/internal/maintenance"
	"winone/internal/processidentity"
	"winone/internal/protocol"
)

type desktopControlRequest struct {
	Version       int    `json:"version"`
	Operation     string `json:"operation"`
	Enabled       *bool  `json:"enabled,omitempty"`
	Pipe          string `json:"pipe"`
	DataDirectory string `json:"dataDirectory"`
	OwnerPID      uint32 `json:"ownerPID"`
}

func (r desktopControlRequest) validate() error {
	if r.Version != 1 || r.OwnerPID == 0 || (r.Operation != "preference" && r.Operation != "restart") ||
		(r.Operation == "preference") != (r.Enabled != nil) {
		return errors.New("权限操作参数无效")
	}
	return adminstartup.ValidateDefaultProfile(r.Pipe, r.DataDirectory)
}

type desktopControlResult struct {
	Closing  bool            `json:"closing"`
	Token    string          `json:"token,omitempty"`
	Settings json.RawMessage `json:"settings,omitempty"`
}
type desktopControlPorts struct {
	guard     func() error
	call      func(string, any) (json.RawMessage, error)
	configure func(bool) error
	launch    func() error
	elevated  bool
}

// Host calls are relayed by the Rust process so restart ownership remains bound
// to the actual settings process, not this short-lived consent helper.
func performDesktopControl(ctx context.Context, request desktopControlRequest, p desktopControlPorts) (desktopControlResult, error) {
	fail := func(err error) (desktopControlResult, error) { return desktopControlResult{}, err }
	if request.Operation != "preference" && request.Operation != "restart" || request.Operation == "preference" && request.Enabled == nil {
		return fail(errors.New("权限操作无效"))
	}
	if err := p.guard(); err != nil {
		return fail(err)
	}
	snapshot := func() (json.RawMessage, bool, error) {
		raw, err := p.call("snapshot", nil)
		if err != nil {
			return nil, false, err
		}
		var value struct {
			Settings      json.RawMessage `json:"settings"`
			Administrator *bool           `json:"administrator"`
		}
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, false, err
		}
		var settings map[string]json.RawMessage
		if value.Administrator == nil || json.Unmarshal(value.Settings, &settings) != nil || settings == nil {
			return nil, false, errors.New("无法确认当前权限状态")
		}
		return value.Settings, *value.Administrator, nil
	}
	settings, administrator, err := snapshot()
	if err != nil {
		return fail(err)
	}
	if request.Operation == "restart" && administrator && p.elevated {
		return desktopControlResult{Settings: settings}, nil
	}
	enabled := request.Operation == "restart" || *request.Enabled
	if err = p.configure(enabled); err != nil {
		return fail(err)
	}
	if request.Operation == "preference" {
		if err = p.guard(); err != nil {
			return fail(err)
		}
		// Consent may outlive unrelated settings edits. Preserve the entire
		// latest JSON object, including fields unknown to this UI version.
		settings, _, err = snapshot()
		if err != nil {
			return fail(err)
		}
		var value map[string]json.RawMessage
		if err = json.Unmarshal(settings, &value); err != nil {
			return fail(err)
		}
		startup := map[string]json.RawMessage{"startMinimized": json.RawMessage("true")}
		if raw := value["startup"]; len(raw) > 0 && string(raw) != "null" {
			if err = json.Unmarshal(raw, &startup); err != nil {
				return fail(err)
			}
		}
		if startup == nil {
			startup = map[string]json.RawMessage{"startMinimized": json.RawMessage("true")}
		}
		startup["preferAdministrator"], _ = json.Marshal(enabled)
		value["startup"], err = json.Marshal(startup)
		if err != nil {
			return fail(err)
		}
		saved, err := p.call("settings.save", value)
		if err != nil {
			return fail(err)
		}
		return desktopControlResult{Settings: saved}, nil
	}
	restartCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	result := desktopControlResult{}
	err = adminstartup.Restart(restartCtx, adminstartup.RestartPorts{
		Guard: p.guard, Launch: p.launch,
		Call: func(method string, params any) (adminstartup.HandoffSnapshot, error) {
			raw, err := p.call("application."+method, params)
			if err != nil {
				return adminstartup.HandoffSnapshot{}, err
			}
			var status adminstartup.HandoffSnapshot
			err = json.Unmarshal(raw, &status)
			return status, err
		},
		Close: func(token string) error { result.Closing = true; result.Token = token; return nil },
	})
	if err != nil {
		return fail(err)
	}
	return result, nil
}

type desktopControlMessage struct {
	Version  int            `json:"version"`
	Kind     string         `json:"kind"`
	Sequence uint64         `json:"sequence"`
	Method   string         `json:"method,omitempty"`
	Params   any            `json:"params,omitempty"`
	Result   any            `json:"result,omitempty"`
	Error    *host.RPCError `json:"error,omitempty"`
}
type desktopControlReply struct {
	Version  int             `json:"version"`
	Sequence uint64          `json:"sequence"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    *host.RPCError  `json:"error,omitempty"`
}
type desktopControlRemote struct {
	input    io.Reader
	output   io.Writer
	sequence uint64
}

func (r *desktopControlRemote) write(message desktopControlMessage) error {
	message.Version = 1
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return protocol.WriteFrame(r.output, data)
}
func (r *desktopControlRemote) exchange(kind, method string, params any) (json.RawMessage, error) {
	r.sequence++
	if err := r.write(desktopControlMessage{Kind: kind, Sequence: r.sequence, Method: method, Params: params}); err != nil {
		return nil, err
	}
	raw, err := protocol.ReadFrame(r.input)
	if err != nil {
		return nil, err
	}
	var reply desktopControlReply
	if err = configjson.DecodeObject(raw, &reply); err != nil {
		return nil, err
	}
	if reply.Version != 1 || reply.Sequence != r.sequence || (reply.Error == nil) == (reply.Result == nil) {
		return nil, errors.New("权限操作答复无效")
	}
	if reply.Error != nil {
		return nil, errors.New(reply.Error.Message)
	}
	return reply.Result, nil
}

func desktopControl(args []string, executable string, input io.Reader, output io.Writer) (bool, error) {
	if len(args) == 0 || args[0] != "--desktop-control" {
		return false, nil
	}
	if len(args) != 1 {
		return true, errors.New("权限辅助参数无效")
	}
	remote := desktopControlRemote{input: input, output: output}
	result, err := runDesktopControl(executable, &remote)
	message := desktopControlMessage{Kind: "done", Sequence: remote.sequence + 1}
	if err != nil {
		code := "permission"
		if errors.Is(err, windows.ERROR_CANCELLED) || errors.Is(err, context.Canceled) {
			code = "cancelled"
		}
		message.Error = &host.RPCError{Code: code, Message: err.Error()}
	} else {
		message.Result = result
	}
	return true, remote.write(message)
}
func runDesktopControl(executable string, remote *desktopControlRemote) (desktopControlResult, error) {
	raw, err := protocol.ReadFrame(remote.input)
	if err != nil {
		return desktopControlResult{}, err
	}
	if len(raw) > 16384 {
		return desktopControlResult{}, errors.New("权限操作信息过长")
	}
	var request desktopControlRequest
	if err = configjson.DecodeObject(raw, &request); err != nil {
		return desktopControlResult{}, err
	}
	if err = request.validate(); err != nil {
		return desktopControlResult{}, err
	}
	desktop := filepath.Join(filepath.Dir(executable), "WinOne.exe")
	owner, err := processidentity.Open(request.OwnerPID, desktop)
	if err != nil {
		return desktopControlResult{}, err
	}
	defer owner.Close()
	lease, err := maintenance.AcquireRuntimeForInstallation(request.DataDirectory, filepath.Dir(executable))
	if err != nil {
		return desktopControlResult{}, err
	}
	defer lease.Close()
	ctx, cancel := context.WithCancel(context.Background())
	ended := make(chan struct{})
	go func() { _ = owner.Wait(ctx); cancel(); close(ended) }()
	defer func() { cancel(); <-ended }()
	guard := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := remote.exchange("guard", "", nil)
		if err != nil {
			return err
		}
		var clean bool
		if err = json.Unmarshal(raw, &clean); err != nil || !clean {
			return errors.New("请先完成当前编辑并保存设置")
		}
		return nil
	}
	return performDesktopControl(ctx, request, desktopControlPorts{
		guard: guard, elevated: windows.GetCurrentProcessToken().IsElevated(),
		call: func(method string, params any) (json.RawMessage, error) {
			return remote.exchange("call", method, params)
		},
		configure: func(enabled bool) error {
			return adminstartup.ConfigureCurrent(enabled, request.Pipe, request.DataDirectory, desktop)
		},
		launch: func() error {
			launcher, err := logintask.NewAdministratorLauncher(desktop)
			if err != nil {
				return err
			}
			_, err = launcher.Run(logintask.InteractiveLaunch)
			return err
		},
	})
}

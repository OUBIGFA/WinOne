//go:build windows

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
	"winone/internal/host"
	"winone/internal/processidentity"
	"winone/internal/transport"
)

var errAdministratorHandoffNotPrepared = errors.New("重启尚未准备完成")

type administratorHandoffSnapshot struct {
	Token              string `json:"token"`
	State              string `json:"state"`
	Error              string `json:"error"`
	HelperPID          uint32 `json:"helperPid"`
	HelperAcknowledged bool   `json:"helperAcknowledged"`
}
type administratorHandoff struct {
	pid       uint32
	token     string
	committed bool
}

func (h *administratorHandoff) observe(call func(string, any) (administratorHandoffSnapshot, error)) error {
	method := "restart.status"
	var params any = map[string]string{"token": h.token}
	if h.token == "" {
		method = "restart.bootstrap"
		params = nil
	}
	state, err := call(method, params)
	if err != nil {
		if h.token == "" && errors.Is(err, errAdministratorHandoffNotPrepared) {
			return nil
		}
		return err
	}
	if len(state.Token) != 32 || state.HelperPID != h.pid || h.pid == 0 {
		return errors.New("管理员交接标识无效")
	}
	if _, err = hex.DecodeString(state.Token); err != nil {
		return errors.New("管理员交接标识无效")
	}
	if h.token != "" && state.Token != h.token {
		return errors.New("管理员交接已变化")
	}
	h.token = state.Token
	switch state.State {
	case "prepared":
		return nil
	case "committed":
		// Remember the commit before acknowledging: the old Host may exit as soon
		// as it receives the acknowledgement and miss writing the reply.
		h.committed = true
		ack, err := call("restart.acknowledge", map[string]string{"token": h.token})
		if err != nil {
			return err
		}
		if ack.Token != h.token || ack.HelperPID != h.pid || ack.State != "committed" || !ack.HelperAcknowledged {
			return errors.New("管理员交接确认不完整")
		}
		return nil
	default:
		if state.Error != "" {
			return errors.New(state.Error)
		}
		return errors.New("重启已取消")
	}
}
func (h *administratorHandoff) ready() error {
	if h.token != "" && !h.committed {
		return errors.New("未收到原窗口的重启确认")
	}
	return nil
}

func callAdministratorHandoff(ctx context.Context, pipe, expectedHost, method string, params any) (administratorHandoffSnapshot, error) {
	var state administratorHandoffSnapshot
	payload, err := json.Marshal(params)
	if err != nil {
		return state, err
	}
	var peer *processidentity.Process
	defer func() {
		if peer != nil {
			peer.Close()
		}
	}()
	response, err := transport.CallWithVerifiedServer(ctx, pipe, host.Request{Version: 1, ID: "administrator-handoff", Method: "application." + method, Params: payload}, time.Second, func(pid uint32) error {
		var err error
		peer, err = processidentity.Open(pid, expectedHost)
		return err
	})
	if err != nil {
		return state, err
	}
	if response.Error != nil {
		if response.Error.Code == "restart_not_prepared" {
			return state, errAdministratorHandoffNotPrepared
		}
		return state, errors.New(response.Error.Message)
	}
	err = json.Unmarshal(response.Result, &state)
	return state, err
}

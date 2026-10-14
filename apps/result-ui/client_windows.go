package resultui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"strconv"
	"strings"
	"time"
	"winone/internal/host"
	"winone/internal/transport"
)

type Client struct {
	Pipe    string
	HostPID uint32
	Parent  windows.Handle
}

func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	if state, err := windows.WaitForSingleObject(c.Parent, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return errors.New("WinOne 已退出")
	}
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	r, err := transport.CallWithVerifiedServer(ctx, c.Pipe, host.Request{Version: 1, ID: strconv.FormatInt(time.Now().UnixNano(), 10), Method: method, Params: data}, 5*time.Second, func(pid uint32) error {
		if pid != c.HostPID {
			return errors.New("WinOne 服务身份不匹配")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if r.Error != nil {
		return errors.New(r.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

type job struct {
	ID, State, Text, Reasoning, Error, Model, ModelName, Provider, ProviderName, ProviderIcon string
	TextOffset, ReasoningOffset                                                               int
}
type Outcome struct{ Text, Reasoning, Model, ModelName, Provider, ProviderName, ProviderIcon, State string }

func (o Outcome) displayModel() string {
	if o.ModelName != "" {
		return o.ModelName
	}
	return o.Model
}

func (c *Client) Work(ctx context.Context, method string, params any, started func(), progress func(Outcome)) (Outcome, error) {
	// Starting a local job is bounded but is not cut short by the Stop button:
	// retain its returned ID so even a just-cancelled request can be released.
	startCtx, stop := context.WithTimeout(context.Background(), 6*time.Second)
	var current job
	err := c.Call(startCtx, method, params, &current)
	stop()
	if err != nil {
		return Outcome{}, err
	}
	if current.ID == "" {
		return Outcome{}, errors.New("AI 任务未返回标识")
	}
	defer func() {
		release, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		_ = c.Call(release, "ai.jobs.cancel", map[string]string{"id": current.ID}, nil)
	}()
	if ctx.Err() != nil {
		return Outcome{}, ctx.Err()
	}
	if started != nil {
		started()
	}
	var text, reasoning strings.Builder
	value := Outcome{Model: current.Model, ModelName: current.ModelName, Provider: current.Provider, ProviderName: current.ProviderName, ProviderIcon: current.ProviderIcon, State: "running"}
	if progress != nil {
		progress(value)
	}
	for {
		var next job
		if err = c.Call(ctx, "ai.jobs.get", map[string]any{"id": current.ID, "textOffset": text.Len(), "reasoningOffset": reasoning.Len()}, &next); err != nil {
			return value, err
		}
		if next.ID != current.ID || next.TextOffset != text.Len() || next.ReasoningOffset != reasoning.Len() || text.Len()+reasoning.Len()+len(next.Text)+len(next.Reasoning) > 1<<20 {
			return value, errors.New("AI 增量响应无效")
		}
		text.WriteString(next.Text)
		reasoning.WriteString(next.Reasoning)
		value.Text, value.Reasoning, value.State = text.String(), reasoning.String(), next.State
		if progress != nil && (next.Text != "" || next.Reasoning != "") {
			progress(value)
		}
		current.State, current.Error = next.State, next.Error
		switch current.State {
		case "completed":
			return value, nil
		case "failed":
			return value, errors.New(current.Error)
		case "cancelled":
			return value, context.Canceled
		case "running":
		default:
			return value, fmt.Errorf("AI 任务状态无效：%s", current.State)
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return value, ctx.Err()
		case <-timer.C:
		}
	}
}

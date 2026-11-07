//go:build windows

package transport

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"winone/internal/host"
)

// CallCompleting submits a mutation once, then follows its read-only job handle.
// A polling error never causes a retry of the original save.
func CallCompleting(ctx context.Context, pipe string, request host.Request, timeout time.Duration) (host.Response, error) {
	response, err := CallWithTimeout(ctx, pipe, request, timeout)
	if err != nil || response.Error != nil {
		return response, err
	}
	var ticket struct {
		ID     string `json:"configurationJob"`
		WaitMs int    `json:"configurationJobWaitMs"`
	}
	if json.Unmarshal(response.Result, &ticket) != nil || ticket.ID == "" {
		return response, nil
	}
	waitMs := min(max(ticket.WaitMs, 0), 1000)
	if timeout > 0 {
		waitMs = min(waitMs, max(1, int(timeout/time.Millisecond/2)))
	}
	params, _ := json.Marshal(map[string]any{"id": ticket.ID, "waitMs": waitMs})
	poll := host.Request{Version: 1, ID: request.ID + "-configuration", Method: "configuration.jobs.get", Params: params}
	for {
		response, err = CallWithTimeout(ctx, pipe, poll, timeout)
		if err != nil {
			return host.Response{}, err
		}
		if response.Error != nil {
			return response, nil
		}
		var job host.ConfigurationJob
		if err = json.Unmarshal(response.Result, &job); err != nil {
			return host.Response{}, err
		}
		if job.ID != ticket.ID {
			return host.Response{}, errors.New("配置任务标识不匹配")
		}
		switch job.State {
		case "completed", "failed":
			if job.Response == nil || job.Response.ID != request.ID {
				return host.Response{}, errors.New("配置任务结果不完整")
			}
			if (job.State == "failed") != (job.Response.Error != nil) {
				return host.Response{}, errors.New("配置任务状态与结果不一致")
			}
			return *job.Response, nil
		case "running":
		default:
			return host.Response{}, errors.New("无法确认配置任务状态")
		}
		// New Hosts wake the read at actual completion. Older Hosts do not
		// advertise wait support and keep the original bounded polling cadence.
		if waitMs > 0 {
			continue
		}
		timer := time.NewTimer(150 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return host.Response{}, ctx.Err()
		case <-timer.C:
		}
	}
}

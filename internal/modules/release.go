package modules

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"winone/internal/host"
)

// Both disable and uninstall share the worker's asynchronous restoration
// protocol. Only uninstall requests removal of the product login task.
func ReleaseWinTray(ctx context.Context, worker interface {
	Resume()
	Handle(host.Request) host.Response
	Close() error
}, uninstall bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	method := "deactivate"
	if uninstall {
		method = "uninstall.prepare"
	}
	worker.Resume()
	response := worker.Handle(host.Request{Version: 1, ID: "module-deactivate", Method: method})
	var job struct {
		ID    string `json:"id"`
		State string `json:"state"`
		Error string `json:"error"`
	}
	for {
		if response.Error != nil {
			return errors.New(response.Error.Message)
		}
		job.ID, job.State, job.Error = "", "", ""
		if err := json.Unmarshal(response.Result, &job); err != nil {
			return err
		}
		if job.State == "completed" {
			return worker.Close()
		}
		if job.State != "running" {
			if job.Error != "" {
				return errors.New(job.Error)
			}
			return errors.New("组件返回了无效的恢复状态")
		}
		if job.ID == "" {
			return errors.New("组件未返回恢复任务标识")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		params, _ := json.Marshal(map[string]string{"id": job.ID})
		response = worker.Handle(host.Request{Version: 1, ID: "module-deactivate-status", Method: "jobs.get", Params: params})
	}
}

//go:build windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"winone/internal/configjson"
	"winone/internal/host"
)

type restartPreparationJob struct {
	Token              string `json:"token"`
	State              string `json:"state"`
	Error              string `json:"error,omitempty"`
	HelperPID          uint32 `json:"helperPid,omitempty"`
	HelperAcknowledged bool   `json:"helperAcknowledged,omitempty"`
}

var errRestartNotPrepared = errors.New("重启尚未准备完成")

// Preparation is bounded even if the requesting window disappears. The future
// consent handoff takes ownership only after the replacement helper is ready.
type restartPreparation struct {
	mu              sync.Mutex
	prepare         func() (func(), error)
	timeout         time.Duration
	current         restartPreparationJob
	cancelled       bool
	resume          func()
	timer           *time.Timer
	owner           uint32
	authorize       func(uint32) error
	authorizeHelper func(uint32) error
	watchExit       func(uint32, uint32) (func(func() bool) error, func(), error)
	exit            func()
	failed          func(error)
}

func (r *restartPreparation) latestError() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current.Error
}

func (r *restartPreparation) pending() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.current.State {
	case "preparing", "prepared", "cancelling", "committed":
		return true
	}
	return false
}

func (r *restartPreparation) commit(token string) (restartPreparationJob, error) {
	r.mu.Lock()
	if token == "" || token != r.current.Token {
		r.mu.Unlock()
		return restartPreparationJob{}, errRestartNotPrepared
	}
	if r.current.State == "committed" {
		job := r.current
		r.mu.Unlock()
		return job, nil
	}
	owner, helper := r.owner, r.current.HelperPID
	ready := r.current.State == "prepared" && helper != 0
	r.mu.Unlock()
	if !ready || r.watchExit == nil || r.exit == nil {
		return restartPreparationJob{}, errRestartNotPrepared
	}
	wait, closeHandles, err := r.watchExit(owner, helper)
	if err != nil {
		return restartPreparationJob{}, err
	}
	undo, err := r.take(token)
	if err != nil {
		closeHandles()
		return restartPreparationJob{}, err
	}
	go func() {
		defer closeHandles()
		err := wait(func() bool {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.current.Token == token && r.current.State == "committed" && r.current.HelperAcknowledged
		})
		if err != nil {
			undo()
			r.mu.Lock()
			if r.current.Token == token {
				r.current.Error = "重启未完成：" + err.Error()
			}
			r.mu.Unlock()
			if r.failed != nil {
				r.failed(err)
			}
			return
		}
		r.exit()
	}()
	return r.status(token)
}

func (r *restartPreparation) bindHelper(pid uint32) (restartPreparationJob, error) {
	if pid == 0 || r.authorizeHelper == nil {
		return restartPreparationJob{}, errors.New("无法确认管理员辅助进程")
	}
	if err := r.authorizeHelper(pid); err != nil {
		return restartPreparationJob{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if pid == r.owner {
		return restartPreparationJob{}, errors.New("替换进程不能是原窗口")
	}
	if r.current.State == "committed" && r.current.HelperPID == pid {
		// The first bind reply may be lost while the owner commits. Only that
		// already-verified helper may recover the same session after commit.
		return r.current, nil
	}
	if r.current.State != "prepared" {
		return restartPreparationJob{}, errRestartNotPrepared
	}
	if r.current.HelperPID != 0 && r.current.HelperPID != pid {
		return restartPreparationJob{}, errors.New("已有管理员辅助进程正在交接")
	}
	r.current.HelperPID = pid
	return r.current, nil
}
func (r *restartPreparation) acknowledgeHelper(pid uint32, token string) (restartPreparationJob, error) {
	if r.authorizeHelper == nil {
		return restartPreparationJob{}, errors.New("无法确认管理员辅助进程")
	}
	if err := r.authorizeHelper(pid); err != nil {
		return restartPreparationJob{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if token == "" || token != r.current.Token || pid == 0 || pid != r.current.HelperPID || r.current.State != "committed" {
		return restartPreparationJob{}, errRestartNotPrepared
	}
	r.current.HelperAcknowledged = true
	return r.current, nil
}

func (r *restartPreparation) begin() (restartPreparationJob, error) {
	return r.beginFor(0)
}
func (r *restartPreparation) beginFor(owner uint32) (restartPreparationJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.current.State {
	case "preparing", "prepared", "cancelling", "committed":
		return restartPreparationJob{}, errors.New("重启准备正在进行")
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return restartPreparationJob{}, err
	}
	job := restartPreparationJob{Token: hex.EncodeToString(token[:]), State: "preparing"}
	r.current = job
	r.owner = owner
	r.cancelled = false
	go r.run(job.Token)
	return job, nil
}
func (r *restartPreparation) run(token string) {
	resume, err := r.prepare()
	r.mu.Lock()
	if err != nil {
		r.current.State = "failed"
		r.current.Error = err.Error()
		r.mu.Unlock()
		return
	}
	if r.cancelled {
		r.current.State = "cancelling"
		r.mu.Unlock()
		resume()
		r.mu.Lock()
		r.current.State = "cancelled"
		r.mu.Unlock()
		return
	}
	r.resume = resume
	r.current.State = "prepared"
	r.timer = time.AfterFunc(r.timeout, func() { _, _ = r.cancel(token, "重启确认已超时，请重试") })
	r.mu.Unlock()
}
func (r *restartPreparation) status(token string) (restartPreparationJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if token == "" || token != r.current.Token {
		return restartPreparationJob{}, errors.New("重启准备已失效")
	}
	return r.current, nil
}
func (r *restartPreparation) cancel(token, reason string) (restartPreparationJob, error) {
	r.mu.Lock()
	if token == "" || token != r.current.Token {
		r.mu.Unlock()
		return restartPreparationJob{}, errors.New("重启准备已失效")
	}
	switch r.current.State {
	case "committed":
		r.mu.Unlock()
		return restartPreparationJob{}, errors.New("重启已开始")
	case "preparing":
		r.cancelled = true
		r.current.State = "cancelling"
	case "prepared":
		if r.timer != nil {
			r.timer.Stop()
		}
		resume := r.resume
		r.resume = nil
		r.current.State = "cancelling"
		r.mu.Unlock()
		resume()
		r.mu.Lock()
		r.current.State = "cancelled"
		r.current.Error = reason
	}
	result := r.current
	r.mu.Unlock()
	return result, nil
}

// take is deliberately not an RPC: only a validated consent/handoff handler may
// consume preparation and retain the shutdown lease until process exit.
func (r *restartPreparation) take(token string) (func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if token == "" || token != r.current.Token || r.current.State != "prepared" {
		return nil, errors.New("重启尚未准备完成")
	}
	if r.current.HelperPID == 0 || r.authorizeHelper == nil {
		return nil, errors.New("管理员辅助进程尚未就绪")
	}
	if err := r.authorizeHelper(r.current.HelperPID); err != nil {
		return nil, err
	}
	r.timer.Stop()
	r.current.State = "committed"
	resume := r.resume
	r.resume = nil
	var once sync.Once
	return func() {
		once.Do(func() {
			resume()
			r.mu.Lock()
			if r.current.Token == token {
				r.current.State = "cancelled"
			}
			r.mu.Unlock()
		})
	}, nil
}
func (r *restartPreparation) handle(request host.Request) host.Response {
	var result restartPreparationJob
	var err error
	// The Rust elevated launcher runs the shared handshake in WinOne.Host.exe.
	// Validate that role with the helper policy; it must never inherit the
	// desktop owner's prepare/commit/cancel capabilities.
	authorize := r.authorize
	if request.Method == "restart.bootstrap" {
		authorize = r.authorizeHelper
	} else if request.Method == "restart.status" || request.Method == "restart.acknowledge" {
		r.mu.Lock()
		helper := request.PeerPID != 0 && request.PeerPID == r.current.HelperPID
		r.mu.Unlock()
		if helper {
			authorize = r.authorizeHelper
		}
	}
	if request.PeerPID == 0 || authorize == nil {
		err = errors.New("无法确认重启请求来源")
	} else {
		err = authorize(request.PeerPID)
	}
	if err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "restart_identity", Message: err.Error()}}
	}
	if request.Method == "restart.bootstrap" {
		result, err = r.bindHelper(request.PeerPID)
	} else if request.Method == "restart.prepare" {
		result, err = r.beginFor(request.PeerPID)
	} else {
		r.mu.Lock()
		owned := r.owner == request.PeerPID || (r.current.HelperPID == request.PeerPID && (request.Method == "restart.status" || request.Method == "restart.acknowledge"))
		r.mu.Unlock()
		if !owned {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "restart_identity", Message: "重启请求属于另一个窗口"}}
		}
		var params struct {
			Token string `json:"token"`
		}
		if err = configjson.DecodeObject(request.Params, &params); err == nil {
			switch request.Method {
			case "restart.status":
				result, err = r.status(params.Token)
			case "restart.cancel":
				result, err = r.cancel(params.Token, "")
			case "restart.acknowledge":
				result, err = r.acknowledgeHelper(request.PeerPID, params.Token)
			case "restart.commit":
				result, err = r.commit(params.Token)
			default:
				err = errors.New("不支持的重启操作")
			}
		}
	}
	if err != nil {
		code := "restart"
		if errors.Is(err, errRestartNotPrepared) {
			code = "restart_not_prepared"
		}
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: code, Message: err.Error()}}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "restart", Message: err.Error()}}
	}
	return host.Response{ID: request.ID, Result: data}
}

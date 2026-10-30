//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"winone/internal/configjson"
	"winone/internal/directorylinks"
	"winone/internal/host"
)

type directoryMigrationJob struct {
	ID        string                           `json:"id"`
	RequestID string                           `json:"requestId"`
	State     string                           `json:"state"`
	Progress  directorylinks.MigrationProgress `json:"progress"`
	Plan      *directorylinks.MigrationPlan    `json:"plan,omitempty"`
	Record    *directorylinks.Record           `json:"record,omitempty"`
	Error     string                           `json:"error,omitempty"`
	CanCancel bool                             `json:"canCancel"`
}
type directoryMigrationJobs struct {
	mu      sync.Mutex
	current directoryMigrationJob
	owner   uint32
	cancel  context.CancelFunc
	done    chan struct{}
	closed  bool
	guard   func() (func(), bool)
	plans   map[string]struct {
		owner   uint32
		expires time.Time
	}
}

func (j *directoryMigrationJobs) Close() error {
	j.mu.Lock()
	j.closed = true
	stop, done := j.cancel, j.done
	j.mu.Unlock()
	if stop != nil {
		stop()
	}
	if done != nil {
		<-done
	}
	return nil
}
func (j *directoryMigrationJobs) start(owner uint32, requestID string, run func(context.Context, func(directorylinks.MigrationProgress)) (*directorylinks.MigrationPlan, *directorylinks.Record, error)) (directoryMigrationJob, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if requestID == "" || len(requestID) > 80 {
		return directoryMigrationJob{}, errors.New("目录任务请求标识无效")
	}
	if j.closed || j.current.State == "running" {
		return directoryMigrationJob{}, errors.New("目录任务正在运行或停止，请等待当前任务结束")
	}
	release := func() {}
	if j.guard != nil {
		var ok bool
		release, ok = j.guard()
		if !ok {
			return directoryMigrationJob{}, errors.New("配置正在保存，暂时不能开始目录任务")
		}
		if release == nil {
			release = func() {}
		}
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	j.owner = owner
	j.cancel = stop
	j.done = done
	j.current = directoryMigrationJob{ID: rand.Text(), RequestID: requestID, State: "running", CanCancel: true, Progress: directorylinks.MigrationProgress{Phase: "checking"}}
	initial := j.current
	go func() {
		defer close(done)
		defer stop()
		var releaseOnce sync.Once
		finishLease := func() { releaseOnce.Do(release) }
		defer finishLease()
		progress := func(p directorylinks.MigrationProgress) {
			j.mu.Lock()
			j.current.Progress = p
			j.current.CanCancel = p.Phase != "committing" && p.Phase != "cleanup"
			j.mu.Unlock()
		}
		plan, record, err := run(ctx, progress)
		finishLease()
		j.mu.Lock()
		defer j.mu.Unlock()
		j.current.CanCancel = false
		j.current.Plan = plan
		j.current.Record = record
		j.current.State = "completed"
		if err != nil {
			j.current.State = "failed"
			j.current.Error = err.Error()
			if errors.Is(err, context.Canceled) {
				j.current.State = "cancelled"
			}
		}
		if err == nil && plan != nil {
			if j.plans == nil {
				j.plans = map[string]struct {
					owner   uint32
					expires time.Time
				}{}
			}
			for id, old := range j.plans {
				if time.Now().After(old.expires) {
					delete(j.plans, id)
				}
			}
			expires, _ := time.Parse(time.RFC3339Nano, plan.ExpiresAt)
			j.plans[plan.ID] = struct {
				owner   uint32
				expires time.Time
			}{owner, expires}
		}
	}()
	return initial, nil
}
func (j *directoryMigrationJobs) claim(owner uint32, token string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	p, ok := j.plans[token]
	if !ok || p.owner != owner || time.Now().After(p.expires) {
		return errors.New("预览属于另一窗口、已使用或过期，请重新预览")
	}
	if j.current.State == "running" {
		return errors.New("请等待当前目录任务结束")
	}
	delete(j.plans, token)
	return nil
}
func (j *directoryMigrationJobs) read(owner uint32, id, requestID string, cancel bool) (directoryMigrationJob, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if (id == "" && requestID == "") || (id != "" && j.current.ID != id) || (requestID != "" && j.current.RequestID != requestID) || j.owner != owner {
		return directoryMigrationJob{}, errors.New("目录任务不存在或属于另一窗口")
	}
	if cancel && j.current.CanCancel && j.cancel != nil {
		j.cancel()
	}
	return j.current, nil
}

// Long operations are admitted once and observed by ticket. Polling never
// resubmits a filesystem change; cancellation affects only this window's job.
func (j *directoryMigrationJobs) handle(m *directorylinks.Manager, request host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "directory_links_migration", Message: err.Error()}}
	}
	var value any
	var err error
	switch request.Method {
	case "migration.status", "migration.cancel":
		var args struct {
			ID        string `json:"id"`
			RequestID string `json:"requestId"`
		}
		if err = configjson.DecodeObject(request.Params, &args); err != nil {
			return fail(err)
		}
		value, err = j.read(request.PeerPID, args.ID, args.RequestID, request.Method == "migration.cancel")
	case "migration.preview":
		var args directorylinks.MigrationRequest
		if err = configjson.DecodeObject(request.Params, &args); err != nil {
			return fail(err)
		}
		value, err = j.start(request.PeerPID, args.RequestID, func(ctx context.Context, progress func(directorylinks.MigrationProgress)) (*directorylinks.MigrationPlan, *directorylinks.Record, error) {
			p, e := m.PreviewMigration(ctx, args, progress)
			if e != nil {
				return nil, nil, e
			}
			return &p, nil, nil
		})
	case "migration.execute":
		var args struct {
			Token        string `json:"token"`
			RequestID    string `json:"requestId"`
			Confirmed    bool   `json:"confirmed"`
			Acknowledged bool   `json:"acknowledged"`
		}
		if err = configjson.DecodeObject(request.Params, &args); err != nil {
			return fail(err)
		}
		if !args.Confirmed {
			return fail(errors.New("请确认本次搬迁或还原"))
		}
		if err = j.claim(request.PeerPID, args.Token); err != nil {
			return fail(err)
		}
		value, err = j.start(request.PeerPID, args.RequestID, func(ctx context.Context, progress func(directorylinks.MigrationProgress)) (*directorylinks.MigrationPlan, *directorylinks.Record, error) {
			r, e := m.ExecuteMigration(ctx, args.Token, args.Acknowledged, progress)
			if r.ID == "" {
				return nil, nil, e
			}
			return nil, &r, e
		})
	case "migration.recover", "migration.cleanup":
		var args struct {
			ID        string `json:"id"`
			Index     int    `json:"index"`
			Path      string `json:"path"`
			RequestID string `json:"requestId"`
			Confirmed bool   `json:"confirmed"`
		}
		if err = configjson.DecodeObject(request.Params, &args); err != nil {
			return fail(err)
		}
		if !args.Confirmed {
			return fail(errors.New("请确认恢复或将备份移入回收站"))
		}
		value, err = j.start(request.PeerPID, args.RequestID, func(ctx context.Context, progress func(directorylinks.MigrationProgress)) (*directorylinks.MigrationPlan, *directorylinks.Record, error) {
			progress(directorylinks.MigrationProgress{Phase: "cleanup"})
			var r directorylinks.Record
			var e error
			if request.Method == "migration.recover" {
				r, e = m.RecoverMigration(ctx, args.ID)
			} else {
				r, e = m.CleanupMigrationArtifact(ctx, args.ID, args.Index, args.Path)
			}
			if r.ID == "" {
				return nil, nil, e
			}
			return nil, &r, e
		})
	default:
		return fail(errors.New("不支持的目录任务"))
	}
	if err != nil {
		return fail(err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fail(err)
	}
	return host.Response{ID: request.ID, Result: raw}
}

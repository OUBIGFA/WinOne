//go:build windows

package adminstartup

import (
	"context"
	"encoding/hex"
	"errors"
	"time"
)

// RestartPorts lets each desktop protect its own edits and close its own window,
// while sharing the Host token, helper acknowledgement and rollback protocol.
type RestartPorts struct {
	Guard  func() error
	Call   func(string, any) (HandoffSnapshot, error)
	Launch func() error
	Close  func(string) error
}

// Restart requests closure only after the verified helper acknowledges commit.
func Restart(ctx context.Context, p RestartPorts) (err error) {
	if err = p.Guard(); err != nil {
		return err
	}
	job, err := p.Call("restart.prepare", nil)
	if err != nil {
		return err
	}
	token := job.Token
	if len(token) != 32 {
		return errors.New("重启准备标识无效")
	}
	if _, err = hex.DecodeString(token); err != nil {
		return errors.New("重启准备标识无效")
	}
	committed := false
	defer func() {
		if !committed {
			_, cancelErr := p.Call("restart.cancel", map[string]string{"token": token})
			err = errors.Join(err, cancelErr)
		}
	}()
	check := func() error {
		if job.Token != token {
			return errors.New("重启准备已变化")
		}
		if job.State == "failed" || job.State == "cancelled" {
			if job.Error != "" {
				return errors.New(job.Error)
			}
			return errors.New("重启已取消")
		}
		if job.State != "preparing" && job.State != "prepared" && job.State != "committed" {
			return errors.New("无法确认重启状态")
		}
		return nil
	}
	wait := func(ready func() bool) error {
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := check(); err != nil {
				return err
			}
			if ready() {
				return nil
			}
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			var err error
			job, err = p.Call("restart.status", map[string]string{"token": token})
			if err != nil {
				return err
			}
		}
	}
	if err = wait(func() bool { return job.State == "prepared" }); err != nil {
		return err
	}
	if err = p.Launch(); err != nil {
		return err
	}
	if err = wait(func() bool { return job.State == "prepared" && job.HelperPID != 0 }); err != nil {
		return err
	}
	if err = p.Guard(); err != nil {
		return err
	}
	job, err = p.Call("restart.commit", map[string]string{"token": token})
	if err != nil {
		return err
	}
	if err = check(); err != nil {
		return err
	}
	if job.State != "committed" {
		return errors.New("重启尚未提交")
	}
	committed = true
	if err = wait(func() bool { return job.State == "committed" && job.HelperAcknowledged }); err != nil {
		return err
	}
	return p.Close(token)
}

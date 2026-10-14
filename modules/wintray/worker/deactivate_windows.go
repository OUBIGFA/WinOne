//go:build windows

package worker

// Deactivation preserves user preferences, while returning module-owned startup
// sources and tray resources. The product's own login trigger belongs to Host.
func (r *Runtime) startDeactivation() job {
	return r.startResourceRelease(false)
}

// Uninstall additionally removes the product login trigger, but only after
// resources are restored. A failed trigger removal can be retried without
// reactivating managed programs or rewriting their preferences.
func (r *Runtime) startResourceRelease(uninstall bool) job {
	r.jobsMu.Lock()
	defer r.jobsMu.Unlock()
	if r.closing || r.ctx.Err() != nil {
		return job{State: "failed", Error: "模块正在退出"}
	}
	if r.deactivated && !uninstall {
		return job{State: "completed"}
	}
	if r.deactivating {
		return job{State: "failed", Error: "模块正在关闭"}
	}
	for _, pending := range r.jobs {
		if pending.State == "running" {
			return job{State: "failed", Error: "请等待当前程序任务完成后关闭模块"}
		}
	}
	r.deactivating = true
	return r.enqueueJobLocked(func() (any, error) {
		r.effectsMu.Lock()
		defer r.effectsMu.Unlock()
		defer func() { r.jobsMu.Lock(); r.deactivating = false; r.jobsMu.Unlock() }()
		r.jobsMu.Lock()
		restored := r.deactivated
		r.jobsMu.Unlock()
		if !restored {
			// Withdraw readiness before touching tasks, so waiting helpers cannot start new programs.
			if err := r.onUI(func() error {
				if r.readyRelease != nil {
					r.readyRelease()
					r.readyRelease = nil
				}
				return nil
			}); err != nil {
				return nil, err
			}
			if r.allowSystemStartup {
				if err := r.tasks.ReleaseAll(); err != nil {
					return nil, err
				}
			}
			if err := r.onUI(func() error {
				if err := r.box.Close(); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return nil, err
			}
			if err := r.restoreHostedWindows(); err != nil {
				return nil, err
			}
			r.jobsMu.Lock()
			r.deactivated = true
			r.jobsMu.Unlock()
		}
		if uninstall && r.allowSystemStartup {
			if err := r.productLogon.Remove(); err != nil {
				return nil, err
			}
		}
		return map[string]bool{"restored": true}, nil
	})
}

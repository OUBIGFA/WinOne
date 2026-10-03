//go:build windows

package worker

import "time"

// A successful explicit reconciliation can recover failed initialization;
// otherwise that stale flag would keep an empty, healthy worker alive forever.
func (r *Runtime) markReconciled() {
	r.jobsMu.Lock()
	defer r.jobsMu.Unlock()
	if r.initializationState == "failed" {
		r.initializationState = "ready"
		r.initializationError = ""
	}
}

func (r *Runtime) tryIdleExit(now time.Time) bool {
	snapshot := r.manager.Snapshot()
	r.jobsMu.Lock()
	busy := r.closing || r.deactivating || r.deactivated
	for _, job := range r.jobs {
		if job.State == "running" {
			busy = true
			break
		}
	}
	condition := idleConditions{Managed: snapshot.ConfigurationManaged, Initialized: r.initializationState == "ready", Programs: len(snapshot.Settings.ManagedApps), Busy: busy, HasError: snapshot.RecoveryError != "" || r.lastError != "" || r.startupError != "" || Validate(snapshot.Settings) != nil, RecoveryOnly: r.recoverOnly}
	if !condition.eligible() {
		r.idleSince = time.Time{}
		r.jobsMu.Unlock()
		return false
	}
	if r.idleSince.IsZero() {
		r.idleSince = now
	}
	if now.Sub(r.idleSince) < 3*time.Second {
		r.jobsMu.Unlock()
		return false
	}
	r.closing = true
	r.jobsMu.Unlock()
	r.cancel()
	return true
}

//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// taskDelivery records Cindy's final-delivery judgement before the tool returns.
// Ordinary Chat and OneBot share this conversation-bound bridge. It never starts
// an idle OneBot worker; an owner without teammates has no follow-up to notify.
func (c *chatControl) taskDelivery(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	if strings.TrimPrefix(r.URL.Path, "/v1") != "/task/delivery" {
		return false
	}
	if r.Method != http.MethodPost {
		http.Error(w, "不支持的交付请求", http.StatusMethodNotAllowed)
		return true
	}
	var input struct {
		RequestID     string `json:"requestId"`
		FinalDelivery *bool  `json:"finalDelivery"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil || input.RequestID == "" || len(input.RequestID) > 128 || input.FinalDelivery == nil {
		http.Error(w, "交付参数无效", http.StatusBadRequest)
		return true
	}
	fail := func(err error) bool { http.Error(w, err.Error(), http.StatusConflict); return true }
	b := c.onebot
	if b == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "tracked": false})
		return true
	}
	// Keep this report ordered with phase pushes: completion must not overtake it.
	b.activitySend.Lock()
	defer b.activitySend.Unlock()
	b.startMu.Lock()
	runner, stopped := b.runner, b.stopped
	b.startMu.Unlock()
	if runner == nil || stopped {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "tracked": false})
		return true
	}
	control, _, err := b.taskConversation(sessionID)
	if err != nil {
		return fail(err)
	}
	if control.engine == nil {
		return fail(errors.New("任务执行器尚未就绪"))
	}
	raw, err := control.engine.Command(r.Context(), sessionID, "execution.inspect", nil)
	if err != nil {
		return fail(err)
	}
	var live struct {
		Running   bool   `json:"running"`
		RequestID string `json:"requestId"`
		Execution *struct {
			InstanceID string `json:"instanceId"`
			Generation uint64 `json:"generation"`
		} `json:"execution"`
	}
	if err = json.Unmarshal(raw, &live); err != nil {
		return fail(err)
	}
	if !live.Running || live.RequestID != input.RequestID || live.Execution == nil || live.Execution.InstanceID == "" {
		return fail(errors.New("任务输入已变化，未将旧轮次的交付状态写入当前轮次"))
	}
	snapshots := control.engine.ActivitySnapshots(sessionID)
	if len(snapshots) != 1 || snapshots[0].Phase != "running" {
		return fail(errors.New("当前轮次已结束，未记录交付状态"))
	}
	result, err := runner.Call(r.Context(), "autonomy.reportDelivery", map[string]any{"snapshot": snapshots[0], "execution": live.Execution, "finalDelivery": *input.FinalDelivery})
	if err != nil {
		return fail(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(result)
	return true
}

//go:build windows

package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/automations"
	"winone/desktopmygo/chat/pluginhost"
	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/internal/ai"
)

type automationNativeNode struct {
	richNativeNode
	Description string
}

// Supplement the already verified template/project-import matrix, not replay
// it. This is the production automation form/history in a real narrow window,
// bound to the actual Cindy scheduler/Host. A local completion HTTP fixture is
// the only model; unconfigured remote notifications must become real recorded
// errors. Desktop notifications and external accounts are never used.
func TestRealNativeAutomationNotificationBindingAndDraftGuards(t *testing.T) {
	if os.Getenv("WINONE_NATIVE_RICH_MATRIX") != "1" {
		t.Skip("explicit private desktop/real Host automation entry")
	}
	client, err := host.NewClient(os.Getenv("WINONE_NATIVE_UI_REAL_PIPE"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cancel()
	call := func(method string, params, out any) {
		t.Helper()
		if err := client.CallInto(method, params, out); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	var requests atomic.Uint32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"native-automation\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Verified native automation reply\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"native-automation\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\ndata: [DONE]\n\n")
	}))
	defer vendor.Close()
	var configuration ai.Configuration
	call("capture.ai.configuration.get", nil, &configuration)
	configuration.Providers = []ai.Provider{{ID: "native-automation", Name: "Isolated automation fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Models: []string{"model"}}}
	configuration.Tasks = map[string]ai.Selection{"chat": {Provider: "native-automation", Model: "model"}, "default": {Provider: "native-automation", Model: "model"}}
	call("capture.ai.configuration.save", map[string]any{"configuration": configuration, "keys": map[string]string{"native-automation": "fixture-not-an-account"}}, nil)
	profile := os.Getenv("WINONE_NATIVE_UI_REAL_PROFILE")
	work := filepath.Join(filepath.Dir(profile), "automation-owner")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	var owner Conversation
	call("chat.conversation.new", map[string]string{"directory": work, "title": "Manual automation owner"}, &owner)
	call("chat.draft.save", map[string]any{"id": owner.ID, "draft": map[string]any{"text": "Keep the manual owner's unsent draft", "attachments": []any{}}}, nil)
	var schedule automations.Schedule
	call("chat.automations.create", map[string]any{"input": map[string]any{"name": "Native automation guards", "prompt": "Return the fixture reply", "kind": "cron", "cronExpr": "0 9 * * *", "timezone": "UTC", "manual": true, "recurring": true, "agentKind": "pi", "executionMode": "agent", "workspaceKind": "project", "workingDir": work, "targetSessionId": owner.ID, "notify": map[string]bool{"desktop": false, "feishu": true, "wecomGroup": true}}}, &schedule)
	if owner.ID == "" || schedule.ID == "" || schedule.TargetSessionID != owner.ID {
		t.Fatal("the real scheduler did not bind the hand-owned conversation")
	}
	var window *mygo.Window
	var page *Chat
	env := &core.Env{Host: client, Directory: profile, Activity: &core.Activity{}, Rail: &core.Rail{}, Refresh: func() {}, Toast: func(string) {}, Focuses: func() int { return 0 }, TakeIntents: func(string) []core.Intent { return nil }, Navigate: func(string, func()) {}, Open: func(string, core.Intent) {}, RequestChange: func(fn func()) { fn() }}
	env.Post = func(fn func()) {
		if window != nil && !window.IsDestroyed() {
			window.Update(fn)
		}
	}
	env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) { return window.NewWebView(options) }
	mygo.RunOnMain(func() {
		window = mygo.NewWindow(mygo.WindowOptions{Title: "Native automation notification and draft guards", Width: 560, Height: 900, Hidden: true, Content: ui.View(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), false))
			if page != nil {
				page.View(c)
			}
		})})
		page = New(env)
		page.setSidebar(false)
		page.ui.automations = true
		window.SetContentSize(560, 900)
		window.Show()
	})
	defer mygo.RunOnMain(func() { page.Close(); pluginhost.For(env).Close(); window.Destroy() })
	tr := func(key string, args ...any) string { return i18n.T("nativeAutomation."+key, args...) }
	nodes := func() []automationNativeNode {
		t.Helper()
		var tree struct{ Nodes []automationNativeNode }
		if err := json.Unmarshal(richNativeCall(t, "tree", map[string]any{"window": window.ID()}), &tree); err != nil {
			t.Fatal(err)
		}
		return tree.Nodes
	}
	find := func(label, action string) *automationNativeNode {
		for _, node := range slices.Backward(nodes()) {
			if node.Label == label && (action == "" || slices.Contains(node.Actions, action)) {
				copy := node
				return &copy
			}
		}
		return nil
	}
	await := func(label string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s did not settle; actual native nodes=%+v", label, nodes())
	}
	act := func(label, action, text string) {
		t.Helper()
		await("actual control "+label, func() bool { return find(label, action) != nil })
		node := find(label, action)
		if slices.Contains(node.States, "offscreen") {
			richNativeCall(t, "act", map[string]any{"window": window.ID(), "id": node.ID, "action": "scrollIntoView"})
			node = find(label, action)
		}
		if node == nil || slices.Contains(node.States, "disabled") {
			t.Fatalf("actual native action is disabled: %s", label)
		}
		richNativeCall(t, "act", map[string]any{"window": window.ID(), "id": node.ID, "action": action, "text": text})
	}
	shot := func(name string) {
		t.Helper()
		// Capture only a settled production frame. A Host/store receipt may
		// precede the native form's final close or connector-refresh paint.
		richNativeCall(t, "idle", map[string]any{"window": window.ID(), "timeout": 5000})
		tree := nodes()
		for _, node := range tree {
			if node.Bounds.Width > 0 && (node.Bounds.X < -1 || node.Bounds.X+node.Bounds.Width > 561) {
				t.Fatalf("%s native content escaped the narrow viewport: %+v", name, node)
			}
		}
		frame, err := window.CapturePage()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(os.Getenv("WINONE_NATIVE_UI_SHOTS"), name+".png"), frame, 0600); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.MarshalIndent(tree, "", "  ")
		if err := os.WriteFile(filepath.Join(os.Getenv("WINONE_NATIVE_UI_SHOTS"), name+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	get := func() automations.Schedule {
		var value automations.Schedule
		call("chat.automations.get", map[string]string{"id": schedule.ID}, &value)
		return value
	}
	act(schedule.Name, "press", "")
	act(tr("edit"), "press", "")
	act(tr("name"), "setValue", "Native automation saved from UI")
	act(tr("refreshConnections"), "press", "")
	// Switch help is the native control's Description, not another text node.
	// Read the real accessibility contract instead of weakening the status
	// check or assuming that a Host availability response was drawn.
	await("real unconfigured connector feedback", func() bool {
		for _, key := range []string{"feishuNotify", "wecomNotify"} {
			node := find(tr(key), "press")
			if node == nil || node.Description != tr("connectorUnconfigured") {
				return false
			}
		}
		return true
	})
	for _, key := range []string{"feishuNotify", "wecomNotify"} {
		checkbox := find(tr(key), "press")
		if checkbox == nil || !slices.Contains(checkbox.States, "checked") {
			t.Fatalf("unconfigured connector reset the saved native choice: %s %+v", key, checkbox)
		}
	}
	shot("automation-saved-notification-choices")
	act(tr("cancel"), "press", "")
	act(tr("continueEditing"), "press", "")
	if saved := get(); saved.Name != schedule.Name || saved.TargetSessionID != owner.ID {
		t.Fatal("native Cancel/continue silently committed or replaced the bound owner")
	}
	mygo.RunOnMain(func() {
		if !page.tools.automations.Dirty() {
			t.Error("native continue-editing lost the unsaved automation form")
		}
	})
	act(tr("save"), "press", "")
	await("actual native form save", func() bool { return get().Name == "Native automation saved from UI" })
	saved := get()
	if saved.TargetSessionID != owner.ID || !saved.Notify.Feishu || !saved.Notify.WecomGroup || saved.Notify.Desktop {
		t.Fatalf("native delta save lost binding/notifications or enabled real desktop notification: %+v", saved)
	}
	act(tr("run"), "press", "")
	var completed automations.Run
	await("actual bound Pi run and recorded notification failure", func() bool {
		var runs []automations.Run
		call("chat.automations.runs", map[string]string{"id": schedule.ID}, &runs)
		for _, run := range runs {
			if run.Status == "success" && run.SessionID == owner.ID && run.ResultText == "Verified native automation reply" && run.NotificationError != "" {
				completed = run
				return true
			}
		}
		return false
	})
	act(tr("refresh"), "press", "")
	await("actual native notification-error history", func() bool {
		for _, node := range nodes() {
			if strings.Contains(node.Label, completed.NotificationError) {
				return true
			}
		}
		return false
	})
	shot("automation-real-notification-failure")
	var draft struct{ Text string }
	call("chat.draft.get", map[string]string{"id": owner.ID}, &draft)
	if draft.Text != "Keep the manual owner's unsent draft" {
		t.Fatal("scheduled bound turn replaced the hand-owned unsent draft")
	}
	act(tr("delete"), "press", "")
	await("hand-bound owner excluded from generated deletion", func() bool { return find(tr("generated", "count", 0), "") != nil })
	act(tr("keepSessions"), "press", "")
	act(tr("deleteSessions"), "press", "")
	act(tr("confirm"), "press", "")
	await("actual schedule soft deletion", func() bool {
		var deleted []automations.Schedule
		call("chat.automations.deleted", nil, &deleted)
		return slices.ContainsFunc(deleted, func(item automations.Schedule) bool { return item.ID == schedule.ID })
	})
	var catalog Catalog
	call("chat.catalog", nil, &catalog)
	if current := catalog.conversation(owner.ID); current == nil || current.Archived {
		t.Fatal("native generated-session deletion touched the actual manually bound owner")
	}
	act(tr("deleted"), "press", "")
	act(tr("restore"), "press", "")
	// Cindy restores records paused, rather than silently resuming execution.
	await("real restore keeps paused bound definition", func() bool {
		var restored []automations.Schedule
		call("chat.automations.list", nil, &restored)
		return slices.ContainsFunc(restored, func(value automations.Schedule) bool {
			return value.ID == schedule.ID && value.Status == "paused" && value.TargetSessionID == owner.ID
		})
	})
	var restoredRuns []automations.Run
	call("chat.automations.runs", map[string]string{"id": schedule.ID}, &restoredRuns)
	if len(restoredRuns) != 1 || restoredRuns[0].ID != completed.ID || restoredRuns[0].NotificationError != completed.NotificationError {
		t.Fatal("native restore lost the real execution/notification history")
	}
	act(tr("close"), "press", "")
	act(tr("create"), "press", "")
	await("actual new automation form", func() bool { return find(tr("name"), "setValue") != nil })
	for _, key := range []string{"feishuNotify", "wecomNotify"} {
		node := find(tr(key), "")
		if node == nil || !slices.Contains(node.States, "disabled") || slices.Contains(node.States, "checked") {
			t.Fatalf("an unconfigured remote notification could be newly enabled: %s %+v", key, node)
		}
	}
	act(tr("name"), "setValue", "Discarded new automation must not exist")
	act(tr("cancel"), "press", "")
	act(tr("discard"), "press", "")
	await("actual discarded form closed", func() bool { return find(tr("name"), "setValue") == nil && find(tr("discard"), "press") == nil })
	var final []automations.Schedule
	call("chat.automations.list", nil, &final)
	if len(final) != 1 || final[0].ID != schedule.ID || final[0].Status != "paused" || final[0].TargetSessionID != owner.ID || !final[0].Notify.Feishu || !final[0].Notify.WecomGroup || final[0].Notify.Desktop {
		t.Fatalf("discard created a schedule or lost the original binding: %+v", final)
	}
	shot("automation-restore-and-discard")
	if requests.Load() == 0 {
		t.Fatal("bound run was a queue acknowledgement, not a genuine model request")
	}
	t.Logf("actual native 560 window: saved notifications retained with real unconfigured status, cancel/continue/save and new-draft discard; bound Pi completed run=%s owner=%s notificationError=%q; generated-delete excludes manual owner; real restore kept definition; vendor requests=%d", completed.ID, owner.ID, completed.NotificationError, requests.Load())
}

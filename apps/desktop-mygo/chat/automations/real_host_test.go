package automations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/kit"
)

// Real Cindy Scheduler/store behind the native form/import/template controls.
// Run only via the explicit isolated native matrix entry, never a user's Host.
func TestRealHostNativeTemplatesAndExactProjectImport(t *testing.T) {
	pipe, profile := os.Getenv("WINONE_NATIVE_UI_REAL_PIPE"), os.Getenv("WINONE_NATIVE_UI_REAL_PROFILE")
	if pipe == "" || profile == "" {
		t.Skip("explicit real Host/native matrix entry required")
	}
	client, err := host.NewClient(pipe)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cancel()
	var work sync.WaitGroup
	var mu sync.Mutex
	var posted []func()
	env := &core.Env{Host: client, Directory: profile, Activity: &core.Activity{}, Rail: &core.Rail{}, Spawn: func(fn func()) { work.Add(1); go func() { defer work.Done(); fn() }() }, Post: func(fn func()) { mu.Lock(); posted = append(posted, fn); mu.Unlock() }, Toast: func(string) {}, RequestChange: func(fn func()) { fn() }, Refresh: func() {}}
	p := New(env, Options{})
	defer p.Close()
	p.loaded = true
	// Render the same production dialogs without starting the full page's
	// long-poll loop; the separate real-window stage verifies that lifecycle.
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(kit.Theme(c.Preferences(), false))
		p.editorView(c)
		p.templatesView(c)
		p.importsView(c)
		p.confirmView(c)
	}, 1000, 900)
	settle := func() {
		t.Helper()
		for range 100 {
			work.Wait()
			mu.Lock()
			queue := posted
			posted = nil
			mu.Unlock()
			if len(queue) == 0 {
				tt.Frame()
				return
			}
			for _, fn := range queue {
				fn()
			}
			tt.Frame()
		}
		t.Fatal("real automation dialogs did not settle")
	}
	call := func(method string, params, out any) {
		t.Helper()
		if err := p.call(method, params, out); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	call("runtime", nil, &p.runtime)
	call("list", nil, &p.items)
	var original *Schedule
	for _, schedule := range p.items {
		if schedule.Name == "Matrix Script" {
			copy := schedule
			original = &copy
		}
	}
	if original == nil {
		t.Fatal("the real window's script schedule was not persisted")
	}
	p.openEditor(original)
	settle()
	p.draft.Value.Name = "Native template matrix"
	p.saveTemplate()
	settle()
	if p.err != "" {
		t.Fatal(p.err)
	}
	p.Discard()
	p.showTemplates()
	settle()
	var template *Template
	for _, item := range p.templates {
		if item.Name == "Native template matrix" {
			copy := item
			template = &copy
		}
	}
	if template == nil {
		t.Fatalf("native saveTemplate did not persist: %+v", p.templates)
	}
	p.applyTemplate(*template)
	settle()
	if p.draft == nil || p.draft.timing != "manual" || p.draft.Value.ExecutionMode != "script" || p.draft.Value.ScriptConfig == nil || p.draft.Value.ScriptConfig.Command != original.ScriptConfig.Command {
		t.Fatal("actual template application lost its execution definition")
	}
	p.draft.Value.Name = "Created from native template"
	p.save()
	settle()
	if p.draft != nil || p.err != "" {
		t.Fatalf("actual form create failed: %s", p.err)
	}
	created := ""
	for _, schedule := range p.items {
		if schedule.Name == "Created from native template" {
			created = schedule.ID
		}
	}
	if created == "" {
		t.Fatal("the template form was not saved by the real scheduler")
	}
	// Project conversion uses the actual stored definition, not a test copy.
	p.confirmation = &confirmation{action: "project.promote", id: created}
	p.confirm()
	settle()
	var promoted Schedule
	// Cindy promote creates a project-owned record and retires the personal
	// id; follow the real replacement identity rather than assuming an update.
	for _, schedule := range p.items {
		if schedule.Source == "project" && schedule.Name == "Created from native template" {
			promoted = schedule
		}
	}
	if promoted.ID == "" || promoted.ID == created {
		t.Fatalf("native promotion receipt=%+v", promoted)
	}
	created = promoted.ID
	path := filepath.Join(promoted.WorkingDir, ".cindy", "automations", "schedules.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	rows, ok := config["schedules"].([]any)
	if !ok {
		t.Fatalf("real project configuration missing schedules: %s", raw)
	}
	changed := false
	for _, row := range rows {
		item := row.(map[string]any)
		if item["name"] == promoted.Name {
			item["name"] = "Approved native import"
			changed = true
		}
	}
	if !changed {
		t.Fatal("promoted schedule was absent from its actual project configuration")
	}
	raw, _ = json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	call("project.reconcile", map[string]string{"workingDir": promoted.WorkingDir}, nil)
	call("project.pendingImports", nil, &p.imports)
	var pending *PendingImport
	for _, item := range p.imports {
		if strings.EqualFold(item.WorkingDir, promoted.WorkingDir) {
			copy := item
			pending = &copy
		}
	}
	if pending == nil || pending.ConfigHash == "" {
		t.Fatalf("changed project was not held for approval: %+v", p.imports)
	}
	var before Schedule
	call("get", map[string]string{"id": created}, &before)
	if before.Name != promoted.Name {
		t.Fatal("unapproved project change was silently applied")
	}
	p.importOpen = true
	settle()
	if err := tt.Click(tr("approveImport")); err != nil {
		t.Fatal(err)
	}
	settle()
	call("get", map[string]string{"id": created}, &promoted)
	if promoted.Name != "Approved native import" {
		t.Fatal("native import approval did not apply its exact preview")
	}
	p.importOpen = false
	beforeClone := map[string]bool{}
	for _, schedule := range p.items {
		beforeClone[schedule.ID] = true
	}
	p.mutate("project.clone", created, map[string]string{"id": created}, nil)
	settle()
	clones := 0
	for _, schedule := range p.items {
		if !beforeClone[schedule.ID] && schedule.Source != "project" && schedule.ScriptConfig != nil && schedule.ScriptConfig.Command == promoted.ScriptConfig.Command {
			clones++
		}
	}
	if clones != 1 {
		t.Fatalf("actual project clone count=%d", clones)
	}
	beforeDemote := map[string]bool{}
	for _, schedule := range p.items {
		beforeDemote[schedule.ID] = true
	}
	p.confirmation = &confirmation{action: "project.demote", id: created}
	p.confirm()
	settle()
	demoted := false
	for _, schedule := range p.items {
		if schedule.ID == created {
			t.Fatal("native demotion kept the project record")
		}
		if !beforeDemote[schedule.ID] && schedule.Source == "user" && schedule.Name == promoted.Name {
			demoted = true
		}
	}
	if !demoted {
		t.Fatal("native demotion did not preserve the personal definition")
	}
	p.confirmation = &confirmation{action: "template.delete", id: template.ID}
	p.confirm()
	settle()
	for _, item := range p.templates {
		if item.ID == template.ID {
			t.Fatal("native template delete did not persist")
		}
	}
	t.Log("real native template save/apply/form-create/delete; project promote/exact-preview-approval/clone/demote")
}

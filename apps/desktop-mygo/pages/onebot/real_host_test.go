package onebot

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// This explicit combination test runs as the isolated WinOne.exe while the
// real Host stays alive. There is no testhost service, model or account mock.
// It covers native settings operations that Windows context menus cannot open
// on the private non-input desktop used by the separate real-window checks.
func TestRealHostNativeProfileMemorySkillAndVoiceCombinations(t *testing.T) {
	p, tt, settle, _ := realOneBotUI(t)
	if p.error != "" || len(p.profiles) < 2 {
		t.Fatalf("real profiles did not load: %s", p.error)
	}
	bot := p.profiles[0]
	d := newProfileSettings(p, bot, "home")
	p.settings = d
	call := func(method string, params, out any) {
		t.Helper()
		if err := p.call(method, params, out); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	// Every local section loads its actual Host data, and switching keeps its
	// production pane lifecycle. No physical input/call or account login occurs.
	for _, section := range []string{"profile", "personality", "model", "memory", "capabilities", "projects", "history", "routines", "todos", "glance", "channels", "voice", "microphone", "advanced"} {
		d.requestSection(section)
		settle()
		if d.error != "" {
			t.Fatalf("%s: %s", section, d.error)
		}
		// Read failures stay on the pane, not the settings drawer. Verify the
		// actual returned data/error boundary for every local section.
		var paneError string
		switch pane := d.panes[section].(type) {
		case *memoryPane:
			paneError = pane.error
		case *capabilitiesPane:
			paneError = pane.error
		case *projectsPane:
			paneError = pane.error
		case *historyPane:
			paneError = pane.error
		case *routinesPane:
			paneError = pane.error
		case *todosPane:
			paneError = pane.error
		case *glancePane:
			paneError = pane.error
		case *channelsPane:
			paneError = pane.error
		case *voiceSettingsPane:
			paneError = pane.error
		case *microphonePane:
			paneError = pane.error
		}
		if paneError != "" {
			t.Fatalf("%s actual data failed: %s", section, paneError)
		}
		realProfileShots(t, section, p.View)
	}
	d.requestSection("profile")
	settle()
	if err := tt.Click(nt("name")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("Matrix native profile saved")
	d.Save()
	settle()
	var saved Profile
	call("profiles.get", map[string]string{"botId": bot.ID}, &saved)
	if saved.Name != "Matrix native profile saved" || d.Dirty() {
		t.Fatalf("native profile save: %q dirty=%v", saved.Name, d.Dirty())
	}
	// Real memory import gives the production editor a file and CAS revision.
	call("memory.importDocument", map[string]any{"botId": bot.ID, "id": "matrix-memory", "title": "Matrix memory", "text": "Original body", "type": "user"}, nil)
	d.requestSection("memory")
	settle()
	m := d.pane("memory").(*memoryPane)
	m.Refresh()
	settle()
	filename := ""
	for _, row := range m.rows {
		if row.Title == "Matrix memory" {
			filename = row.Filename
		}
	}
	if filename == "" {
		t.Fatalf("real memory import absent: %+v", m.rows)
	}
	m.open(filename)
	settle()
	if err := tt.Click(bt("bots.memory.edit")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(bt("bots.memory.bodyLabel")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("Native memory body")
	m.Save()
	settle()
	var detail MemoryDetail
	call("memory.read", map[string]string{"botId": bot.ID, "filename": filename}, &detail)
	if detail.Body != "Native memory body" || m.Dirty() {
		t.Fatalf("real memory save: %+v", detail)
	}
	m.detail.Body = "Keep my conflicting edit"
	call("memory.update", map[string]string{"botId": bot.ID, "filename": filename, "title": detail.Title, "body": "Changed externally", "expectedUpdatedAt": detail.UpdatedAt}, nil)
	m.Save()
	settle()
	if m.latest == nil || m.detail.Body != "Keep my conflicting edit" {
		t.Fatalf("real memory conflict did not retain both versions: %+v error=%s", m.latest, m.error)
	}
	if err := tt.Click(bt("bots.memory.keepMine")); err != nil {
		t.Fatal(err)
	}
	settle()
	call("memory.read", map[string]string{"botId": bot.ID, "filename": filename}, &detail)
	if detail.Body != "Keep my conflicting edit" || m.Dirty() {
		t.Fatal("real conflict resolution did not save against the newer revision")
	}
	call("skills.save", map[string]any{"botId": bot.ID, "input": map[string]string{"slug": "matrix-skill", "name": "Matrix skill", "description": "Isolated fixture", "body": "Use the real native editor."}}, nil)
	d.requestSection("capabilities")
	settle()
	c := d.pane("capabilities").(*capabilitiesPane)
	c.Refresh()
	settle()
	c.loadSkill("matrix-skill")
	settle()
	if err := tt.Click(bt("bots.capabilities.learned.edit")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(nt("skillBody")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("Updated from the native skill editor.")
	c.Save()
	settle()
	var skill SkillDetail
	call("skills.read", map[string]string{"botId": bot.ID, "slug": "matrix-skill"}, &skill)
	if !strings.Contains(skill.Body, "Updated from the native skill editor.") || c.Dirty() {
		t.Fatalf("real skill save: %+v", skill)
	}
	if err := tt.Click(bt("bots.capabilities.learned.use")); err != nil {
		t.Fatal(err)
	}
	settle()
	call("skills.read", map[string]string{"botId": bot.ID, "slug": "matrix-skill"}, &skill)
	if skill.Enabled == nil || *skill.Enabled {
		t.Fatal("native skill disable did not persist")
	}
	d.requestSection("voice")
	settle()
	v := d.pane("voice").(*voiceSettingsPane)
	before := v.sentence
	if err := tt.Click(nt("sentencePlayback")); err != nil {
		t.Fatal(err)
	}
	v.Save()
	settle()
	var preference SpeechPreference
	call("voice.preference", map[string]string{"botId": bot.ID}, &preference)
	if preference.SentencePlayback == nil || *preference.SentencePlayback == before || v.Dirty() {
		t.Fatal("real inherited voice preference did not save")
	}
	// An IM form can be cancelled without connecting any external service.
	d.requestSection("channels")
	settle()
	channels := d.pane("channels").(*channelsPane)
	if len(channels.rows) != 0 {
		t.Fatal("isolated profile unexpectedly owns an IM account")
	}
	if err := tt.Click(nt("addConnection")); err != nil {
		t.Fatal(err)
	}
	settle()
	channels.token = "unsaved-fixture-not-an-account"
	channels.Discard()
	settle()
	if channels.Dirty() || channels.adding || channels.token != "" {
		t.Fatal("cancelled IM form retained secret draft state")
	}
	// Retain read data for the report, rather than treating source/route names
	// as proof that their actual requests succeeded.
	raw, _ := json.Marshal(map[string]any{"profile": saved.Name, "memory": detail.Body, "skillEnabled": skill.Enabled, "sentencePlayback": preference.SentencePlayback})
	t.Log(string(raw))
}

// Share the real pipe/UI-thread boundary between independently selectable
// matrices so new conversation checks do not rerun already verified profiles.
func realOneBotUI(t *testing.T) (*page, *ui.Tester, func(), func(func(*ui.Context))) {
	t.Helper()
	pipe, directory := os.Getenv("WINONE_NATIVE_UI_REAL_PIPE"), os.Getenv("WINONE_NATIVE_UI_REAL_PROFILE")
	if pipe == "" || directory == "" {
		t.Skip("explicit real Host/native matrix entry required")
	}
	client, err := host.NewClient(pipe)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Cancel)
	var work sync.WaitGroup
	var mu sync.Mutex
	var posted []func()
	env := &core.Env{Host: client, Directory: directory, Activity: &core.Activity{}, Rail: &core.Rail{},
		Spawn:         func(fn func()) { work.Add(1); go func() { defer work.Done(); fn() }() },
		Post:          func(fn func()) { mu.Lock(); posted = append(posted, fn); mu.Unlock() },
		RequestChange: func(fn func()) { fn() }, Refresh: func() {}, Toast: func(string) {},
		Navigate: func(string, func()) {}, Open: func(string, core.Intent) {}, TakeIntents: func(string) []core.Intent { return nil }, Focuses: func() int { return 0 },
	}
	p := &page{env: env, loaded: true, after: func(time.Duration, func()) func() { return func() {} }}
	p.loadPreferences()
	p.loadModels()
	p.refresh()
	t.Cleanup(p.Close)
	view := p.View
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); view(c) }, 1100, 900)
	settle := func() {
		t.Helper()
		for range 100 {
			work.Wait()
			mu.Lock()
			queue := posted
			posted = nil
			mu.Unlock()
			for _, fn := range queue {
				fn()
			}
			tt.Frame()
			// A new pane's actual reads must finish before assertions or images.
			work.Wait()
			mu.Lock()
			pending := len(posted) > 0
			mu.Unlock()
			if !pending {
				return
			}
		}
		t.Fatal("real native controls did not settle")
	}
	settle()
	return p, tt, settle, func(next func(*ui.Context)) { view = next }
}

// Render the same production panes bound to the real Host in every required
// language/theme/width combination. These are component images, not physical
// desktop screenshots or a substitute for the separate native-window test.
func realProfileShots(t *testing.T, name string, view func(*ui.Context)) {
	t.Helper()
	directory := os.Getenv("WINONE_NATIVE_UI_SHOTS")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	before := i18n.Locale()
	defer i18n.SetLocale(before)
	for _, language := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(language)
		for _, dark := range []bool{false, true} {
			for _, width := range []int{1000, 560} {
				tester := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), dark)); view(c) }, width, 900)
				tester.Frame()
				file, err := os.Create(filepath.Join(directory, fmt.Sprintf("onebot-%s-%s-%t-%d.png", name, language, dark, width)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, tester.Image())
				closeErr := file.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("profile snapshot: %v %v", err, closeErr)
				}
			}
		}
	}
}

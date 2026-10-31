package wintray

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"winone/desktopmygo/core"
)

// The documents of the WinTray module, as wintray.snapshot returns them
// (modules/wintray/internal/config): unknown fields are kept, so a save
// writes back everything this window does not edit.

type trayBehavior struct {
	AutoMinimizeAndHideOnLaunch bool `json:"autoMinimizeAndHideOnLaunch"`
	CloseDelaySeconds           int  `json:"closeDelaySeconds"`
}

type schedule struct {
	Enabled           bool `json:"enabled"`
	StartDelayMinutes int  `json:"startDelayMinutes"`
	AutoExitMinutes   int  `json:"autoExitMinutes"`
	FrequencyEnabled  bool `json:"frequencyEnabled"`
	FrequencyDays     int  `json:"frequencyDays"`
	FrequencyRuns     int  `json:"frequencyRuns"`
}

type program struct {
	ID                       string                       `json:"id"`
	Name                     string                       `json:"name"`
	ExePath                  string                       `json:"exePath"`
	Args                     string                       `json:"args"`
	RunOnStartup             bool                         `json:"runOnStartup"`
	CollectTrayIcon          bool                         `json:"collectTrayIcon"`
	LaunchViaLogonTask       bool                         `json:"launchViaLogonTask"`
	LaunchHiddenInBackground bool                         `json:"launchHiddenInBackground"`
	TrayBehavior             core.Preserved[trayBehavior] `json:"trayBehavior"`
	Schedule                 core.Preserved[schedule]     `json:"schedule"`
}

type traySettings struct {
	SchemaVersion                 int                       `json:"schemaVersion"`
	Language                      string                    `json:"language"`
	RunAtLogon                    bool                      `json:"runAtLogon"`
	StartMinimizedToTray          bool                      `json:"startMinimizedToTray"`
	ExitAfterManagedAppsCompleted bool                      `json:"exitAfterManagedAppsCompleted"`
	CloseWindowRetrySeconds       int                       `json:"closeWindowRetrySeconds"`
	StartupIntervalSeconds        int                       `json:"startupIntervalSeconds"`
	ManagedApps                   []core.Preserved[program] `json:"managedApps"`
}

// traySnapshot is the module's saved settings with its revision and the
// errors its worker reports.
type traySnapshot struct {
	Revision        string                       `json:"revision"`
	Settings        core.Preserved[traySettings] `json:"settings"`
	RecoveryError   string                       `json:"recoveryError,omitempty"`
	BackgroundError string                       `json:"backgroundError,omitempty"`
	StartupError    string                       `json:"startupError,omitempty"`
}

// clone copies a settings document through JSON, unknown fields included,
// so a draft never shares a slice with what was saved.
func clone(settings core.Preserved[traySettings]) core.Preserved[traySettings] {
	var copy core.Preserved[traySettings]
	data, err := json.Marshal(settings)
	if err == nil {
		err = json.Unmarshal(data, &copy)
	}
	if err != nil {
		panic(fmt.Errorf("wintray settings do not round-trip: %w", err))
	}
	if copy.Value.ManagedApps == nil {
		copy.Value.ManagedApps = []core.Preserved[program]{}
	}
	return copy
}

// same reports whether two documents hold the same settings.
func same(a, b core.Preserved[traySettings]) bool {
	left, err1 := json.Marshal(a)
	right, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(left) == string(right)
}

// executable reports whether path is a program rather than a script: only
// programs can be started by the sign-in task, collapse into the tray or
// have their tray icon collected.
func executable(path string) bool { return strings.EqualFold(filepath.Ext(path), ".exe") }

// samePath compares paths as Windows does, whatever the slashes and case.
func samePath(a, b string) bool {
	return strings.EqualFold(strings.ReplaceAll(a, "/", `\`), strings.ReplaceAll(b, "/", `\`))
}

// baseName is the file name of path, extension included.
func baseName(path string) string {
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// stem is the file name without its extension.
func stem(path string) string {
	name := baseName(path)
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[:i]
	}
	return name
}

// newProgram is a program added from path with the program manager's
// defaults: started at sign-in, a program collapsed into the tray, a
// script started hidden, no schedule yet (30 minutes, once a day when
// turned on). Adding only edits the draft.
func newProgram(path, name string) core.Preserved[program] {
	exe := executable(path)
	if name == "" {
		name = stem(path)
	}
	value := program{
		ID: newID(), Name: name, ExePath: path, RunOnStartup: true, LaunchHiddenInBackground: !exe,
	}
	value.TrayBehavior.Value = trayBehavior{AutoMinimizeAndHideOnLaunch: exe}
	value.Schedule.Value = schedule{AutoExitMinutes: 30, FrequencyDays: 1, FrequencyRuns: 1}
	return core.Preserved[program]{Value: value}
}

// replaced is p with another target: it keeps its identity, arguments and
// schedule, takes the new file's name, cannot keep collected tray icons,
// and a script cannot keep a program-only launch mode.
func replaced(p program, path, name string) program {
	if samePath(p.ExePath, path) {
		return p
	}
	if name == "" {
		name = stem(path)
	}
	p.ExePath, p.Name, p.CollectTrayIcon = path, name, false
	if !executable(path) {
		p.LaunchViaLogonTask, p.LaunchHiddenInBackground = false, true
		p.TrayBehavior.Value.AutoMinimizeAndHideOnLaunch = false
	}
	return p
}

// mode is how a program starts.
func mode(p program) string {
	switch {
	case p.LaunchViaLogonTask:
		return "task"
	case p.LaunchHiddenInBackground:
		return "hidden"
	case p.TrayBehavior.Value.AutoMinimizeAndHideOnLaunch:
		return "tray"
	}
	return "normal"
}

func setMode(p *program, value string) {
	p.LaunchViaLogonTask = value == "task"
	p.LaunchHiddenInBackground = value == "hidden"
	p.TrayBehavior.Value.AutoMinimizeAndHideOnLaunch = value == "tray"
}

// move puts the program from index before the one at index to, as React's
// splice did: the dragged row takes the target's place.
func move(apps []core.Preserved[program], from, to int) []core.Preserved[program] {
	if from == to || from < 0 || to < 0 || from >= len(apps) || to >= len(apps) {
		return apps
	}
	item := apps[from]
	rest := append(append([]core.Preserved[program]{}, apps[:from]...), apps[from+1:]...)
	return append(rest[:to], append([]core.Preserved[program]{item}, rest[to:]...)...)
}

// newID is a random identifier in the form of a UUID, as React's
// crypto.randomUUID made.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// inRange reports a whole number within bounds.
func inRange(value, minimum, maximum int) bool { return value >= minimum && value <= maximum }

// parseWhole reads a field's text as React's number input and validNumber
// did: a whole number in range, nothing else.
func parseWhole(text string, minimum, maximum int) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(text))
	return value, err == nil && inRange(value, minimum, maximum)
}

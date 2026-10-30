package widgets

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/settings"
)

// Shortcuts edits the global shortcuts of one feature: WinOne's own
// ("application"), "capture", "selection", "quick-assistant" or
// "always-on-top". They are fields of the Host's settings, saved through
// settings.save as one transaction, with each binding's registration state
// from shortcuts.snapshot beside it.
//
// React source: features/shortcuts.tsx.
type Shortcuts struct {
	env       *core.Env
	module    string
	draft     *settings.Settings
	saved     *settings.Settings
	status    *shortcutStatus
	err       string
	loading   bool
	working   bool
	recorders map[string]*ShortcutRecorder
	// wasRecording detects the end of a recording, after which the
	// registrations paused for it are read again.
	wasRecording bool
}

type shortcutStatus struct {
	ActionErrors map[string]string `json:"actionErrors,omitempty"`
	Bindings     []struct {
		ID         string `json:"id"`
		Shortcut   string `json:"shortcut"`
		Registered bool   `json:"registered"`
		Error      string `json:"error,omitempty"`
	} `json:"bindings"`
	LastError string `json:"lastError"`
}

type shortcutField struct{ key, label, id string }

func shortcutFields(module string) []shortcutField {
	switch module {
	case "application":
		return []shortcutField{{"openShortcut", "shortcuts.open", "open-winone"}}
	case "capture":
		return []shortcutField{{"captureShortcut", "shortcuts.capture", "capture"}, {"ocrShortcut", "shortcuts.ocr", "ocr"}, {"translateShortcut", "shortcuts.translate", "translate"}}
	case "quick-assistant":
		return []shortcutField{{"quickAssistantShortcut", "shortcuts.quickAssistant", "quick-assistant"}}
	case "selection":
		return []shortcutField{{"selectionShortcut", "shortcuts.selection", "selection"}}
	}
	return []shortcutField{{"pinShortcut", "shortcuts.pin", "pin"}}
}

// shortcut reads one field; unset optional shortcuts show their defaults,
// as the Host applies them.
func shortcut(s *settings.Settings, key string) string {
	value := func(p *string, fallback string) string {
		if p == nil {
			return fallback
		}
		return *p
	}
	switch key {
	case "openShortcut":
		return value(s.OpenShortcut, "Alt+A")
	case "captureShortcut":
		return s.CaptureShortcut
	case "ocrShortcut":
		return value(s.OCRShortcut, "Shift+C")
	case "translateShortcut":
		return value(s.TranslateShortcut, "Alt+Shift+A")
	case "quickAssistantShortcut":
		return value(s.QuickAssistantShortcut, "")
	case "selectionShortcut":
		return value(s.SelectionShortcut, "")
	}
	return s.PinShortcut
}

func setShortcut(s *settings.Settings, key, text string) {
	value := text
	switch key {
	case "openShortcut":
		s.OpenShortcut = &value
	case "captureShortcut":
		s.CaptureShortcut = value
	case "ocrShortcut":
		s.OCRShortcut = &value
	case "translateShortcut":
		s.TranslateShortcut = &value
	case "quickAssistantShortcut":
		s.QuickAssistantShortcut = &value
	case "selectionShortcut":
		s.SelectionShortcut = &value
	default:
		s.PinShortcut = value
	}
}

// NewShortcuts makes the editor and reads the shortcuts.
func NewShortcuts(env *core.Env, module string) *Shortcuts {
	s := &Shortcuts{env: env, module: module, recorders: map[string]*ShortcutRecorder{}}
	for _, field := range shortcutFields(module) {
		s.recorders[field.key] = NewShortcutRecorder(env)
	}
	s.Reload()
	return s
}

// Dirty reports an edit not saved yet.
func (s *Shortcuts) Dirty() bool {
	if s.draft == nil || s.saved == nil {
		return false
	}
	for _, field := range shortcutFields(s.module) {
		if shortcut(s.draft, field.key) != shortcut(s.saved, field.key) {
			return true
		}
	}
	return false
}

func (s *Shortcuts) recording() bool {
	for _, r := range s.recorders {
		if r.Recording() {
			return true
		}
	}
	return false
}

// Report records the editor's activity; call it every frame, whether or
// not the editor shows, as its draft lives on.
func (s *Shortcuts) Report() {
	recording := s.recording()
	if s.wasRecording && !recording {
		s.refreshStatus()
	}
	s.wasRecording = recording
	s.env.Activity.Report("shortcuts:"+s.module, s.Dirty(), s.working || recording, s.Reload)
}

// Reload reads the settings and the registration state again, dropping
// the draft.
func (s *Shortcuts) Reload() {
	s.loading, s.err = true, ""
	type result struct {
		settings settings.Settings
		status   shortcutStatus
	}
	core.Go(s.env, func() (result, error) {
		var r result
		if err := s.env.Host.CallInto("snapshot", nil, &struct {
			Settings *settings.Settings `json:"settings"`
		}{&r.settings}); err != nil {
			return r, err
		}
		err := s.env.Host.CallInto("shortcuts.snapshot", nil, &r.status)
		return r, err
	}, func(r result, err error) {
		s.loading = false
		if err != nil {
			s.err = err.Error()
			return
		}
		// The editor shows, and a save from it stores, the defaults of
		// shortcuts never set, as the React window did.
		defaults := map[**string]string{&r.settings.OpenShortcut: "Alt+A", &r.settings.OCRShortcut: "Shift+C", &r.settings.TranslateShortcut: "Alt+Shift+A", &r.settings.QuickAssistantShortcut: ""}
		for field, value := range defaults {
			if *field == nil {
				*field = &value
			}
		}
		draft, saved := r.settings, r.settings
		s.draft, s.saved, s.status = &draft, &saved, &r.status
	})
}

func (s *Shortcuts) refreshStatus() {
	core.Go(s.env, func() (shortcutStatus, error) {
		var status shortcutStatus
		err := s.env.Host.CallInto("shortcuts.snapshot", nil, &status)
		return status, err
	}, func(status shortcutStatus, err error) {
		if err != nil {
			s.status, s.err = nil, err.Error()
			return
		}
		s.status = &status
	})
}

// Save stores the draft. A later status read can fail on its own; the save
// is reported first and never sent again.
func (s *Shortcuts) Save(c *ui.Context) {
	if s.draft == nil || s.working {
		return
	}
	s.working, s.err = true, ""
	draft := *s.draft
	core.Go(s.env, func() (settings.Settings, error) {
		var saved settings.Settings
		err := s.env.Host.CallCompletingInto("settings.save", draft, &saved)
		return saved, err
	}, func(saved settings.Settings, err error) {
		s.working = false
		if err != nil {
			s.err = err.Error()
			return
		}
		next, kept := saved, saved
		s.draft, s.saved = &next, &kept
		c.Toast(i18n.T("common.saved"))
		s.env.Refresh()
		s.refreshStatus()
	})
}

func (s *Shortcuts) binding(id string) (registered bool, failure string, known bool) {
	if s.status == nil {
		return false, "", false
	}
	for _, b := range s.status.Bindings {
		if b.ID == id {
			return b.Registered, b.Error, true
		}
	}
	return false, "", true
}

// View shows the editor as a group, and puts its Save and Undo on the
// rail. Call it only while it shows.
func (s *Shortcuts) View(c *ui.Context) {
	s.Report()
	busy := s.working || s.loading
	recording := s.recording()
	dirty := s.Dirty()
	fields := shortcutFields(s.module)
	registrationError := ""
	for _, field := range fields {
		if _, failure, _ := s.binding(field.id); failure != "" && registrationError == "" {
			registrationError = failure
		}
	}
	if s.draft != nil {
		s.env.Rail.Set(
			core.Action{Label: i18n.T("shortcuts.save"), Disabled: !dirty || recording, Busy: s.working, Run: func() { s.Save(c) }},
			core.Action{Disabled: !dirty || busy, Run: s.Reload},
		)
	}
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		if s.err != "" {
			kit.Notice(c, kit.Failure, "", s.err, func() {
				if !dirty && kit.Button(c, i18n.T("shortcuts.readAgain"), nil, s.loading).Disabled(busy).Clicked() {
					s.Reload()
				}
			})
		}
		if s.draft == nil {
			if s.loading {
				kit.Skeleton(c, 3)
			}
			return
		}
		kit.Group(c, icons.Keyboard, i18n.T("shortcuts.title"), i18n.T("shortcuts.recordHint"), func() {
			for _, field := range fields {
				status := i18n.T("shortcuts.unknown")
				registered, failure, known := s.binding(field.id)
				switch {
				case recording:
					status = i18n.T("shortcuts.pausedForRecording")
				case !known:
				case failure != "":
					status = failure
				case registered:
					status = i18n.T("settingsDesign.shortcutActive")
				default:
					status = i18n.T("shortcuts.disabled")
				}
				value := shortcut(s.draft, field.key)
				kit.Row(c.Key(field.key), i18n.T(field.label), status, func() {
					if s.recorders[field.key].View(c, i18n.T(field.label), &value, busy, false) {
						setShortcut(s.draft, field.key, value)
					}
				})
			}
		})
		if !dirty && registrationError != "" {
			ui.Row(c).Children(func() {
				if kit.Button(c, i18n.T("shortcuts.retry"), icons.RefreshCw, s.working).Disabled(busy).Clicked() {
					s.Save(c)
				}
			})
		}
		if s.status != nil {
			for _, field := range fields {
				if failure := s.status.ActionErrors[field.id]; failure != "" {
					kit.Notice(c, kit.Failure, "", i18n.T("shortcuts.failed", "error", failure), nil)
				}
			}
		}
	})
}

// Close ends a recording in progress when the page closes.
func (s *Shortcuts) Close() {
	for _, r := range s.recorders {
		r.Stop()
	}
}

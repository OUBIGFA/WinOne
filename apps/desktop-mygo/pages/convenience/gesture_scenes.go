package convenience

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
	"winone/modules/stroke"
)

// React source: features/mouse-gestures-scenes.tsx.

// sceneEditor names the action the editor dialog edits.
type sceneEditor struct{ packageID, actionID string }

// sceneAssignment is the assignment dialog's form.
type sceneAssignment struct {
	source, name, gesture, code string
	codeOpen                    bool
}

// scenes is the gestures by scene: the scenes with their switches, the
// chosen scene's actions with their gestures, and dialogs for the scene's
// settings, an action and a new assignment.
type scenes struct {
	t            *gesturesTab
	settingsOpen bool
	advanced     bool
	editor       *sceneEditor
	editorOpen   bool
	assign       *sceneAssignment
	assignOpen   bool
	err          string
	pathPicker   pathPicker
	rulePicker   pathPicker
	// classes is the scene's window classes as typed; classesScope is the
	// scene it was read from.
	classes      string
	classesScope string
	editName     string
	editCode     string
}

// interacting reports an open dialog, which holds the page guard and the
// tabs, as an edit in progress.
func (s *scenes) interacting() bool { return s.settingsOpen || s.editor != nil || s.assign != nil }

var (
	browserScene  = regexp.MustCompile(`(?i)browser|chrome|firefox|msedge|catsxp|浏览器`)
	terminalScene = regexp.MustCompile(`(?i)terminal|powershell|cmd\.exe|终端`)
	editorScene   = regexp.MustCompile(`(?i)code|editor|notepad|编辑器`)
	mediaScene    = regexp.MustCompile(`(?i)music|media|player|音乐|播放器`)
)

// sceneIcon keeps the built-in scenes' symbols; a custom application rule
// shows the generic window unless its name or path names a task.
func sceneIcon(scope stroke.Scope) *ui.SVG {
	builtIn := map[string]*ui.SVG{"explorer-folders": icons.FolderOpen, "global": icons.Monitor, "media": icons.Music,
		"windows": icons.AppWindow, "tabs": icons.PanelsTopLeft, "browser": icons.Globe, "applications": icons.AppWindow, "keyboard": icons.Keyboard}
	if icon := builtIn[scope.ID]; icon != nil {
		return icon
	}
	if scope.Kind == "explorer-folders" {
		return icons.FolderOpen
	}
	identity := strings.Join([]string{scope.ID, scope.Name, scope.ProcessPath, scope.Pattern}, " ")
	switch {
	case browserScene.MatchString(identity):
		return icons.Globe
	case terminalScene.MatchString(identity):
		return icons.Terminal
	case editorScene.MatchString(identity):
		return icons.CodeXml
	case mediaScene.MatchString(identity):
		return icons.Music
	}
	return icons.AppWindow
}

// gestureOptions are the custom trails, then Stroke's seven button and
// wheel gestures.
func gestureOptions(p stroke.Preferences) []kit.SelectOption {
	var options []kit.SelectOption
	for _, g := range p.Gestures {
		options = append(options, kit.SelectOption{Value: g.ID, Label: g.Name})
	}
	for i := range 7 {
		options = append(options, kit.SelectOption{Value: "#" + strconv.Itoa(i), Label: i18n.T("stroke.special" + strconv.Itoa(i))})
	}
	return options
}

func (s *scenes) scope() (stroke.Scope, int, bool) {
	d := s.t.draft
	ordered := orderedScopes(d.Scopes)
	if len(ordered) == 0 {
		return stroke.Scope{}, -1, false
	}
	chosen := ordered[0]
	for _, scope := range ordered {
		if scope.ID == s.t.scopeID {
			chosen = scope
		}
	}
	index := slices.IndexFunc(d.Scopes, func(sc stroke.Scope) bool { return sc.ID == chosen.ID })
	return chosen, index, true
}

// apply takes a model edit, or shows why it failed.
func (s *scenes) apply(next stroke.Preferences, err error) bool {
	if err != nil {
		s.err = i18n.T("stroke." + err.Error())
		return false
	}
	*s.t.draft = next
	s.err = ""
	return true
}

func (s *scenes) updateAction(packageID, actionID string, patch func(*stroke.Action)) {
	scope, _, ok := s.scope()
	if !ok {
		return
	}
	next, id, err := updateSceneAction(*s.t.draft, scope.ID, packageID, actionID, patch)
	if s.apply(next, err) && s.editor != nil {
		s.editor.packageID = id
	}
}

func (s *scenes) addScope() {
	d := s.t.draft
	id := strokeID()
	d.Scopes = append(d.Scopes, stroke.Scope{ID: id, Name: i18n.T("stroke.newScope"), Kind: "application", Packages: []string{}})
	s.t.scopeID, s.settingsOpen, s.err = id, true, ""
}

type actionSource struct {
	value  string
	label  string
	action stroke.Action
}

func (s *scenes) sources() []actionSource {
	var sources []actionSource
	for _, pack := range s.t.draft.ActionPackages {
		for _, action := range pack.Actions {
			sources = append(sources, actionSource{value: pack.ID + "/" + action.ID, label: pack.Name + " · " + action.Name, action: action})
		}
	}
	return sources
}

func (s *scenes) openAssignment() {
	d := s.t.draft
	a := &sceneAssignment{source: "custom", name: i18n.T("stroke.newAction"), gesture: "#1"}
	if len(d.Gestures) > 0 {
		a.gesture = d.Gestures[0].ID
	}
	if sources := s.sources(); len(sources) > 0 {
		first := sources[0]
		a.source, a.name, a.gesture, a.code = first.value, first.action.Name, first.action.Gesture, first.action.Code
	}
	a.codeOpen = a.source == "custom"
	s.assign, s.assignOpen, s.err = a, true, ""
}

func (s *scenes) View(c *ui.Context, disabled bool) {
	t := s.t
	d := t.draft
	theme, pal := c.Theme(), kit.Colors(c)
	ordered := orderedScopes(d.Scopes)
	scope, scopeIndex, hasScope := s.scope()
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			if kit.Button(c, i18n.T("stroke.addScope"), icons.Plus, false).Disabled(disabled || len(d.Scopes) >= 64).Clicked() {
				s.addScope()
			}
		})
		if s.err != "" && !s.interacting() {
			kit.Notice(c, kit.Failure, "", s.err, nil)
		}
		// The scenes beside their content; one column in a narrow window.
		ui.Row(c).FillWidth().GapX(20).GapY(16).Wrap().AlignItems(ui.Start).Children(func() {
			ui.Column(c).Basis(220).Grow(0).Shrink(1).MinWidth(180).Gap(2).Role(ui.RoleGroup).Label(i18n.T("stroke.scopes")).
				Children(func() {
					for _, item := range ordered {
						index := slices.IndexFunc(d.Scopes, func(sc stroke.Scope) bool { return sc.ID == item.ID })
						selected := hasScope && item.ID == scope.ID
						row := ui.Row(c.Key(item.ID)).FillWidth().Gap(8).AlignItems(ui.Center).Padding(0, 8, 0, 0).Radius(theme.Radius)
						if selected {
							row.Background(theme.SurfaceHover)
						}
						row.Children(func() {
							b := ui.ButtonBase(c).Grow(1).MinWidth(0).MinHeight(44).Padding(10, 8).Gap(8).Justify(ui.Start).AlignItems(ui.Center).
								Radius(theme.Radius).Label(item.Name).Tooltip(item.Name).Checked(selected).Disabled(disabled)
							b.Children(func() {
								ui.Icon(c, sceneIcon(item)).FontSize(16).Shrink(0).TextColor(theme.TextMuted)
								ui.Text(c, item.Name).SingleLine().Ellipsis("…").MinWidth(0)
							})
							if b.Clicked() {
								t.scopeID, s.err = item.ID, ""
							}
							state := "stroke.sceneDisabled"
							if item.Enabled {
								state = "stroke.sceneEnabled"
							}
							ui.Switch(c, &d.Scopes[index].Enabled).Label(i18n.T("stroke.scopeEnabled", "name", item.Name)).
								Tooltip(i18n.T(state)).Disabled(disabled)
						})
					}
				})
			ui.Column(c).Basis(320).Grow(1).Shrink(1).MinWidth(0).Gap(12).Children(func() {
				if !hasScope {
					kit.Help(c, i18n.T("stroke.emptyScenes"))
					return
				}
				s.content(c, scope, disabled, pal.Hairline)
			})
		})
	})
	if hasScope {
		s.settingsDialog(c, scopeIndex, disabled)
		s.editorDialog(c, scope, disabled)
		s.assignDialog(c, scope, disabled)
	}
}

// content is the chosen scene: its actions with their gestures.
func (s *scenes) content(c *ui.Context, scope stroke.Scope, disabled bool, hairline ui.Color) {
	t := s.t
	d := t.draft
	entries := sceneActions(*d, scope)
	hint := "stroke.applicationSceneHint"
	switch scope.Kind {
	case "explorer-folders":
		hint = "stroke.explorerSceneHint"
	case "global":
		hint = "stroke.globalSceneHint"
	}
	kit.SectionHeading(c, nil, scope.Name, i18n.T(hint))
	ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		if kit.Button(c, i18n.T("stroke.assignGesture"), icons.Plus, false).Disabled(disabled).Clicked() {
			s.openAssignment()
		}
		if kit.GhostButton(c, i18n.T("stroke.sceneSettings"), icons.Settings, false).Disabled(disabled).Clicked() {
			s.settingsOpen, s.classesScope = true, ""
		}
		kit.Caption(c, i18n.T("stroke.sceneActionCount", "count", len(entries)))
	})
	kit.Help(c, i18n.T("stroke.drawInstruction", "button", i18n.T("stroke."+d.TriggerButton)))
	options := gestureOptions(*d)
	ui.Column(c).FillWidth().BorderWidth(1, 0, 0, 0).BorderColor(hairline).Role(ui.RoleList).Label(i18n.T("stroke.sceneGestures")).
		Children(func() {
			for _, entry := range entries {
				pack, action := entry.pack, entry.action
				ui.Row(c.Key(pack.ID+"/"+action.ID)).FillWidth().Padding(14, 0).GapX(14).GapY(10).Wrap().AlignItems(ui.Center).
					BorderWidth(0, 0, 1, 0).BorderColor(hairline).Role(ui.RoleListItem).Label(i18n.T("stroke.sceneAction", "name", action.Name)).
					Children(func() {
						ui.Row(c).Size(64, 48).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
							if g, ok := findGesture(*d, action.Gesture); ok {
								gestureThumbnail(c, g, d.Pen.Color)
							} else {
								ui.Icon(c, icons.Mouse).FontSize(24).TextColor(c.Theme().TextMuted)
							}
						})
						ui.Column(c).Basis(160).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
							kit.Strong(c, action.Name)
							kit.Help(c, pack.Name)
							if entry.overriddenBy != "" {
								kit.Help(c, i18n.T("stroke.overriddenAction", "name", entry.overriddenBy))
							}
						})
						gesture := action.Gesture
						changed, trigger := kit.Select(c, &gesture, options)
						trigger.Label(i18n.T("stroke.gestureFor", "name", action.Name)).Disabled(disabled)
						if changed {
							s.updateAction(pack.ID, action.ID, func(a *stroke.Action) { a.Gesture = gesture })
						}
						ui.Row(c).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
							on := action.Enabled
							if ui.Switch(c, &on).Label(i18n.T("stroke.enableAction", "name", action.Name)).Disabled(disabled).Changed() {
								s.updateAction(pack.ID, action.ID, func(a *stroke.Action) { a.Enabled = on })
							}
							label := i18n.T("stroke.editNamedAction", "name", action.Name)
							if kit.IconButton(c, icons.CodeXml, label).Disabled(disabled).Clicked() {
								s.editor, s.editorOpen, s.err = &sceneEditor{pack.ID, action.ID}, true, ""
								s.editName, s.editCode = action.Name, action.Code
							}
						})
					})
			}
			if len(entries) == 0 {
				ui.Column(c).Padding(16, 0).Children(func() { kit.Help(c, i18n.T("stroke.emptyScene")) })
			}
		})
	kit.Help(c, i18n.T("stroke.sceneEditHint"))
}

func findGesture(p stroke.Preferences, id string) (stroke.Gesture, bool) {
	for _, g := range p.Gestures {
		if g.ID == id {
			return g, true
		}
	}
	return stroke.Gesture{}, false
}

// splitLines keeps blank lines while typing; saving drops them.
func splitLines(text string) []string { return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") }

// settingsDialog edits the chosen scene: its name, what it matches and
// the packages it uses.
func (s *scenes) settingsDialog(c *ui.Context, index int, disabled bool) {
	if !s.settingsOpen || index < 0 {
		s.settingsOpen = false
		return
	}
	t := s.t
	d := t.draft
	scope := &d.Scopes[index]
	if s.classesScope != scope.ID {
		s.classesScope, s.classes = scope.ID, strings.Join(scope.WindowClasses, "\n")
	}
	s.pathPicker.env, s.rulePicker.env = t.env, t.env
	kit.Dialog(c, &s.settingsOpen, kit.DialogOptions{
		Title: i18n.T("stroke.sceneSettings"), Message: i18n.T("stroke.sceneSettingsHint"), Width: 640,
		Confirm: i18n.T("stroke.doneEditing"),
		Body: func() {
			kit.Stack(c, i18n.T("stroke.scopeName"), "", func() {
				ui.TextInput(c, &scope.Name).FillWidth().Label(i18n.T("stroke.scopeName")).Disabled(disabled)
			})
			kit.Row(c, i18n.T("stroke.scopeKind"), "", func() {
				kit.Segmented(c, &scope.Kind, []kit.SelectOption{
					{Value: "explorer-folders", Label: i18n.T("stroke.explorerFolders")},
					{Value: "application", Label: i18n.T("stroke.application")},
					{Value: "global", Label: i18n.T("stroke.global")},
				}, i18n.T("stroke.scopeKind"))
			})
			if scope.Kind == "application" {
				kit.Stack(c, i18n.T("stroke.scopePath"), "", func() {
					in := ui.TextInput(c, &scope.ProcessPath).FillWidth().Font(kit.MonoFont).Label(i18n.T("stroke.scopePath")).Disabled(disabled)
					widgets.DropArea(c, t.env, in, widgets.DropFiles, false, disabled, func(paths []string, err error) {
						if err != nil {
							s.err = err.Error()
							return
						}
						if i := slices.IndexFunc(d.Scopes, func(sc stroke.Scope) bool { return sc.ID == s.classesScope }); i >= 0 {
							d.Scopes[i].ProcessPath = paths[0]
						}
					})
					s.pathPicker.View(c, disabled, func(target strokeWindow) {
						if i := slices.IndexFunc(d.Scopes, func(sc stroke.Scope) bool { return sc.ID == s.classesScope }); i >= 0 {
							d.Scopes[i].ProcessPath = target.ProcessPath
						}
					})
				})
			}
			ui.Collapsible(c, i18n.T("stroke.advancedScope"), &s.advanced, func() {
				ui.Column(c).FillWidth().Gap(12).Padding(8, 0, 0, 0).Children(func() {
					if scope.Kind == "application" {
						kit.Stack(c, i18n.T("stroke.scopePattern"), i18n.T("stroke.pathRuleHint"), func() {
							area := ui.TextArea(c, &scope.Pattern).FillWidth().MinHeight(72).Font(kit.MonoFont).
								Label(i18n.T("stroke.scopePattern")).Disabled(disabled)
							widgets.DropArea(c, t.env, area, widgets.DropFiles, true, disabled, func(paths []string, err error) {
								if err != nil {
									s.err = err.Error()
									return
								}
								s.appendPatterns(paths, false)
							})
							s.rulePicker.View(c, disabled, func(target strokeWindow) { s.appendPatterns([]string{target.ProcessPath}, true) })
						})
					}
					kit.Stack(c, i18n.T("stroke.windowClasses"), "", func() {
						if ui.TextArea(c, &s.classes).FillWidth().MinHeight(72).Font(kit.MonoFont).
							Label(i18n.T("stroke.windowClasses")).Disabled(disabled).Changed() {
							scope.WindowClasses = splitLines(s.classes)
						}
					})
					kit.Stack(c, i18n.T("stroke.scopePackages"), i18n.T("stroke.packageOrder"), func() {
						for _, pack := range d.ActionPackages {
							on := slices.Contains(scope.Packages, pack.ID)
							if ui.Checkbox(c.Key(pack.ID), &on, pack.Name).Disabled(disabled).Changed() {
								if on {
									scope.Packages = append(slices.Clone(scope.Packages), pack.ID)
								} else {
									scope.Packages = slices.DeleteFunc(slices.Clone(scope.Packages), func(id string) bool { return id == pack.ID })
								}
							}
						}
					})
				})
			}).FontSize(kit.HelpSize(c))
			if s.err != "" {
				kit.Notice(c, kit.Failure, "", s.err, nil)
			}
		},
		Actions: func() {
			if kit.GhostButton(c, i18n.T("stroke.removeScope"), icons.Trash2, false).Disabled(disabled).Clicked() {
				removed := scope.ID
				ordered := orderedScopes(d.Scopes)
				d.Scopes = slices.DeleteFunc(slices.Clone(d.Scopes), func(sc stroke.Scope) bool { return sc.ID == removed })
				t.scopeID = ""
				for _, sc := range ordered {
					if sc.ID != removed {
						t.scopeID = sc.ID
						break
					}
				}
				s.settingsOpen = false
			}
		},
	})
}

// appendPatterns adds programs to the scene's path rules, each matched
// exactly; picking a window also clears the full path, which would match
// first.
func (s *scenes) appendPatterns(paths []string, clearPath bool) {
	d := s.t.draft
	i := slices.IndexFunc(d.Scopes, func(sc stroke.Scope) bool { return sc.ID == s.classesScope })
	if i < 0 {
		return
	}
	scope := &d.Scopes[i]
	var parts []string
	if scope.Pattern != "" {
		parts = append(parts, scope.Pattern)
	}
	for _, path := range paths {
		parts = append(parts, escapedProcessPattern(path))
	}
	scope.Pattern = strings.Join(parts, "\n")
	if clearPath {
		scope.ProcessPath = ""
	}
}

// editorDialog edits one of the scene's actions; an edit to a package
// other scenes share copies it for this scene first.
func (s *scenes) editorDialog(c *ui.Context, scope stroke.Scope, disabled bool) {
	if s.editor == nil {
		return
	}
	t := s.t
	d := t.draft
	var action stroke.Action
	found := false
	for _, pack := range d.ActionPackages {
		if pack.ID == s.editor.packageID {
			for _, a := range pack.Actions {
				if a.ID == s.editor.actionID {
					action, found = a, true
				}
			}
		}
	}
	if !found || !s.editorOpen {
		s.editor, s.editorOpen = nil, false
		return
	}
	editor := *s.editor
	kit.Dialog(c, &s.editorOpen, kit.DialogOptions{
		Title: i18n.T("stroke.editAction"), Message: i18n.T("stroke.sceneEditHint"), Width: 720,
		Confirm: i18n.T("stroke.doneEditing"),
		Body: func() {
			kit.Stack(c, i18n.T("stroke.actionName"), "", func() {
				if ui.TextInput(c, &s.editName).FillWidth().Label(i18n.T("stroke.actionName")).Disabled(disabled).Changed() {
					name := s.editName
					s.updateAction(editor.packageID, editor.actionID, func(a *stroke.Action) { a.Name = name })
				}
			})
			kit.Stack(c, i18n.T("stroke.actionGesture"), "", func() {
				gesture := action.Gesture
				changed, trigger := kit.Select(c, &gesture, gestureOptions(*d))
				trigger.Label(i18n.T("stroke.actionGesture")).Disabled(disabled)
				if changed {
					s.updateAction(editor.packageID, editor.actionID, func(a *stroke.Action) { a.Gesture = gesture })
				}
				if _, custom := findGesture(*d, action.Gesture); custom {
					ui.Row(c).Children(func() {
						if kit.GhostButton(c, i18n.T("stroke.manageTrail"), nil, false).Disabled(disabled).Clicked() {
							t.gestureID, t.tab = action.Gesture, gesturesTrails
							s.editor, s.editorOpen = nil, false
						}
					})
				}
			})
			kit.Stack(c, i18n.T("stroke.actionCode"), "", func() {
				if ui.TextArea(c, &s.editCode).FillWidth().MinHeight(256).Font(kit.MonoFont).Label(i18n.T("stroke.actionCode")).
					Disabled(disabled).Changed() {
					code := s.editCode
					s.updateAction(editor.packageID, editor.actionID, func(a *stroke.Action) { a.Code = code })
				}
				kit.Help(c, i18n.T("stroke.markHint"))
				kit.Help(c, i18n.T("stroke.scriptHint"))
			})
			if s.err != "" {
				kit.Notice(c, kit.Failure, "", s.err, nil)
			}
		},
		Actions: func() {
			if kit.GhostButton(c, i18n.T("stroke.removeSceneAction"), icons.Trash2, false).Disabled(disabled).Clicked() {
				next, err := removeSceneAction(*d, scope.ID, editor.packageID, editor.actionID)
				if s.apply(next, err) {
					s.editor, s.editorOpen = nil, false
				}
			}
		},
	})
	if !s.editorOpen {
		s.editor = nil
	}
}

// assignDialog adds an action to the scene: a copy of an existing one,
// or a script of the user's, with its gesture.
func (s *scenes) assignDialog(c *ui.Context, scope stroke.Scope, disabled bool) {
	if s.assign == nil {
		return
	}
	if !s.assignOpen {
		s.assign = nil
		return
	}
	t := s.t
	d := t.draft
	a := s.assign
	sources := s.sources()
	options := make([]kit.SelectOption, 0, len(sources)+1)
	for _, source := range sources {
		options = append(options, kit.SelectOption{Value: source.value, Label: source.label})
	}
	options = append(options, kit.SelectOption{Value: "custom", Label: i18n.T("stroke.customAction")})
	add := kit.Dialog(c, &s.assignOpen, kit.DialogOptions{
		Title: i18n.T("stroke.assignGesture"), Message: i18n.T("stroke.assignGestureHint"), Width: 640,
		Cancel: i18n.T("stroke.cancelAssignment"), Confirm: i18n.T("stroke.addToScene"),
		ConfirmDisabled: disabled || strings.TrimSpace(a.name) == "",
		Body: func() {
			kit.Stack(c, i18n.T("stroke.chooseAction"), "", func() {
				changed, trigger := kit.Select(c, &a.source, options)
				trigger.Label(i18n.T("stroke.chooseAction")).Disabled(disabled)
				if changed {
					a.name, a.code = i18n.T("stroke.newAction"), ""
					for _, source := range sources {
						if source.value == a.source {
							a.name, a.gesture, a.code = source.action.Name, source.action.Gesture, source.action.Code
						}
					}
					a.codeOpen = a.source == "custom"
				}
			})
			kit.Stack(c, i18n.T("stroke.actionName"), "", func() {
				ui.TextInput(c, &a.name).FillWidth().Label(i18n.T("stroke.actionName")).Disabled(disabled)
			})
			kit.Stack(c, i18n.T("stroke.actionGesture"), "", func() {
				_, trigger := kit.Select(c, &a.gesture, gestureOptions(*d))
				trigger.Label(i18n.T("stroke.actionGesture")).Disabled(disabled)
				if g, ok := findGesture(*d, a.gesture); ok {
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						gestureThumbnail(c, g, d.Pen.Color)
						kit.Help(c, i18n.T("stroke.assignmentPreview", "button", i18n.T("stroke."+d.TriggerButton), "gesture", g.Name, "action", a.name)).Shrink(1)
					})
				}
			})
			ui.Collapsible(c, i18n.T("stroke.actionCode"), &a.codeOpen, func() {
				ui.Column(c).FillWidth().Gap(8).Padding(8, 0, 0, 0).Children(func() {
					ui.TextArea(c, &a.code).FillWidth().MinHeight(192).Font(kit.MonoFont).Label(i18n.T("stroke.actionCode")).Disabled(disabled)
					kit.Help(c, i18n.T("stroke.scriptHint"))
				})
			}).FontSize(kit.HelpSize(c))
			if s.err != "" {
				kit.Notice(c, kit.Failure, "", s.err, nil)
			}
		},
	})
	if add {
		next, err := addSceneAction(*d, scope.ID, stroke.Action{Name: a.name, Gesture: a.gesture, Code: a.code, Enabled: true})
		if s.apply(next, err) {
			s.assign, s.assignOpen = nil, false
		}
	}
}

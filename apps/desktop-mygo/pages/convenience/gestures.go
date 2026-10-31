package convenience

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
	"winone/modules/stroke"
)

// The gesture tab's own tabs.
const (
	gesturesScenes = iota
	gesturesTrails
	gesturesPen
	gesturesFilters
	gesturesAdvanced
)

// The advanced tab's own tabs.
const (
	advancedActions = iota
	advancedCompile
	advancedThreads
)

// gesturesTab is mouse gestures: Stroke's scenes, trails, pen, filters,
// action packages, compiler and execution settings, edited as one draft
// that the Host commits and applies to the worker as a whole.
//
// React source: features/mouse-gestures.tsx and its parts.
type gesturesTab struct {
	env      *core.Env
	modules  *widgets.Modules
	draft    *stroke.Preferences
	saved    *stroke.Preferences
	busy     bool
	loading  bool
	closed   bool
	err      string
	tab      int
	advanced int
	// pack and action are the chosen package and action of the package
	// editor; action is "" when the package itself is chosen.
	pack, action string
	gestureID    string
	scopeID      string
	scenes       scenes
	recorder     *gestureRecorder
	filterPicker pathPicker
	// The lists edited as text, as typed: one item per line, blank lines
	// kept until saved.
	filters, namespaces, assemblies string
	// opened keeps the package tree's open packages.
	closedPacks map[string]bool
	apiOpen     bool
}

func newGesturesTab(env *core.Env) *gesturesTab {
	t := &gesturesTab{env: env, modules: widgets.NewModules(env), loading: true, closedPacks: map[string]bool{}}
	t.scenes.t = t
	t.filterPicker.env = env
	core.Go(env, func() (stroke.Preferences, error) {
		var p stroke.Preferences
		err := env.Host.CallCompletingInto("stroke.settings.get", nil, &p)
		return p, err
	}, func(p stroke.Preferences, err error) {
		t.loading = false
		if t.closed {
			return
		}
		if err != nil {
			t.err = err.Error()
			return
		}
		t.load(p)
		if len(p.ActionPackages) > 0 {
			t.pack = p.ActionPackages[0].ID
			if len(p.ActionPackages[0].Actions) > 0 {
				t.action = p.ActionPackages[0].Actions[0].ID
			}
		}
		if len(p.Gestures) > 0 {
			t.gestureID = p.Gestures[0].ID
		}
		if ordered := orderedScopes(p.Scopes); len(ordered) > 0 {
			t.scopeID = ordered[0].ID
		}
	})
	return t
}

// load shows preferences the Host returned as the saved state.
func (t *gesturesTab) load(p stroke.Preferences) {
	draft, saved := cloneStroke(p), cloneStroke(p)
	t.draft, t.saved = &draft, &saved
	t.syncText()
}

// syncText shows the draft's lists in their text areas.
func (t *gesturesTab) syncText() {
	t.filters = strings.Join(t.draft.Filters, "\n")
	t.namespaces = strings.Join(t.draft.Namespaces, "\n")
	t.assemblies = strings.Join(t.draft.Assemblies, "\n")
	t.scenes.classesScope = ""
}

func (t *gesturesTab) dirty() bool {
	if t.draft == nil || t.saved == nil {
		return false
	}
	a, _ := json.Marshal(t.draft)
	b, _ := json.Marshal(t.saved)
	return string(a) != string(b)
}

func (t *gesturesTab) discard() {
	if t.saved != nil {
		draft := cloneStroke(*t.saved)
		t.draft = &draft
		t.syncText()
	}
	t.err = ""
}

// committedLines is a list as saved: lines trimmed, blank ones dropped.
func committedLines(lines []string) []string {
	out := []string{}
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func (t *gesturesTab) save() {
	if t.draft == nil || t.busy {
		return
	}
	value := cloneStroke(*t.draft)
	value.Namespaces, value.Assemblies, value.Filters = committedLines(value.Namespaces), committedLines(value.Assemblies), committedLines(value.Filters)
	for i := range value.Scopes {
		if value.Scopes[i].WindowClasses != nil {
			value.Scopes[i].WindowClasses = committedLines(value.Scopes[i].WindowClasses)
		}
	}
	t.busy, t.err = true, ""
	core.Go(t.env, func() (stroke.Preferences, error) {
		var saved stroke.Preferences
		err := t.env.Host.CallCompletingInto("stroke.settings.save", value, &saved)
		return saved, err
	}, func(saved stroke.Preferences, err error) {
		t.busy = false
		if t.closed {
			return
		}
		if err != nil {
			t.err = err.Error()
			return
		}
		t.load(saved)
		t.env.Toast(i18n.T("stroke.saved"))
	})
}

func (t *gesturesTab) Close() {
	t.closed = true
	if t.recorder != nil {
		t.recorder.close()
	}
	t.env.Activity.Report("mouse-gestures", false, false, nil)
	t.env.Activity.Report("module-availability:stroke", false, false, nil)
}

func (t *gesturesTab) View(c *ui.Context, locked bool) {
	recording := t.recorder != nil && t.recorder.open
	interacting := t.scenes.interacting()
	t.env.Activity.Report("mouse-gestures", t.dirty(), t.busy || recording || interacting, t.discard)
	t.modules.Switch(c, "stroke", locked)
	// Recording and the scripts' own worker need the module on.
	disabled := moduleDisabled(t.env, "stroke")
	busy := t.busy || t.loading
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		kit.Help(c, i18n.T("stroke.scopeHint"))
		if t.loading {
			kit.Status(c, i18n.T("stroke.loading"))
		}
		if t.draft == nil {
			if t.err != "" {
				kit.Notice(c, kit.Failure, "", t.err, nil)
			}
			return
		}
		dirty := t.dirty()
		t.env.Rail.Set(
			core.Action{Label: i18n.T("stroke.save"), Disabled: busy || !dirty, Busy: t.busy, Run: t.save},
			core.Action{Disabled: busy || !dirty, Run: t.discard},
		)
		tabs := []kit.Tab{
			{Label: i18n.T("stroke.scopes"), Icon: icons.ScanLine},
			{Label: i18n.T("stroke.gestures"), Icon: icons.MousePointer2},
			{Label: i18n.T("stroke.pen"), Icon: icons.Paintbrush},
			{Label: i18n.T("stroke.filters"), Icon: icons.ShieldOff},
			{Label: i18n.T("stroke.advancedSettings"), Icon: icons.Settings},
		}
		ui.Column(c).FillWidth().Disabled(t.busy || recording || interacting).Children(func() {
			kit.Tabs(c, &t.tab, tabs, t.env.RequestChange)
		})
		if t.err != "" {
			kit.Notice(c, kit.Failure, "", t.err, nil)
		}
		switch t.tab {
		case gesturesScenes:
			t.scenes.View(c, busy)
		case gesturesTrails:
			t.trails(c, busy, disabled)
		case gesturesPen:
			t.pen(c, busy)
		case gesturesFilters:
			t.filtersView(c, busy)
		default:
			t.advancedView(c, busy)
		}
	})
	if t.recorder != nil {
		t.recorder.View(c)
		if !t.recorder.open {
			t.recorder = nil
		}
	}
}

func (t *gesturesTab) addGesture() {
	id := strokeID()
	t.draft.Gestures = append(t.draft.Gestures, stroke.Gesture{ID: id, Name: i18n.T("stroke.newGesture")})
	t.gestureID = id
}

// removeGesture removes a trail with the actions that use it.
func (t *gesturesTab) removeGesture(id string) {
	d := t.draft
	d.Gestures = slices.DeleteFunc(slices.Clone(d.Gestures), func(g stroke.Gesture) bool { return g.ID == id })
	packages := make([]stroke.ActionPackage, len(d.ActionPackages))
	for i, pack := range d.ActionPackages {
		pack.Actions = slices.DeleteFunc(slices.Clone(pack.Actions), func(a stroke.Action) bool { return a.Gesture == id })
		packages[i] = pack
	}
	d.ActionPackages = packages
	t.gestureID = ""
	if len(d.Gestures) > 0 {
		t.gestureID = d.Gestures[0].ID
	}
}

// trails lists the gestures' trails and edits the chosen one.
func (t *gesturesTab) trails(c *ui.Context, busy, disabled bool) {
	d := t.draft
	theme := c.Theme()
	index := slices.IndexFunc(d.Gestures, func(g stroke.Gesture) bool { return g.ID == t.gestureID })
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c, i18n.T("stroke.addGesture"), icons.Plus, false).Disabled(busy).Clicked() {
				t.addGesture()
			}
			if index >= 0 {
				b := kit.GhostButton(c, i18n.T("stroke.removeGesture"), icons.Trash2, false).Disabled(busy).Tooltip(i18n.T("stroke.deleteReferences"))
				if b.Clicked() {
					t.removeGesture(t.gestureID)
					index = -1
				}
			}
		})
		ui.Row(c).FillWidth().GapX(20).GapY(16).Wrap().AlignItems(ui.Start).Children(func() {
			ui.Scroll(c).Basis(192).Grow(0).Shrink(1).MinWidth(160).MaxHeight(384).Gap(2).Role(ui.RoleGroup).
				Label(i18n.T("stroke.gestures")).Children(func() {
				for _, g := range d.Gestures {
					id := g.ID
					b := ui.ButtonBase(c.Key(id)).FillWidth().Padding(8, 12).Justify(ui.Start).Radius(theme.Radius).Label(g.Name).Checked(id == t.gestureID)
					if id == t.gestureID || b.Hovered() {
						b.Background(theme.SurfaceHover)
					}
					b.Children(func() { ui.Text(c, g.Name).SingleLine().Ellipsis("…").MinWidth(0) })
					if b.Clicked() {
						t.gestureID = id
					}
					b.ContextMenu(func(m *ui.Menu) {
						if m.Item(i18n.T("stroke.addGesture")).Disabled(busy).Chosen() {
							t.addGesture()
						}
						if m.Item(i18n.T("stroke.removeGesture")).Disabled(busy).Chosen() {
							t.removeGesture(id)
						}
					})
				}
			})
			if index < 0 {
				return
			}
			g := &d.Gestures[index]
			ui.Column(c).Basis(320).Grow(1).Shrink(1).MinWidth(0).Gap(16).Children(func() {
				kit.Stack(c, i18n.T("stroke.gestureName"), "", func() {
					if ui.TextInput(c, &g.Name).FillWidth().Label(i18n.T("stroke.gestureName")).Disabled(busy).Changed() {
						// "#" starts Stroke's built-in gesture IDs.
						g.Name = strings.TrimLeft(g.Name, "#")
					}
				})
				record := func() { t.record(*g) }
				reset := func() { g.Points, g.Vectors = nil, nil }
				gesturePreview(c, *g, d.Pen.Color, busy || disabled, record, func() {
					if !busy {
						reset()
					}
				})
				ui.Row(c).Gap(8).Wrap().Children(func() {
					if kit.Button(c, i18n.T("stroke.record"), icons.MousePointer2, false).Disabled(busy || disabled).Clicked() {
						record()
					}
					if kit.GhostButton(c, i18n.T("stroke.resetGesture"), icons.RotateCcw, false).Disabled(busy).Clicked() {
						reset()
					}
				})
				hint := "stroke.gestureHint"
				if disabled {
					hint = "stroke.recordDisabled"
				}
				kit.Help(c, i18n.T(hint))
				kit.Help(c, i18n.T("stroke.blankAllowed"))
			})
		})
	})
}

// record opens the recorder for a gesture; its result replaces the
// gesture's trail in the draft.
func (t *gesturesTab) record(g stroke.Gesture) {
	id := g.ID
	t.recorder = newGestureRecorder(t.env, g, *t.draft, func(points []stroke.Point, vectors [][]int) {
		if i := slices.IndexFunc(t.draft.Gestures, func(g stroke.Gesture) bool { return g.ID == id }); i >= 0 {
			t.draft.Gestures[i].Points, t.draft.Gestures[i].Vectors = points, vectors
		}
	})
}

// pen edits the trigger button and the trail drawn while gesturing.
func (t *gesturesTab) pen(c *ui.Context, busy bool) {
	d := t.draft
	kit.Group(c, icons.MousePointer2, i18n.T("stroke.trigger"), "", func() {
		kit.Row(c, i18n.T("stroke.trigger"), "", func() {
			options := []kit.SelectOption{}
			for _, button := range []string{"middle", "left", "right", "x1", "x2"} {
				options = append(options, kit.SelectOption{Value: button, Label: i18n.T("stroke." + button)})
			}
			ui.Column(c).Disabled(busy).Children(func() { kit.Segmented(c, &d.TriggerButton, options, i18n.T("stroke.trigger")) })
		})
	})
	kit.Group(c, icons.Paintbrush, i18n.T("stroke.pen"), i18n.T("stroke.penHint"), func() {
		// A pen saved before the switch existed is on.
		on := d.Pen.IsEnabled()
		if kit.Toggle(c, &on, i18n.T("stroke.penEnabled"), "").Disabled(busy).Changed() {
			d.Pen.Enabled = &on
		}
		kit.Row(c, i18n.T("stroke.penColor"), "", func() {
			color := strokeColor(d.Pen.Color, 1)
			if ui.ColorWell(c, &color).Label(i18n.T("stroke.penColor")).Disabled(busy).Changed() {
				d.Pen.Color = fmt.Sprintf("#%02x%02x%02x", color.R, color.G, color.B)
			}
		})
		kit.Row(c, i18n.T("stroke.penOpacity"), "", func() {
			// Stroke stores the opacity as a fraction; the field shows percent.
			opacity := math.Round(d.Pen.Opacity * 100)
			kit.NumberField(c, &opacity, 0, 100, 1, "%", i18n.T("stroke.penOpacity")).Disabled(busy)
			if opacity != math.Round(d.Pen.Opacity*100) {
				d.Pen.Opacity = opacity / 100
			}
		})
		kit.Row(c, i18n.T("stroke.penWidth"), "", func() {
			width := float64(d.Pen.Thickness)
			kit.NumberField(c, &width, 0, 10, 1, "", i18n.T("stroke.penWidth")).Disabled(busy)
			d.Pen.Thickness = int(width)
		})
	})
}

// filtersView edits the processes gestures leave alone.
func (t *gesturesTab) filtersView(c *ui.Context, busy bool) {
	d := t.draft
	ui.Column(c).FillWidth().Gap(12).Children(func() {
		kit.Help(c, i18n.T("stroke.excludeHint"))
		kit.Label(c, i18n.T("stroke.excludePatterns"))
		area := ui.TextArea(c, &t.filters).FillWidth().MinHeight(192).Font(kit.MonoFont).Label(i18n.T("stroke.excludePatterns")).Disabled(busy)
		if area.Changed() {
			d.Filters = splitLines(t.filters)
		}
		add := func(paths []string) {
			filters := slices.Clone(d.Filters)
			for _, path := range paths {
				if pattern := escapedProcessPattern(path); !slices.Contains(filters, pattern) {
					filters = append(filters, pattern)
				}
			}
			d.Filters = filters
			t.filters = strings.Join(filters, "\n")
		}
		widgets.DropArea(c, t.env, area, widgets.DropFiles, true, busy, func(paths []string, err error) {
			if err != nil {
				t.err = err.Error()
				return
			}
			add(paths)
		})
		t.filterPicker.View(c, busy, func(target strokeWindow) {
			d.Filters = append(slices.Clone(d.Filters), escapedProcessPattern(target.ProcessPath))
			t.filters = strings.Join(d.Filters, "\n")
		})
	})
}

func (t *gesturesTab) advancedView(c *ui.Context, busy bool) {
	kit.SubTabs(c, &t.advanced, []string{i18n.T("stroke.actions"), i18n.T("stroke.compile"), i18n.T("stroke.threads")}, t.env.RequestChange)
	switch t.advanced {
	case advancedActions:
		t.packagesView(c, busy)
	case advancedCompile:
		t.compileView(c, busy)
	default:
		t.threadsView(c, busy)
	}
}

func (t *gesturesTab) addPackage() {
	id := strokeID()
	t.draft.ActionPackages = append(t.draft.ActionPackages, stroke.ActionPackage{ID: id, Name: i18n.T("stroke.newPackage"), Actions: []stroke.Action{}})
	t.pack, t.action = id, ""
}

func (t *gesturesTab) removePackage(id string) {
	d := t.draft
	d.ActionPackages = slices.DeleteFunc(slices.Clone(d.ActionPackages), func(p stroke.ActionPackage) bool { return p.ID == id })
	scopes := make([]stroke.Scope, len(d.Scopes))
	for i, scope := range d.Scopes {
		scope.Packages = slices.DeleteFunc(slices.Clone(scope.Packages), func(p string) bool { return p == id })
		scopes[i] = scope
	}
	d.Scopes = scopes
	t.pack, t.action = "", ""
	if len(d.ActionPackages) > 0 {
		t.pack = d.ActionPackages[0].ID
	}
}

func (t *gesturesTab) packIndex(id string) int {
	return slices.IndexFunc(t.draft.ActionPackages, func(p stroke.ActionPackage) bool { return p.ID == id })
}

func (t *gesturesTab) addAction(packID string) {
	i := t.packIndex(packID)
	if i < 0 {
		return
	}
	gesture := "#1"
	if len(t.draft.Gestures) > 0 {
		gesture = t.draft.Gestures[0].ID
	}
	id := strokeID()
	pack := &t.draft.ActionPackages[i]
	pack.Actions = append(slices.Clone(pack.Actions), stroke.Action{ID: id, Name: i18n.T("stroke.newAction"), Gesture: gesture, Enabled: true})
	t.pack, t.action = packID, id
}

func (t *gesturesTab) removeAction(packID, actionID string) {
	if i := t.packIndex(packID); i >= 0 {
		pack := &t.draft.ActionPackages[i]
		pack.Actions = slices.DeleteFunc(slices.Clone(pack.Actions), func(a stroke.Action) bool { return a.ID == actionID })
	}
	t.pack, t.action = packID, ""
}

func (t *gesturesTab) movePackage(id string, offset int) {
	d := t.draft
	i := t.packIndex(id)
	if i < 0 || i+offset < 0 || i+offset >= len(d.ActionPackages) {
		return
	}
	packages := slices.Clone(d.ActionPackages)
	packages[i], packages[i+offset] = packages[i+offset], packages[i]
	d.ActionPackages = packages
}

// packageMenu is a package's or an action's context menu.
func (t *gesturesTab) packageMenu(m *ui.Menu, packID, actionID string, busy bool) {
	if m.Item(i18n.T("stroke.addPackage")).Disabled(busy).Chosen() {
		t.addPackage()
	}
	if m.Item(i18n.T("stroke.addAction")).Disabled(busy).Chosen() {
		t.addAction(packID)
	}
	if actionID != "" {
		if m.Item(i18n.T("stroke.removeAction")).Disabled(busy).Chosen() {
			t.removeAction(packID, actionID)
		}
	} else if m.Item(i18n.T("stroke.removePackage")).Disabled(busy).Chosen() {
		t.removePackage(packID)
	}
}

// packagesView is the action packages: a tree beside the chosen package
// or action's editor.
func (t *gesturesTab) packagesView(c *ui.Context, busy bool) {
	d := t.draft
	theme, pal := c.Theme(), kit.Colors(c)
	packIndex := t.packIndex(t.pack)
	actionIndex := -1
	if packIndex >= 0 {
		actionIndex = slices.IndexFunc(d.ActionPackages[packIndex].Actions, func(a stroke.Action) bool { return a.ID == t.action })
	}
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			if kit.Button(c, i18n.T("stroke.addPackage"), icons.FolderPlus, false).Disabled(busy).Clicked() {
				t.addPackage()
			}
			if packIndex >= 0 && kit.Button(c, i18n.T("stroke.addAction"), icons.Plus, false).Disabled(busy).Clicked() {
				t.addAction(t.pack)
			}
			kit.Help(c, i18n.T("stroke.packageOrder"))
		})
		ui.Row(c).FillWidth().GapX(20).GapY(16).Wrap().AlignItems(ui.Start).Children(func() {
			ui.Scroll(c).Basis(240).Grow(0).Shrink(1).MinWidth(180).MaxHeight(544).Padding(8).Radius(10).
				Border(1, pal.Hairline).Gap(4).Role(ui.RoleGroup).Label(i18n.T("stroke.actions")).Children(func() {
				for _, pack := range d.ActionPackages {
					packID := pack.ID
					open := !t.closedPacks[packID]
					ui.Column(c.Key(packID)).FillWidth().Gap(2).Children(func() {
						ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
							chevron := icons.ChevronRight
							if open {
								chevron = icons.ChevronDown
							}
							if kit.IconButton(c, chevron, pack.Name).Expanded(open).Clicked() {
								t.closedPacks[packID] = open
							}
							chosen := t.pack == packID && t.action == ""
							b := ui.ButtonBase(c).Grow(1).MinWidth(0).Padding(6, 4).Justify(ui.Start).Radius(6).Label(pack.Name).Tooltip(pack.Name).Checked(chosen)
							if chosen || b.Hovered() {
								b.Background(theme.SurfaceHover)
							}
							b.Children(func() { kit.Strong(c, pack.Name).SingleLine().Ellipsis("…").MinWidth(0).FontSize(kit.HelpSize(c)) })
							if b.Clicked() {
								t.pack, t.action = packID, ""
							}
							b.ContextMenu(func(m *ui.Menu) { t.packageMenu(m, packID, "", busy) })
						})
						if !open {
							return
						}
						ui.Column(c).FillWidth().PaddingStart(28).Gap(2).Children(func() {
							for _, action := range pack.Actions {
								actionID := action.ID
								chosen := t.pack == packID && t.action == actionID
								b := ui.ButtonBase(c.Key(actionID)).FillWidth().Padding(6, 8).Justify(ui.Start).Radius(6).Label(action.Name).Tooltip(action.Name).Checked(chosen)
								if chosen || b.Hovered() {
									b.Background(theme.SurfaceHover)
								}
								b.Children(func() { ui.Text(c, action.Name).FontSize(kit.HelpSize(c)).SingleLine().Ellipsis("…").MinWidth(0) })
								if b.Clicked() {
									t.pack, t.action = packID, actionID
								}
								b.ContextMenu(func(m *ui.Menu) { t.packageMenu(m, packID, actionID, busy) })
							}
						})
					})
				}
			})
			ui.Column(c).Basis(352).Grow(1).Shrink(1).MinWidth(0).Gap(16).Children(func() {
				switch {
				case packIndex < 0:
					kit.Help(c, i18n.T("stroke.emptyActions"))
				case actionIndex >= 0:
					t.actionEditor(c, packIndex, actionIndex, busy)
				default:
					t.packageEditor(c, packIndex, busy)
				}
			})
		})
	})
}

func (t *gesturesTab) actionEditor(c *ui.Context, packIndex, actionIndex int, busy bool) {
	d := t.draft
	pack := &d.ActionPackages[packIndex]
	action := &pack.Actions[actionIndex]
	kit.Stack(c, i18n.T("stroke.actionName"), "", func() {
		ui.TextInput(c, &action.Name).FillWidth().Label(i18n.T("stroke.actionName")).Disabled(busy)
	})
	kit.Toggle(c, &action.Enabled, i18n.T("stroke.actionEnabled"), "").Disabled(busy)
	kit.Stack(c, i18n.T("stroke.actionGesture"), "", func() {
		_, trigger := kit.Select(c, &action.Gesture, gestureOptions(*d))
		trigger.Label(i18n.T("stroke.actionGesture")).Disabled(busy)
	})
	kit.Stack(c, i18n.T("stroke.actionCode"), "", func() {
		ui.TextArea(c, &action.Code).FillWidth().MinHeight(256).Font(kit.MonoFont).Label(i18n.T("stroke.actionCode")).Disabled(busy)
		kit.Help(c, i18n.T("stroke.markHint"))
		kit.Help(c, i18n.T("stroke.scriptHint"))
		ui.Collapsible(c, i18n.T("stroke.advanced"), &t.apiOpen, func() {
			ui.Column(c).Padding(8, 0).Children(func() { kit.Help(c, i18n.T("stroke.apiHint")) })
		}).FontSize(kit.HelpSize(c))
	})
	packID, actionID := pack.ID, action.ID
	ui.Row(c).Children(func() {
		if kit.GhostButton(c, i18n.T("stroke.removeAction"), icons.Trash2, false).Disabled(busy).Clicked() {
			t.removeAction(packID, actionID)
		}
	})
}

func (t *gesturesTab) packageEditor(c *ui.Context, packIndex int, busy bool) {
	d := t.draft
	pack := &d.ActionPackages[packIndex]
	id := pack.ID
	kit.Stack(c, i18n.T("stroke.packageName"), i18n.T("stroke.packageOrder"), func() {
		ui.TextInput(c, &pack.Name).FillWidth().Label(i18n.T("stroke.packageName")).Disabled(busy)
	})
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if kit.Button(c, i18n.T("stroke.moveUp"), icons.ArrowUp, false).Disabled(busy || packIndex == 0).Clicked() {
			t.movePackage(id, -1)
		}
		if kit.Button(c, i18n.T("stroke.moveDown"), icons.ArrowDown, false).Disabled(busy || packIndex == len(d.ActionPackages)-1).Clicked() {
			t.movePackage(id, 1)
		}
		if kit.GhostButton(c, i18n.T("stroke.removePackage"), icons.Trash2, false).Disabled(busy).Clicked() {
			t.removePackage(id)
		}
	})
}

// compileView edits the scripts' references: assemblies and namespaces.
func (t *gesturesTab) compileView(c *ui.Context, busy bool) {
	d := t.draft
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		kit.Help(c, i18n.T("stroke.assemblyHint"))
		kit.Stack(c, i18n.T("stroke.assemblies"), "", func() {
			area := ui.TextArea(c, &t.assemblies).FillWidth().MinHeight(160).Font(kit.MonoFont).Label(i18n.T("stroke.assemblies")).Disabled(busy)
			if area.Changed() {
				d.Assemblies = splitLines(t.assemblies)
			}
			widgets.DropArea(c, t.env, area, widgets.DropFiles, true, busy, func(paths []string, err error) {
				if err != nil {
					t.err = err.Error()
					return
				}
				assemblies := slices.Clone(d.Assemblies)
				for _, path := range paths {
					if !slices.Contains(assemblies, path) {
						assemblies = append(assemblies, path)
					}
				}
				d.Assemblies = assemblies
				t.assemblies = strings.Join(assemblies, "\n")
			})
		})
		kit.Stack(c, i18n.T("stroke.namespaces"), "", func() {
			if ui.TextArea(c, &t.namespaces).FillWidth().MinHeight(256).Font(kit.MonoFont).Label(i18n.T("stroke.namespaces")).
				Disabled(busy).Changed() {
				d.Namespaces = splitLines(t.namespaces)
			}
		})
	})
}

// threadsView edits how scripts run: threads and timeout.
func (t *gesturesTab) threadsView(c *ui.Context, busy bool) {
	d := t.draft
	kit.Panel(c, func() {
		kit.Row(c, i18n.T("stroke.threadCount"), "", func() {
			threads := float64(d.ScriptThreadCount)
			kit.NumberField(c, &threads, 1, 64, 1, "", i18n.T("stroke.threadCount")).Disabled(busy)
			d.ScriptThreadCount = int(threads)
		})
		kit.Row(c, i18n.T("stroke.actionTimeout"), "", func() {
			timeout := float64(d.ScriptTimeoutSeconds)
			kit.NumberField(c, &timeout, 0, 3600, 1, "", i18n.T("stroke.actionTimeout")).Disabled(busy)
			d.ScriptTimeoutSeconds = int(timeout)
		})
	})
	kit.Help(c, i18n.T("stroke.threadHint"))
}

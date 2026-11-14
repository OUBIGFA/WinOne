// Package modelmenu is the one model list every native model picker of
// WinOne opens: the settings window's (Chat composer, AI services, capture
// and selection settings) and the result windows' (quick assistant,
// selection, OCR and translation). It is the Chat composer's list: a
// search field, the models grouped under their provider's mark with a
// count, groups that fold, each model with its mark, its label and its
// badges, and a check on the chosen one. Choices that are not models
// (following the default service, task defaults, off) come first. Change
// it here and every picker follows; callers only supply the data, the
// marks and the strings.
package modelmenu

import (
	"embed"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

//go:embed icons/*.svg
var files embed.FS

func icon(name string) *ui.SVG {
	data, err := files.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

// Lucide icons (ISC), the same files as the settings window's.
var (
	iconCheck  = icon("check")
	iconOpen   = icon("chevron-down")
	iconClosed = icon("chevron-right")
	iconChoice = icon("bot")
)

// Choice is an entry of the menu that is not a model.
type Choice struct {
	Value, Label string
	// Icon defaults to a robot.
	Icon *ui.SVG
}

// Group is a provider's models.
type Group struct {
	ID, Name string
	// Icon draws the provider's mark at size.
	Icon   func(c *ui.Context, size float32) ui.Element
	Models []Entry
}

// Entry is one model of a Group. Value is what the menu returns when it
// is chosen; Label is what it shows; Keywords are searched as well.
type Entry struct {
	Value, Label, Keywords string
	// Icon draws the model's mark at size.
	Icon  func(c *ui.Context, size float32) ui.Element
	Badge func(c *ui.Context)
}

// State is what a menu keeps between frames.
type State struct {
	Open      bool
	Search    string
	collapsed map[string]bool
	shown     bool
	// highlight is the row the arrow keys moved to, "" for none; searched
	// is the search it was moved in.
	highlight, searched string
	lit                 bool
}

// Strings are what the menu says, in the window's language.
type Strings struct {
	// Search is the search field's placeholder and name.
	Search string
	// List names the list for assistive technology.
	List string
	// Empty is shown when there are no models at all; NoMatch when the
	// search finds none.
	Empty, NoMatch string
	// Group, when set, prefixes a provider heading's name for assistive
	// technology, as "供应商：".
	Group string
}

// Options configure Menu.
type Options struct {
	Choices  []Choice
	Groups   []Group
	Selected string
	Strings  Strings
}

// Toggle opens or closes the menu of a trigger that was clicked. Opening
// clears the search and unfolds the group of the chosen model.
func Toggle(s *State, selectedGroup string) {
	s.Open = !s.Open
	if s.Open {
		s.Search, s.searched, s.highlight, s.lit = "", "", "", false
		delete(s.collapsed, selectedGroup)
	}
}

// SelectedFill is the chosen row's fill, the settings window's selection.
func SelectedFill(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#323232")
	}
	return ui.Hex("#e6e6e6")
}

// HelpSize and CaptionSize are the menu's smaller text sizes.
func HelpSize(t *ui.Theme) float32    { return t.Rem(13.0 / 14) }
func CaptionSize(t *ui.Theme) float32 { return t.Rem(12.0 / 14) }

// matches reports whether every word of the query is in text.
func matches(terms []string, text string) bool {
	text = strings.ToLower(text)
	for _, term := range terms {
		if !strings.Contains(text, term) {
			return false
		}
	}
	return true
}

// shownGroup is a group with the models the search finds.
type shownGroup struct {
	Group
	found    []Entry
	expanded bool
}

// Menu shows the menu of state on trigger while it is open. It returns the
// value chosen and whether one was chosen; choosing closes the menu. In the
// search, Down and Up move through the rows shown (starting at the chosen
// one) and Enter chooses the row moved to, or else the first match of a
// search; without either it chooses nothing, as the models of folded
// groups are hidden.
func Menu(c *ui.Context, trigger ui.Element, state *State, o Options) (string, bool) {
	t := c.Theme()
	picked, chosenValue := false, ""
	choose := func(value string) {
		picked, chosenValue = true, value
		state.Open = false
	}
	ui.Popover(c, trigger, &state.Open, func() {
		terms := strings.Fields(strings.ToLower(state.Search))
		// The rows shown, in order, for the keys.
		var choices []Choice
		var groups []shownGroup
		var order []string
		for _, choice := range o.Choices {
			if matches(terms, choice.Label) {
				choices = append(choices, choice)
				order = append(order, choice.Value)
			}
		}
		for _, group := range o.Groups {
			g := shownGroup{Group: group, expanded: len(terms) > 0 || !state.collapsed[group.ID]}
			for _, m := range group.Models {
				if matches(terms, group.Name+" "+m.Value+" "+m.Label+" "+m.Keywords) {
					g.found = append(g.found, m)
					if g.expanded {
						order = append(order, m.Value)
					}
				}
			}
			if len(g.found) > 0 {
				groups = append(groups, g)
			}
		}
		if state.Search != state.searched {
			state.searched, state.highlight, state.lit = state.Search, "", false
		}
		if state.lit && !slices.Contains(order, state.highlight) {
			state.highlight, state.lit = "", false
		}
		width, height := c.Size()
		ui.Column(c).FillWidth().MinWidth(min(340, max(200, width-24))).Gap(6).Children(func() {
			search := ui.TextInput(c, &state.Search).FillWidth().Placeholder(o.Strings.Search).Label(o.Strings.Search).AutoFocus()
			moved := false
			move := func(step int) {
				if len(order) == 0 {
					return
				}
				moved = true
				at := slices.Index(order, state.highlight)
				switch {
				case !state.lit:
					at = slices.Index(order, o.Selected)
					if at < 0 {
						at = 0
					}
				default:
					at = min(max(at+step, 0), len(order)-1)
				}
				state.highlight, state.lit = order[at], true
			}
			if search.Shortcut(0, ui.KeyDown) {
				move(1)
			}
			if search.Shortcut(0, ui.KeyUp) {
				move(-1)
			}
			if search.Submitted() && !search.Composing() {
				switch {
				case state.lit:
					choose(state.highlight)
				case len(terms) > 0 && len(order) > 0:
					choose(order[0])
				}
			}
			ui.Scroll(c).FillWidth().MaxHeight(min(360, max(120, height-180))).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Role(ui.RoleList).Label(o.Strings.List).Children(func() {
					row := func(key, value, label string, mark func(), badge func(c *ui.Context), indent float32) {
						chosen := value == o.Selected
						lit := state.lit && value == state.highlight
						r := ui.ButtonBase(c.Key(key)).FillWidth().Padding(6, 8, 6, indent).Gap(8).Radius(t.Radius).
							Justify(ui.Start).AlignItems(ui.Center).Role(ui.RoleListItem).Selected(chosen).Label(label).Description(label)
						switch {
						case chosen:
							r.Background(SelectedFill(t)).TextColor(t.Text)
						case lit || r.Hovered():
							r.Background(t.SurfaceHover)
						}
						r.Children(func() {
							mark()
							ui.Text(c, label).Grow(1).SingleLine().Ellipsis("…").MinWidth(0)
							if badge != nil {
								badge(c)
							}
							if chosen {
								ui.Icon(c, iconCheck).FontSize(14).Shrink(0)
							}
						})
						if chosen && state.Open && !state.shown || lit && moved {
							r.ScrollIntoView()
						}
						if r.Clicked() {
							choose(value)
						}
					}
					for _, choice := range choices {
						glyph := choice.Icon
						if glyph == nil {
							glyph = iconChoice
						}
						row("choice-"+choice.Value, choice.Value, choice.Label, func() {
							ui.Icon(c, glyph).FontSize(15).TextColor(t.TextMuted).Shrink(0)
						}, nil, 8)
					}
					for index, group := range groups {
						// Provider groups need a larger break than adjacent model rows.
						// The header rule keeps the parent label distinct in both themes.
						space := float32(6)
						if index > 0 || len(choices) > 0 {
							space = 16
						}
						ui.Column(c.Key("models-"+group.ID)).FillWidth().Gap(4).Margin(space, 0, 0, 0).Children(func() {
							head := ui.ButtonBase(c).FillWidth().Padding(6, 8).Gap(8).Radius(t.Radius).Justify(ui.Start).AlignItems(ui.Center).
								Label(o.Strings.Group + group.Name).Description(group.Name).Expanded(group.expanded)
							head.BorderWidth(0, 0, 1, 0).BorderColor(t.Border.Mix(t.Background, 0.4))
							if head.Hovered() {
								head.Background(t.SurfaceHover)
							}
							head.Children(func() {
								if group.Icon != nil {
									group.Icon(c, 16).Shrink(0)
								}
								ui.Text(c, group.Name).FontSize(HelpSize(t)).FontWeight(500).Grow(1).SingleLine().Ellipsis("…").MinWidth(0)
								ui.Text(c, strconv.Itoa(len(group.found))).FontSize(CaptionSize(t)).TextColor(t.TextMuted)
								chevron := iconClosed
								if group.expanded {
									chevron = iconOpen
								}
								ui.Icon(c, chevron).FontSize(13).TextColor(t.TextMuted)
							})
							if head.Clicked() {
								if state.collapsed == nil {
									state.collapsed = map[string]bool{}
								}
								state.collapsed[group.ID] = group.expanded
							}
							if !group.expanded {
								return
							}
							for _, m := range group.found {
								m := m
								row("model-"+m.Value, m.Value, m.Label, func() {
									if m.Icon != nil {
										m.Icon(c, 16).Shrink(0)
									}
								}, m.Badge, 24)
							}
						})
					}
				})
			})
			state.shown = true
			if len(choices) == 0 && len(groups) == 0 {
				message := o.Strings.Empty
				if len(terms) > 0 || message == "" {
					message = o.Strings.NoMatch
				}
				ui.Text(c, message).FontSize(HelpSize(t)).LineHeight(1.45).TextColor(t.TextMuted)
			}
		})
	})
	if !state.Open {
		state.shown = false
	}
	return chosenValue, picked
}

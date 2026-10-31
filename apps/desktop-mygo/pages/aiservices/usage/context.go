package usage

import (
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// contextView is React's UsageContext: how WinOne's requests filled their
// context windows, by provider and by conversation, from the ledger's
// classified prompts.
type contextView struct {
	env      *core.Env
	provider string
	offset   int
	page     *contextReply
	err      string
	loading  bool
	active   bool
	// generation tells the read in flight from those it replaced; key is
	// what the shown or wanted page is of.
	generation int
	key        contextKey
	retry      int
	// open is the request whose prompt shows; titles name conversations.
	open   int64
	titles map[string]string
}

// contextKey is what a page of the analysis is of.
type contextKey struct {
	period, provider        string
	offset, revision, retry int
}

func newContextView(env *core.Env) *contextView {
	return &contextView{env: env, titles: map[string]string{}}
}

func (x *contextView) deactivate() {
	if x.active {
		x.active = false
		x.generation++
		x.loading = false
	}
}

func (x *contextView) sync(period string, revision int) {
	key := contextKey{period: period, provider: x.provider, offset: x.offset, revision: revision, retry: x.retry}
	if !x.active {
		x.active = true
		// Chat's catalog names the conversations; a missing title never
		// hides a recorded request.
		core.Go(x.env, func() (catalog, error) {
			var c catalog
			err := x.env.Host.CallInto("chat.catalog", map[string]any{}, &c)
			return c, err
		}, func(c catalog, err error) {
			if err != nil {
				return
			}
			titles := map[string]string{}
			for _, conversation := range c.Conversations {
				titles[conversation.ID] = conversation.Title
			}
			x.titles = titles
		})
	} else if key == x.key {
		return
	}
	x.key = key
	x.generation++
	generation := x.generation
	x.loading, x.err = true, ""
	params := map[string]any{"period": period, "provider": x.provider, "offset": x.offset}
	core.Go(x.env, func() (contextReply, error) {
		var reply contextReply
		err := x.env.Host.CallCompletingInto("capture.ai.usage.get", params, &reply)
		return reply, err
	}, func(reply contextReply, err error) {
		if generation != x.generation {
			return
		}
		x.loading = false
		if err != nil {
			x.err = err.Error()
			return
		}
		x.page = &reply
	})
}

func (x *contextView) view(c *ui.Context, period string, revision int, providers []Provider, env *core.Env) {
	x.sync(period, revision)
	kit.Help(c, tr("usage.contextHint"))
	ui.Row(c).FillWidth().Children(func() {
		field(c, tr("usage.provider"), func() {
			options := []kit.SelectOption{{Value: "", Label: tr("usage.allProviders")}}
			for _, p := range providers {
				options = append(options, kit.SelectOption{Value: p.ID, Label: p.Name})
			}
			if choose(c, &x.provider, options) {
				x.offset = 0
			}
		}).Grow(0).Basis(260)
	})
	if x.loading {
		kit.Status(c, tr("usage.loading"))
	}
	if x.err != "" {
		kit.Notice(c, kit.Failure, "", x.err, func() {
			if kit.Button(c, tr("common.retry"), icons.RefreshCw, false).Clicked() {
				x.retry++
			}
		})
	}
	page := x.page
	if page == nil {
		return
	}
	var agents []ctxAgent
	var sessions []ctxSession
	if page.Context != nil {
		agents, sessions = page.Context.Agents, page.Context.Sessions
	}
	ui.Row(c).FillWidth().GapX(24).GapY(24).Wrap().AlignItems(ui.Start).Children(func() {
		for _, agent := range agents {
			x.agentView(c, agent, findProvider(providers, agent.Agent), env)
		}
	})
	if len(agents) == 0 {
		kit.Help(c, tr("usage.noContext"))
	}
	var selected *ctxRequest
	for i := range page.ContextRequests {
		if page.ContextRequests[i].ID == x.open {
			selected = &page.ContextRequests[i]
		}
	}
	shownHere := false
	ui.Column(c).FillWidth().Gap(10).Role(ui.RoleGroup).Label(tr("usage.contextSessions")).Children(func() {
		kit.Heading(c, tr("usage.contextSessions")).Role(ui.RoleHeading).Level(3)
		p := kit.Colors(c)
		ui.Column(c).FillWidth().Dividers(1, p.Hairline).Gap(1).Role(ui.RoleList).Children(func() {
			for _, s := range sessions {
				ui.Column(c.Key(s.Key)).FillWidth().Gap(8).Padding(12, 0).Role(ui.RoleListItem).Children(func() {
					title := x.titles[s.Key]
					if title == "" {
						title = s.Title
					}
					if title == "" {
						title = s.Model
					}
					small := s.Model + " · " + toString(s.Requests) + " " + tr("usage.calls") + " · " + toString(s.Compacts) + " " + tr("usage.compactions")
					window := "—"
					if s.Latest.Window != 0 {
						window = toString(s.Latest.Window)
					}
					if groupRow(c, "row", title, small, toString(s.Peak)+" / "+window+" Token", true) {
						x.open = s.LatestID
					}
					x.spark(c, s)
					if selected != nil && selected.Session == s.Key {
						shownHere = true
						promptDetail(c, selected.Prompt)
						if x.titles[s.Key] != "" {
							ui.Row(c).Children(func() {
								if kit.Button(c, tr("usage.openChat"), nil, false).Clicked() {
									openChat(env, s.Key)
								}
							})
						}
					}
				})
			}
		})
		pager(c, page.ContextTotal, x.offset, len(sessions), x.loading, func(offset int) { x.offset = offset })
	})
	if selected != nil && !shownHere {
		promptDetail(c, selected.Prompt)
	}
}

// agentView is a provider's context habits: its score and its parts, the
// sizes its prompts start at and grow by, its cache and its tags.
func (x *contextView) agentView(c *ui.Context, agent ctxAgent, provider *Provider, env *core.Env) {
	t, p := c.Theme(), kit.Colors(c)
	name, mark := agent.Agent, ""
	if provider != nil {
		name, mark = provider.Name, providerMark(provider)
	}
	ui.Column(c.Key(agent.Agent)).Basis(280).Grow(1).Shrink(1).MinWidth(0).Gap(14).Padding(18, 0).BorderWidth(1, 0, 1, 0).
		BorderColor(p.Hairline).Role(ui.RoleGroup).Label(name).Children(func() {
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				widgets.ProviderIcon(c, env, mark, 16)
				kit.Strong(c, name).Role(ui.RoleHeading).Level(4).Grow(1).Shrink(1).MinWidth(0)
				score := "—"
				if agent.WindowKnown {
					score = toString(agent.Score) + "/100"
				}
				ui.Text(c, score).FontSize(t.Rem(20.0 / 14)).FontWeight(500).FontFeatures("tnum").Shrink(0)
			})
			kit.Help(c, toString(agent.Requests)+" "+tr("usage.calls")+" · "+toString(agent.Sessions)+" "+tr("usage.sessions"))
		})
		// Each score a line: its name, a bar of its points of the most, and
		// the points; the names share one width so the bars line up.
		var shown []ctxScore
		labelWidth := float32(0)
		for _, score := range agent.Scores {
			if agent.WindowKnown || score.Key == "cache" || score.Key == "reliable" {
				shown = append(shown, score)
				w, _ := c.MeasureText(0, ui.Span{Text: tr("usage.score" + score.Key)})
				labelWidth = max(labelWidth, w)
			}
		}
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			for _, score := range shown {
				label := tr("usage.score" + score.Key)
				value := 0.0
				if score.Most > 0 {
					value = math.Max(0, math.Min(1, score.Points/score.Most))
				}
				ui.Row(c.Key(score.Key)).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).NoWrap().Width(labelWidth + 1).Shrink(0)
					ui.Progress(c, value).Grow(1).Basis(0).MinWidth(60).Label(label)
					ui.Text(c, toString(score.Points)+"/"+toString(score.Most)).NoWrap().FontFeatures("tnum").MinWidth(44).TextAlign(ui.End).Shrink(0)
				})
			}
		})
		details(c, []tile{
			{label: tr("usage.median"), value: toString(agent.Median) + " Token"},
			{label: tr("usage.p90"), value: toString(agent.P90) + " Token"},
			{label: tr("usage.baseline"), value: toString(agent.Baseline) + " Token"},
			{label: tr("usage.growth"), value: toString(agent.Growth) + " Token"},
			{label: tr("usage.hitRate"), value: toFixed(agent.Cache*100, 1) + "%"},
			{label: tr("usage.compactions"), value: toString(agent.Compacts)},
		})
		if len(agent.Tags) > 0 {
			ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
				for _, tag := range agent.Tags {
					// A tag of a newer Magpie than these words shows its key.
					name := tag.Key
					if i18n.Has("usage.tag" + tag.Key) {
						name = tr("usage.tag" + tag.Key)
					}
					chip := ui.Text(c.Key(tag.Key), name).FontSize(kit.HelpSize(c)).Padding(4, 8).Radius(5).
						Background(t.Surface).MinWidth(0)
					if tag.Value != nil {
						chip.Tooltip(toString(*tag.Value))
					}
				}
			})
		}
		ui.Row(c).FillWidth().Height(8).Gap(2).Children(func() {
			for i, part := range contextParts {
				share := agent.Shares[part]
				if share <= 0 {
					continue
				}
				ui.Box(c.Key(part)).Grow(float32(share)).Basis(0).FillHeight().Background(seriesColors[i]).
					Tooltip(tr("usage.part"+part) + " " + toFixed(share*100, 1) + "%")
			}
		})
	})
}

// spark is a session's prompts over its requests (Magpie's spark): the
// request the card tells of is marked, and a point clicked opens its
// request.
func (x *contextView) spark(c *ui.Context, s ctxSession) {
	if len(s.Points) == 0 {
		return
	}
	t := c.Theme()
	g := newSpark(s.Points, s.Latest.Window)
	window := s.Latest.Window
	box := ui.Box(c.Key("spark")).FillWidth().Height(80)
	bounds := box.Bounds()
	px, _, over := box.PointerPosition()
	hover := -1
	if over && bounds.W > 0 {
		vx := float64(px) * sparkW / float64(bounds.W)
		for i := range s.Points {
			if left, width := g.hit(i); vx >= left && vx < left+width {
				hover = i
			}
		}
	}
	clicked := box.Clicked()
	if hover >= 0 {
		box.Tooltip(tr("usage.contextPoint", "n", hover+1, "tokens", fmtK(s.Points[hover].Tokens))).Cursor(ui.CursorPointer)
		if clicked {
			x.open = s.Points[hover].ID
		}
	}
	open := x.open
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		sx, sy := r.W/sparkW, r.H/sparkH
		X := func(i int) float32 { return r.X + float32(g.x(i))*sx }
		Y := func(v float64) float32 { return r.Y + float32(g.y(v))*sy }
		if window != 0 && window <= g.top {
			line := (&ui.Path{}).MoveTo(r.X, Y(window)).LineTo(r.X+r.W, Y(window))
			p.StrokePath(line.Cap(ui.ButtCap).Dash(0, 3, 3), 1, t.TextMuted)
		}
		if hover >= 0 {
			left, width := g.hit(hover)
			p.Fill(ui.Rect{X: r.X + float32(left)*sx, Y: r.Y, W: float32(width) * sx, H: r.H}, t.Accent.Alpha(0.1), 0)
		}
		if len(s.Points) > 1 {
			area := (&ui.Path{}).MoveTo(X(0), Y(s.Points[0].Tokens))
			line := (&ui.Path{}).MoveTo(X(0), Y(s.Points[0].Tokens))
			for i, pt := range s.Points[1:] {
				area.LineTo(X(i+1), Y(pt.Tokens))
				line.LineTo(X(i+1), Y(pt.Tokens))
			}
			area.LineTo(X(len(s.Points)-1), r.Y+r.H).LineTo(X(0), r.Y+r.H).Close()
			p.FillPath(area, t.Accent.Alpha(0.08))
			p.StrokePath(line, 1.5, t.Accent)
		}
		for i, pt := range s.Points {
			if pt.ID == open {
				// A dot over the stretched line, so it stays round.
				dot := &ui.Path{}
				dot.Circle(X(i), Y(pt.Tokens), 3)
				p.FillPath(dot, t.Accent)
			}
		}
	})
}

// promptDetail is a request's context: how full its window was, the
// grid of its parts, and each part's items.
func promptDetail(c *ui.Context, pr prompt) {
	t := c.Theme()
	window := "—"
	if pr.Window != 0 {
		window = toString(pr.Window)
	}
	ui.Column(c).FillWidth().Gap(12).Padding(18, 0).Children(func() {
		kit.Strong(c, tr("usage.contextWindow")+" · "+toString(pr.Tokens)+" / "+window+" Token").Role(ui.RoleHeading).Level(4)
		note := tr("usage.promptEstimated")
		if pr.Counted {
			note = tr("usage.promptCounted")
		}
		if pr.Held {
			note += " " + tr("usage.promptHeld")
		}
		kit.Help(c, note)
		cells := cellsOf(pr, pr.Window)
		gap := float32(3)
		if w, _ := c.Size(); w <= 760 {
			gap = 2
		}
		ui.Grid(c).FillWidth().Columns(40).Gap(gap).Margin(4, 0, 12, 0).Role(ui.RoleImage).Label(tr("usage.contextComposition")).
			Children(func() {
				for i, cl := range cells {
					color, name := t.Surface, tr("usage.freeContext")
					if cl != nil {
						for k, part := range contextParts {
							if part == cl.kind {
								color = seriesColors[k]
							}
						}
						name = tr("usage.part" + cl.kind)
					}
					ui.Box(c.Key(i)).AspectRatio(1).Radius(1).MinWidth(0).Background(color).Tooltip(name)
				}
			})
		ui.Row(c).FillWidth().GapX(24).GapY(18).Wrap().AlignItems(ui.Start).Children(func() {
			for _, kind := range contextParts {
				var part *promptPart
				for i := range pr.Parts {
					if pr.Parts[i].Kind == kind {
						part = &pr.Parts[i]
						break
					}
				}
				tokens := 0.0
				if part != nil {
					tokens = part.Tokens
				}
				ui.Column(c.Key(kind)).Basis(280).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
					kit.Label(c, tr("usage.part"+kind)+" · "+toString(tokens)+" Token").FontWeight(500)
					if part == nil {
						return
					}
					ui.Column(c).FillWidth().Role(ui.RoleList).Children(func() {
						for i, item := range part.Items {
							name := item.Name
							if item.Tag != "" {
								name += " · " + item.Tag
							}
							groupRow(c, strconv.Itoa(i), name, "", toString(item.Tokens)+" Token", false)
						}
					})
				})
			}
		})
	})
}

package usage

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// requestList keeps which requests show their details, by the time and
// place of their row as React keyed them, and the archives they opened.
type requestList struct {
	open     map[string]bool
	archives map[string]*archiveView
}

// exportState is the CSV export: running, its saved message, its error.
type exportState struct {
	busy  bool
	saved string
	err   string
}

// requestsView is the requests: the heatmap, the account, gateway key and
// purpose filters with the CSV export, the requests and their pages.
func (u *Usage) requestsView(c *ui.Context, page *ledgerPage) {
	f := &u.filter
	providers := u.opts.Providers
	u.heatmapView(c, page.Heatmap)
	ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().AlignItems(ui.End).Children(func() {
		field(c, tr("usage.account"), func() {
			options := []kit.SelectOption{{Value: "", Label: tr("usage.allAccounts")}}
			for _, a := range page.Accounts {
				label := a.Name
				if p := findProvider(providers, a.Account); p != nil && p.Name != "" {
					label = p.Name
				}
				if label == "" {
					label = a.ID
				}
				options = append(options, kit.SelectOption{Value: a.Account, Label: label})
			}
			if choose(c, &f.Account, options) {
				u.change(func(*filter) {})
			}
		})
		field(c, tr("usage.callerKey"), func() {
			options := []kit.SelectOption{{Value: "", Label: tr("usage.allCallerKeys")}, {Value: "none", Label: tr("usage.noCallerKey")}}
			for _, k := range page.CallerKeys {
				label := k.CallerKeyName
				if label == "" {
					label = k.ID
				}
				options = append(options, kit.SelectOption{Value: k.ID, Label: label})
			}
			if choose(c, &f.CallerKey, options) {
				u.change(func(*filter) {})
			}
		})
		field(c, tr("usage.purpose"), func() {
			options := []kit.SelectOption{{Value: "", Label: tr("usage.allPurposes")}}
			for _, p := range page.Purposes {
				options = append(options, kit.SelectOption{Value: p, Label: p})
			}
			if choose(c, &f.Purpose, options) {
				u.change(func(*filter) {})
			}
		})
		if kit.Button(c, tr("usage.export"), icons.Download, u.export.busy).Disabled(u.export.busy || page.Total == 0).Shrink(0).Clicked() {
			u.exportCSV()
		}
	})
	if u.export.err != "" {
		kit.Notice(c, kit.Failure, "", u.export.err, func() {
			if kit.Button(c, tr("common.retry"), icons.RefreshCw, false).Disabled(u.export.busy).Clicked() {
				u.exportCSV()
			}
		})
	}
	u.requestRows(c, page.Rows)
	pager(c, page.Total, page.Offset, len(page.Rows), u.ledger.loading, func(offset int) { u.filter.Offset = offset })
}

// exportCSV saves every request the filters keep as CSV where the user
// chooses. The rows stay in the Host, which writes the file; the window
// sends the filters and the path the system's save dialog returned, and
// shows a save only once the Host confirms that file and its row count.
// A cancelled dialog sends nothing.
func (u *Usage) exportCSV() {
	if u.export.busy {
		return
	}
	u.export = exportState{busy: true}
	query := u.filter
	type saved struct {
		path string
		rows int
	}
	core.Go(u.env, func() (saved, error) {
		path, err := u.env.Dialogs.SaveFile(tr("usage.exportDialog"), core.Filter{Name: tr("usage.exportCsvFilter"), Extensions: []string{"csv"}}, "WinOne-usage.csv")
		if err != nil || path == "" {
			return saved{}, err
		}
		if path, err = withExtension(path, ".csv"); err != nil {
			return saved{}, err
		}
		var receipt struct {
			State string `json:"state"`
			Path  string `json:"path"`
			Rows  *int   `json:"rows"`
		}
		if err := u.env.Host.CallCompletingInto("capture.ai.usage.export.save", map[string]any{"query": query, "path": path}, &receipt); err != nil {
			return saved{}, err
		}
		if receipt.State != "saved" || receipt.Path != path || receipt.Rows == nil || *receipt.Rows < 0 {
			return saved{}, core.Errorf(tr("usage.exportUnconfirmed"))
		}
		return saved{path, *receipt.Rows}, nil
	}, func(s saved, err error) {
		if u.closed {
			return
		}
		u.export.busy = false
		switch {
		case err != nil:
			u.export.err = err.Error()
		case s.path != "":
			u.export.saved = tr("usage.exportSaved", "path", s.path, "count", s.rows)
		}
	})
}

// withExtension adds ext to a name typed without one. MyGo's save dialog
// has no default extension, so it asked before replacing only the name as
// typed: a file that already has the name with ext added is refused
// rather than replaced unasked. Another extension is kept; the Host
// refuses it.
func withExtension(path, ext string) (string, error) {
	if filepath.Ext(path) != "" {
		return path, nil
	}
	path += ext
	if _, err := os.Stat(path); err == nil {
		return "", core.Errorf(tr("usage.exportExists", "path", path))
	}
	return path, nil
}

// requestRows are React's UsageRequests: a request a row, its time
// opening its details and archive. Each row lays its fields out as many
// to a line as fit, named as the table's columns were.
func (u *Usage) requestRows(c *ui.Context, rows []row) {
	if len(rows) == 0 {
		kit.Help(c, tr("usage.empty"))
		return
	}
	if u.rows.open == nil {
		u.rows.open, u.rows.archives = map[string]bool{}, map[string]*archiveView{}
	}
	t, p := c.Theme(), kit.Colors(c)
	providers := u.opts.Providers
	ui.Column(c).FillWidth().Dividers(1, p.Hairline).Gap(1).BorderWidth(1, 0, 1, 0).BorderColor(p.Hairline).Role(ui.RoleList).
		Children(func() {
			for i, r := range rows {
				id := r.Time + strconv.Itoa(i)
				open := u.rows.open[id]
				provider := findProvider(providers, r.Provider)
				ui.Column(c.Key(id)).FillWidth().Padding(14, 0).Gap(12).Role(ui.RoleListItem).Children(func() {
					ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
						toggle := ui.ButtonBase(c).Gap(6).Padding(2, 4).Margin(0, 0, 0, -4).Radius(t.Radius).AlignItems(ui.Center).
							Expanded(open).Shrink(1).MinWidth(0)
						if toggle.Hovered() {
							toggle.Background(t.SurfaceHover)
						}
						toggle.Children(func() {
							chevron := icons.ChevronRight
							if open {
								chevron = icons.ChevronDown
							}
							ui.Icon(c, chevron).FontSize(16).TextColor(t.TextMuted).Shrink(0)
							ui.Text(c, dateTime(r.Time)).FontWeight(500).FontFeatures("tnum").MinWidth(0)
						})
						if toggle.Clicked() {
							if open {
								delete(u.rows.open, id)
								delete(u.rows.archives, id)
							} else {
								u.rows.open[id] = true
							}
						}
						ui.Spacer(c)
						status := tr("usage.success")
						if r.failed() {
							status = tr("usage.failed")
						}
						if r.Rejected {
							status = tr("usage.rejected")
						}
						if r.Status != 0 {
							status += " " + toString(r.Status)
						}
						text := ui.Text(c, status).NoWrap().Shrink(0)
						if r.failed() {
							text.TextColor(t.Danger)
						}
					})
					ui.Row(c).FillWidth().GapX(20).GapY(12).Wrap().AlignItems(ui.Start).Children(func() {
						cell := func(label string, basis float32, fn func()) {
							ui.Column(c.Key(label)).Basis(basis).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
								kit.Caption(c, label)
								fn()
							})
						}
						cell(tr("usage.model"), 200, func() {
							ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
								name := ""
								if provider != nil {
									name = provider.ModelNames[r.Model]
								}
								widgets.ModelIcon(c, u.env, r.Model, name, 16)
								if name == "" {
									name = r.Model
								}
								ui.Text(c, name).MinWidth(0)
							})
						})
						cell(tr("usage.provider"), 150, func() {
							name := r.Provider
							if provider != nil && provider.Name != "" {
								name = provider.Name
							}
							ui.Text(c, name).MinWidth(0)
							source := "usage.winone"
							if r.Agent == "gateway" {
								source = "usage.gateway"
							}
							kit.Help(c, tr(source))
						})
						cell(tr("usage.tokenTotal"), 130, func() {
							ui.Text(c, num(r.In)+" / "+num(r.Out)).FontFeatures("tnum")
							kit.Help(c, tr("usage.cache")+" "+num(valueOf(r.CacheRead)+valueOf(r.CacheWrite)))
						})
						cell(tr("usage.cost"), 120, func() {
							cost := tr("usage.unknownPrice")
							if r.Priced {
								cost = "$" + toFixed(r.Cost, 6)
							}
							ui.Text(c, cost).FontFeatures("tnum")
							if r.Credits != nil {
								kit.Help(c, num(*r.Credits)+" "+tr("usage.credits"))
							}
						})
						cell(tr("usage.elapsed"), 80, func() {
							elapsed := "—"
							if r.Ms > 0 {
								elapsed = toFixed(r.Ms/1000, 2) + " s"
							}
							ui.Text(c, elapsed).FontFeatures("tnum")
						})
						cell(tr("usage.ttft"), 80, func() {
							ttft := "?"
							if r.TTFT != 0 {
								ttft = toFixed(r.TTFT/1000, 2) + " s"
							}
							ui.Text(c, ttft).FontFeatures("tnum")
						})
						cell(tr("usage.speed"), 120, func() {
							s := requestSpeed(r)
							text := "—"
							if s.value != 0 {
								text = num(round(s.value)) + " Token/s"
								if s.floor {
									text = "≥" + text
								}
							}
							shown := ui.Text(c, text).FontFeatures("tnum")
							if s.reason != "" {
								shown.Tooltip(tr("usage." + s.reason))
							}
						})
					})
					if open {
						u.requestDetails(c, id, r)
					}
				})
			}
		})
}

// requestDetails are a request's other fields and its archive.
func (u *Usage) requestDetails(c *ui.Context, id string, r row) {
	optional := func(v *float64) string {
		if v == nil {
			return ""
		}
		return toString(*v)
	}
	ttft := ""
	if r.TTFT != 0 {
		ttft = toString(r.TTFT) + " ms"
	}
	caller := r.CallerKeyName
	if caller == "" {
		caller = r.CallerKeyID
	}
	if caller == "" {
		caller = tr("usage.noCallerKey")
	}
	facts := []tile{
		{label: tr("usage.callerKey"), value: caller}, {label: tr("usage.requested"), value: r.Requested},
		{label: tr("usage.served"), value: r.Served}, {label: tr("usage.endpoint"), value: r.Endpoint},
		{label: tr("usage.reasoning"), value: optional(r.Reasoning)}, {label: tr("usage.effort"), value: r.Effort},
		{label: tr("usage.cacheRead"), value: optional(r.CacheRead)}, {label: tr("usage.cacheWrite"), value: optional(r.CacheWrite)},
		{label: tr("usage.cacheHour"), value: optional(r.CacheWrite1h)}, {label: tr("usage.ttft"), value: ttft},
		{label: tr("usage.stopReason"), value: r.Stop}, {label: tr("usage.error"), value: r.Err},
	}
	p := kit.Colors(c)
	ui.Column(c).FillWidth().Gap(16).Padding(14, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(p.Hairline).Children(func() {
		details(c, facts)
		if r.Archive == "" {
			return
		}
		archive := u.rows.archives[id]
		if archive == nil {
			archive = &archiveView{id: r.Archive}
			u.rows.archives[id] = archive
		}
		archive.view(c, u)
	})
}

// details are labeled values as many to a line as fit, each at least
// 145 DIPs wide; an absent value is a dash. Values can be selected.
func details(c *ui.Context, facts []tile) {
	ui.Row(c).FillWidth().GapX(16).GapY(14).Wrap().AlignItems(ui.Start).Children(func() {
		for _, f := range facts {
			ui.Column(c.Key(f.label)).Basis(145).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
				kit.Caption(c, f.label)
				value := f.value
				if strings.TrimSpace(value) == "" {
					value = "—"
				}
				ui.Text(c, value).Selectable().MinWidth(0)
			})
		}
	})
}

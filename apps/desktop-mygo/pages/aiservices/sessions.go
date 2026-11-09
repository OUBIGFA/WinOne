package aiservices

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"net/url"
	"sort"
	"strings"
	"time"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type externalSession struct {
	ID, Agent, Name, Icon, App, Title, Nickname, Cwd, Path, Resume, Role, ParentSession, RootSession, WSL string
	Start, Last                                                                                           time.Time
	Cost                                                                                                  float64
	Input, Output                                                                                         int64
	CacheRead                                                                                             int64 `json:"cache_read"`
	CacheWrite                                                                                            int64 `json:"cache_write"`
	Unpriced                                                                                              int
	Transcript, Gateway                                                                                   bool
	ReadOnly                                                                                              bool `json:"read_only"`
	Models                                                                                                []struct {
		ID, Model, Name string
		Input, Output   int64
		Cost            float64
	}
	Carry []struct {
		Agent, Command string
		Convert        bool
	}
	Subagents             []struct{ ID, Title, Kind, Parent string }
	ModelEffort           []map[string]any `json:"model_effort"`
	Via                   []map[string]any
	Size                  int64
	Messages, Files       int
	Deletable, Pending    bool
	Undeletable, Provider string
	UsesProvider          string `json:"uses_provider"`
}
type externalSessions struct {
	rows                                    []externalSession
	dirs                                    []string
	terminal                                bool
	reads                                   map[string]any
	chosen                                  map[string]bool
	query, agent, model, cwd, sort, days    string
	limit, page                             int
	selected                                *externalSession
	transcript                              *sessionTranscript
	stats, overview                         map[string]any
	exporting                               bool
	agents                                  []sessionAgent
	trash                                   []sessionTrash
	trashOn, selecting, recording, recorded bool
	trashDir, home                          string
	picked                                  map[string]bool
	search                                  *sessionSearch
	queryShown                              string
	searchDue                               time.Time
	searching                               bool
	relocation                              *sessionRelocation
	accessOpen                              bool
	providerMove                            []externalSession
	movePicked                              map[string]bool
	lastMoves                               []sessionProviderMove
}
type sessionTranscript struct {
	Parts []struct {
		Role, Kind, Name, Text, Model string
		Cut                           int
	}
	Cut      bool
	Source   string
	Captured int
}

func (s *externalSessions) filtered() []externalSession {
	out := []externalSession{}
	query := strings.ToLower(s.query)
	for _, row := range s.rows {
		if s.agent != "" && row.Agent != s.agent && row.App != s.agent || s.cwd != "" && row.Cwd != s.cwd {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(row.Title+" "+row.ID+" "+row.Cwd+" "+row.Name), query) {
			continue
		}
		if s.model != "" {
			found := false
			for _, m := range row.Models {
				if m.ID == s.model || m.Model == s.model || m.Name == s.model {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		if s.days != "" && s.days != "0" {
			duration, err := time.ParseDuration(s.days + "h")
			if err == nil && row.Last.Before(time.Now().Add(-24*duration)) {
				continue
			}
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		switch s.sort {
		case "cost":
			return out[i].Cost > out[j].Cost
		case "tokens":
			return out[i].Input+out[i].Output > out[j].Input+out[j].Output
		case "title":
			return out[i].Title < out[j].Title
		case "start":
			return out[i].Start.After(out[j].Start)
		default:
			return out[i].Last.After(out[j].Last)
		}
	})
	return out
}
func (x *externalPanel) loadSessions() {
	x.request("GET", "/api/sessions/reads", nil, func(raw json.RawMessage) {
		if err := json.Unmarshal(raw, &x.sessions.reads); err != nil {
			x.err = err.Error()
			return
		}
		x.sessions.chosen = map[string]bool{}
		for _, a := range rowsOf(x.sessions.reads, "found") {
			x.sessions.chosen[textOf(a, "agent")] = boolOf(a, "on")
		}
		if boolOf(x.sessions.reads, "asked") {
			x.readSessions()
		}
	})
}
func (x *externalPanel) readSessions() {
	x.request("GET", "/api/sessions/manage?agent="+url.QueryEscape(x.sessions.agent), nil, x.acceptSessions)
}
func (x *externalPanel) sessionReads(all bool, selected map[string]bool) {
	x.request("POST", "/api/sessions/reads", map[string]any{"all": all, "agents": selected}, func(raw json.RawMessage) {
		if err := json.Unmarshal(raw, &x.sessions.reads); err != nil {
			x.err = err.Error()
			return
		}
		x.readSessions()
	})
}

func sessionModels(row externalSession) []string {
	out := []string{}
	for _, m := range row.Models {
		value := m.ID
		if value == "" {
			value = m.Model
		}
		if value == "" {
			value = m.Name
		}
		out = append(out, value)
	}
	return out
}
func (x *externalPanel) readSessionStats() {
	s := &x.sessions
	query := "?days=" + url.QueryEscape(s.days)
	x.request("GET", "/api/sessions/stats"+query, nil, func(raw json.RawMessage) {
		if err := json.Unmarshal(raw, &s.stats); err != nil {
			x.err = err.Error()
			return
		}
		x.request("GET", "/api/sessions/overview"+query+"&agent="+url.QueryEscape(s.agent)+"&model="+url.QueryEscape(s.model)+"&cwd="+url.QueryEscape(s.cwd), nil, func(raw json.RawMessage) {
			if err := json.Unmarshal(raw, &s.overview); err != nil {
				x.err = err.Error()
			}
		})
	})
}
func (x *externalPanel) sessionStatistics(c *ui.Context) {
	s := &x.sessions
	kit.Section(c, nil, upstreamT("Overview"), "", func() { externalDetails(c, s.overview) })
	// Daily rows preserve upstream grouping by agent, folder and model. They are
	// native readable tables, including sparse usage fields and active counts.
	for _, day := range rowsOf(s.stats, "days") {
		label := textOf(day, "date")
		if label == "" {
			label = textOf(day, "day")
		}
		kit.Section(c.Key(label), nil, label, "", func() { externalDetails(c, day) })
	}
	if kit.Button(c, upstreamT("Hide statistics"), nil, false).Clicked() {
		s.stats = nil
		s.overview = nil
	}
}
func (x *externalPanel) sessionDetail(c *ui.Context) {
	s := &x.sessions
	row := s.selected
	ui.Row(c).Gap(8).Wrap().Children(func() {
		kit.Strong(c, row.Title).Grow(1)
		if kit.Button(c, upstreamT("Back"), nil, false).Disabled(x.working).Clicked() {
			s.selected = nil
			s.transcript = nil
		}
	})
	for _, pair := range [][2]string{{"Client", row.Name}, {"ID", row.ID}, {"Folder", row.Cwd}, {"File", row.Path}, {"Model", strings.Join(sessionModels(*row), ", ")}, {"Started", row.Start.Local().Format(time.RFC3339)}, {"Last active", row.Last.Local().Format(time.RFC3339)}, {"Resume command", row.Resume}} {
		kit.Help(c, upstreamT(pair[0])+": "+pair[1])
	}
	kit.Help(c, fmt.Sprintf("%s: %d / %d · %s: $%.6f", upstreamT("Input / Output"), row.Input, row.Output, upstreamT("Cost"), row.Cost))
	externalDetails(c, row.ModelEffort)
	externalDetails(c, row.Via)
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if kit.Button(c, externalT("copyCommand"), icons.Copy, false).Disabled(row.Resume == "").Clicked() {
			c.Services().WriteClipboard(row.Resume)
		}
		if kit.Button(c, upstreamT("Continue in terminal"), nil, false).Disabled(x.working || !s.terminal || row.Resume == "").Clicked() {
			x.request("POST", "/api/sessions/terminal", map[string]string{"agent": row.Agent, "id": row.ID}, func(json.RawMessage) { x.message = externalT("saved") })
		}
		if kit.Button(c, externalT("transcript"), nil, false).Disabled(x.working || !row.Transcript).Clicked() {
			x.readTranscript("")
		}
		if kit.Button(c, upstreamT("Export Markdown"), icons.Download, false).Disabled(x.working || !row.Transcript).Clicked() {
			x.ask(upstreamT("Export Markdown"), "/api/sessions/export?agent="+url.QueryEscape(row.Agent)+"&id="+url.QueryEscape(row.ID), map[string]any{}, x.receiveResult)
		}
		if kit.Button(c, upstreamT("Copy Markdown"), icons.Copy, false).Disabled(x.working || !row.Transcript).Clicked() {
			x.request("GET", "/api/sessions/markdown?agent="+url.QueryEscape(row.Agent)+"&id="+url.QueryEscape(row.ID), nil, func(raw json.RawMessage) {
				var text string
				if err := json.Unmarshal(raw, &text); err != nil {
					x.err = err.Error()
					return
				}
				c.Services().WriteClipboard(text)
			})
		}
	})
	for _, carry := range row.Carry {
		target := carry
		kit.Section(c.Key("carry"+target.Agent), nil, upstreamT("Continue in")+" "+target.Agent, target.Command, func() {
			if target.Convert {
				for _, open := range []bool{false, true} {
					label := upstreamT("Convert")
					if open {
						label = upstreamT("Convert and open")
					}
					if kit.Button(c, label, nil, false).Disabled(x.working || open && !s.terminal).Clicked() {
						x.ask(label, "/api/sessions/carry", map[string]any{"agent": row.Agent, "id": row.ID, "to": target.Agent, "open": open}, x.receiveResult)
					}
				}
			} else {
				if kit.Button(c, externalT("copyCommand"), icons.Copy, false).Clicked() {
					c.Services().WriteClipboard(target.Command)
				}
				if kit.Button(c, upstreamT("Continue in terminal"), nil, false).Disabled(x.working || !s.terminal).Clicked() {
					x.request("POST", "/api/sessions/terminal", map[string]string{"agent": row.Agent, "id": row.ID, "in": target.Agent}, func(json.RawMessage) {})
				}
			}
		})
	}
	for _, sub := range row.Subagents {
		item := sub
		if kit.Button(c, upstreamT("Subagent")+" · "+item.Title+" · "+item.Kind, nil, false).Disabled(x.working).Clicked() {
			x.readTranscript(item.ID)
		}
	}
	if s.transcript != nil {
		if s.transcript.Cut {
			kit.Help(c, upstreamT("The conversation preview is truncated. Export Markdown for the full conversation."))
		}
		for index, part := range s.transcript.Parts {
			p := part
			kit.Section(c.Key(fmt.Sprint(index)), nil, upstreamT(p.Role)+" · "+upstreamT(p.Kind), p.Name+" "+p.Model, func() {
				ui.TextArea(c, &p.Text).FillWidth().MinHeight(80).MaxHeight(460).ReadOnly(true).Label(p.Role)
				if p.Cut > 0 {
					kit.Help(c, fmt.Sprintf("%d %s", p.Cut, upstreamT("characters omitted")))
				}
				if kit.Button(c.Key(fmt.Sprint(index)+"copy"), upstreamT("Copy"), icons.Copy, false).Clicked() {
					c.Services().WriteClipboard(p.Text)
				}
			})
		}
	}
	if x.result != nil {
		externalDetails(c, x.result)
		if kit.Button(c, i18n.T("common.close"), nil, false).Clicked() {
			x.result = nil
		}
	}
}
func (x *externalPanel) readTranscript(sub string) {
	row := x.sessions.selected
	if row == nil {
		return
	}
	x.request("GET", "/api/sessions/transcript?agent="+url.QueryEscape(row.Agent)+"&id="+url.QueryEscape(row.ID)+"&sub="+url.QueryEscape(sub), nil, func(raw json.RawMessage) {
		var transcript sessionTranscript
		if err := json.Unmarshal(raw, &transcript); err != nil {
			x.err = err.Error()
			return
		}
		x.sessions.transcript = &transcript
	})
}

package aiservices

import (
	"encoding/json"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Read the same live gateway that executes calls. A draft changes the shared
// setting only; runtime observations remain the last saved execution's facts.
type claudeModePanel struct {
	api    nativeAPI
	url    string
	readAt time.Time
	state  *claudeRunMode
}
type claudeRunMode struct {
	TUI  bool   `json:"tui"`
	Why  string `json:"why"`
	WSL  bool   `json:"wsl"`
	Last *struct {
		TUI bool      `json:"tui"`
		Why string    `json:"why"`
		At  time.Time `json:"at"`
	} `json:"last"`
}

func (e *editor) claudeModeView(c *ui.Context, locked bool) {
	p, m := e.p, &e.claudeMode
	if p.extensions.state != nil {
		address := p.extensions.state.URL
		if address != m.url {
			m.api.close()
			m.url = address
			m.state = nil
			m.readAt = time.Time{}
			m.api.err = ""
		}
		if !m.api.busy && m.api.err == "" && time.Since(m.readAt) > 30*time.Second {
			m.readAt = time.Now()
			m.api.request(p, "GET", "/api/claude/mode", nil, func(data json.RawMessage) {
				var state claudeRunMode
				if err := json.Unmarshal(data, &state); err != nil {
					m.api.err = err.Error()
					return
				}
				m.state = &state
			})
		}
	}
	kit.Group(c, icons.Terminal, i18n.T("aiComplete.claudeMode"), "", func() {
		kit.Stack(c, "", "", func() {
			s := p.draft.Value.settings()
			mode := "tui"
			if s.ClaudePrintMode {
				mode = "p"
			}
			ui.Row(c).FillWidth().Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Disabled(locked).Children(func() {
					if kit.Segmented(c, &mode, options("tui", i18n.T("aiComplete.claudeTUI"), "p", "claude -p"), i18n.T("aiComplete.claudeMode")) {
						s.ClaudePrintMode = mode == "p"
					}
				})
				if kit.IconButton(c, icons.RefreshCw, i18n.T("common.refresh")).Disabled(m.api.busy).Clicked() {
					m.api.err = ""
					m.readAt = time.Time{}
				}
			})
			if m.state != nil {
				state := m.state
				if !s.ClaudePrintMode && state.Why != "" && state.Why != "off" {
					kit.Help(c, i18n.T("aiComplete.claudeWhy", "reason", claudeModeReason(state.Why)))
				}
				if state.WSL {
					kit.Help(c, i18n.T("aiComplete.claudeWSL"))
				}
				if last := state.Last; last != nil {
					name := "claude -p"
					if last.TUI {
						name = i18n.T("aiComplete.claudeTUI")
					}
					kit.Help(c, i18n.T("aiComplete.claudeLast", "time", last.At.Local().Format("2006-01-02 15:04:05"), "mode", name))
					if last.Why != "" {
						kit.Help(c, claudeModeReason(last.Why))
					}
				}
			}
			kit.InlineError(c, m.api.err)
			ui.Collapsible(c, i18n.T("aiComplete.claudeModeDetails"), &e.claudeModeDetails, func() {
				kit.Help(c, i18n.T("aiComplete.claudeTUIHint"))
				kit.Help(c, i18n.T("aiComplete.claudePrintHint"))
				kit.Help(c, i18n.T("aiComplete.claudeFallbackHint"))
			})
		})
	})
}

func claudeModeReason(code string) string {
	switch code {
	case "off", "os", "missing":
		return i18n.T("aiComplete.claudeReason." + code)
	default:
		return code // Keep a newer upstream reason instead of inventing one.
	}
}

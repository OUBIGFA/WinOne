package mcpapp

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

type setupField struct {
	ID, Type, Label, Description, Placeholder string
	Required                                  bool
	MaxLength                                 int
	ExternalLink                              *struct {
		URL string `json:"url"`
	} `json:"externalLink"`
}
type setupAction struct {
	ID, Kind string
	Form     struct {
		Fields []setupField `json:"fields"`
	} `json:"form"`
}
type setupStep struct {
	ID, GroupID, GroupMode, Title, Description, Phase, ErrorCode string
	Action                                                       *setupAction `json:"action"`
}
type setupSnapshot struct {
	Kind, RequestID, Intro string
	Revision               int
	Terminal               bool
	Ghost                  struct{ ID, Name string } `json:"ghost"`
	Steps                  []setupStep               `json:"steps"`
}

// SetupPrompt is a native composition of the Host-authored any-of card. Values
// remain in private native Password controls and are cleared on submission;
// there is no JSON text editor, plugin iframe or model credential parameter.
type SetupPrompt struct {
	snapshot       setupSnapshot
	values         map[string]string
	OnOpenExternal func(string)
}

func NewSetupPrompt(raw json.RawMessage) (*SetupPrompt, error) {
	var snapshot setupSnapshot
	if json.Unmarshal(raw, &snapshot) != nil || snapshot.Kind != "plugin_setup" || snapshot.RequestID == "" || snapshot.Revision < 0 || len(snapshot.Steps) == 0 {
		return nil, errors.New("INVALID_PLUGIN_SETUP")
	}
	return &SetupPrompt{snapshot: snapshot, values: map[string]string{}}, nil
}
func (p *SetupPrompt) Clear() { clear(p.values) }
func (p *SetupPrompt) Update(raw json.RawMessage) error {
	next, err := NewSetupPrompt(raw)
	if err != nil {
		return err
	}
	if next.snapshot.RequestID != p.snapshot.RequestID || next.snapshot.Revision != p.snapshot.Revision {
		p.Clear()
	}
	p.snapshot = next.snapshot
	return nil
}
func setupButton(action setupAction) string {
	if action.Kind == "inline_form" {
		return i18n.T("pluginHost.save")
	}
	if action.Kind == "oauth_connect" {
		return i18n.T("pluginChecks.connect")
	}
	return i18n.T("pluginChecks.settings")
}

// View supplies the body of the existing native pluginHost dialog. The dialog's
// Cancel response is separate; a clicked step returns only its current action.
func (p *SetupPrompt) View(c *ui.Context) (map[string]any, bool) {
	var response map[string]any
	ui.Column(c).FillWidth().MinWidth(0).Gap(14).Children(func() {
		if p.snapshot.Intro != "" {
			kit.Help(c, p.snapshot.Intro)
		}
		previous := ""
		for _, step := range p.snapshot.Steps {
			if step.GroupID != previous {
				count := 0
				for _, other := range p.snapshot.Steps {
					if other.GroupID == step.GroupID && other.Action != nil {
						count++
					}
				}
				if count > 1 {
					kit.Help(c, i18n.T("pluginChecks.setupAnyOf"))
				}
				previous = step.GroupID
			}
			ui.Column(c.Key(step.ID)).FillWidth().MinWidth(0).Gap(7).Children(func() {
				ui.Text(c, step.Title).MinWidth(0)
				if step.Description != "" {
					kit.Help(c, step.Description)
				}
				if step.ErrorCode != "" {
					kit.Help(c, i18n.T("pluginHost.actionFailed"))
				}
				action := step.Action
				if p.snapshot.Terminal || action == nil || step.Phase != "pending" && step.Phase != "failed" {
					return
				}
				disabled := false
				if action.Kind == "inline_form" {
					if len(action.Form.Fields) != 1 || action.Form.Fields[0].Type != "secret" || action.Form.Fields[0].ID != "value" {
						return
					}
					field := action.Form.Fields[0]
					value := p.values[action.ID]
					if kit.TextInput(c.Key("secret:"+action.ID), &value).Password().FillWidth().Label(field.Label).Placeholder(field.Placeholder).Changed() {
						p.values[action.ID] = value
					}
					disabled = strings.TrimSpace(value) == "" || field.MaxLength < 1 || len(utf16.Encode([]rune(strings.TrimSpace(value)))) > field.MaxLength
					if field.Description != "" && field.Description != step.Description {
						kit.Help(c, field.Description)
					}
					if field.ExternalLink != nil && p.OnOpenExternal != nil {
						address, err := url.Parse(field.ExternalLink.URL)
						if err == nil && address.Scheme == "https" && address.User == nil && kit.Button(c.Key("link:"+action.ID), address.Host, nil, false).Clicked() {
							p.OnOpenExternal(address.String())
						}
					}
				}
				if kit.PrimaryButton(c.Key("action:"+action.ID), setupButton(*action), nil, false).Disabled(disabled).Clicked() {
					response = map[string]any{"action": "run_action", "actionId": action.ID}
					if action.Kind == "inline_form" {
						response["action"], response["value"] = "submit", p.values[action.ID]
					}
				}
			})
		}
	})
	if response != nil {
		p.Clear()
		return response, true
	}
	return nil, false
}

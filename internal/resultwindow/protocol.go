// Package resultwindow defines the shared native result host's local protocol.
package resultwindow

import "encoding/json"

const MaxQuickState = 256 * 1024

// QuickOptions carries only presentation preferences into the shared host.
type QuickOptions struct {
	AlwaysOnTop   bool  `json:"alwaysOnTop"`
	ShowReasoning *bool `json:"showReasoning,omitempty"`
}

type Initial struct {
	State    json.RawMessage   `json:"state,omitempty"`
	Quick    QuickOptions      `json:"quick,omitzero"`
	Actions  []SelectionAction `json:"actions,omitempty"`
	ID       string            `json:"id"`
	Token    string            `json:"token"`
	Pipe     string            `json:"pipe"`
	HostPID  uint32            `json:"hostPid"`
	Mode     string            `json:"mode"`
	Language string            `json:"language"`
	Theme    string            `json:"theme"`
	Source   string            `json:"source,omitempty"`
	Text     string            `json:"text,omitempty"`
	Action   string            `json:"action,omitempty"`
	Toolbar  bool              `json:"toolbar,omitempty"`
	Anchor   [2]int32          `json:"anchor,omitempty"`
}

type Command struct {
	Request string   `json:"request,omitempty"`
	Kind    string   `json:"kind"`
	Initial *Initial `json:"initial,omitempty"`
	ID      string   `json:"id,omitempty"`
	Point   [2]int32 `json:"point,omitempty"`
	Force   bool     `json:"force,omitempty"`
	Window  uint64   `json:"window,omitempty"`
}

// SelectionHandoff carries text over the authenticated result session channel,
// never through the system clipboard, process arguments, or diagnostic logs.
type SelectionHandoff struct {
	Session string `json:"session"`
	Text    string `json:"text"`
}

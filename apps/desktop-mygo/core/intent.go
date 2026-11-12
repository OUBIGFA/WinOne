package core

import "encoding/json"

// Intent is a prefill another launch queued in the Host for a page, as
// Explorer's "create a link" and "send with WinOne" commands do: paths to
// use, and what the command chose. The page shows it; nothing changes on
// the disk or the network until the user confirms there.
type Intent struct {
	Requested bool            `json:"requested"`
	Paths     []string        `json:"paths"`
	Options   json.RawMessage `json:"options,omitempty"`
	// Receive and Code are a transfer's: open the receiving side, with a
	// share code.
	Receive bool   `json:"receive,omitempty"`
	Code    string `json:"code,omitempty"`
}

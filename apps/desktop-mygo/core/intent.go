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
	// Conversation, File and ChatSettings are Chat's, from links of other
	// pages (OneBot's): the conversation to open, a file of it to show,
	// or Chat settings.
	Conversation string      `json:"conversation,omitempty"`
	File         *IntentFile `json:"file,omitempty"`
	ChatSettings bool        `json:"chatSettings,omitempty"`
	// NewChat opens a new conversation with Prefill added to its unsent
	// draft, as Cindy's error diagnosis prefills a new task (nothing is sent).
	NewChat bool   `json:"newChat,omitempty"`
	Prefill string `json:"prefill,omitempty"`
	// AutomationDraft opens the existing create form only. Its values never
	// become a saved schedule until the user presses that form's Save button.
	AutomationDraft json.RawMessage `json:"automationDraft,omitempty"`
}

// IntentFile is a file a link shows: at a line, or a folder in the file
// library.
type IntentFile struct {
	Path      string `json:"path"`
	Line      int    `json:"line,omitempty"`
	Directory bool   `json:"directory,omitempty"`
}

// Package webui holds the web content the native pages still show in web
// views of the window: pages of the Chat side panel's browser and sandboxed
// HTML previews. Nothing here hosts React or binds a service: every Chat,
// automation and OneBot control is native (docs/MyGo前端迁移.md, section 9).
package webui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

// openURL opens a link in the system's browser, off the UI thread.
var openURL = func(u string) { go mygo.Shell.OpenExternal(u) }

// copyPNG puts a screenshot on the clipboard.
func copyPNG(data []byte) error {
	return mygo.Clipboard.Write(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, data))))
}

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

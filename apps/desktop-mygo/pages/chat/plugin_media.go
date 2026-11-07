package chat

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/pluginhost/globalpages"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

const pluginHandoverFormat transfer.Format = "application/x-cindy-ghost-handover"

// URI and source ticket are opaque inputs to the real Host ledger gate. The
// native drop path never derives a filename, MIME, hash or authority from them.
func pluginMediaSource(data transfer.Data) (uri, token string, found bool, err error) {
	if _, offered := data.Preferred(pluginHandoverFormat); offered {
		raw, readErr := data.Read(pluginHandoverFormat)
		if readErr != nil {
			return "", "", true, readErr
		}
		var value struct {
			URI         string `json:"uri"`
			SourceToken string `json:"sourceToken"`
		}
		if len(raw) > 4096 || json.Unmarshal(raw, &value) != nil || value.URI == "" || len(value.SourceToken) > 128 {
			return "", "", true, core.Errorf(i18n.T("chat.attachmentImportFailed"))
		}
		return value.URI, value.SourceToken, true, nil
	}
	for _, format := range []transfer.Format{transfer.URIList, transfer.Text} {
		raw, readErr := data.Read(format)
		if errors.Is(readErr, transfer.ErrFormat) {
			continue
		}
		if readErr != nil {
			return "", "", false, readErr
		}
		for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
			candidate := strings.TrimSpace(line)
			if candidate == "" || strings.HasPrefix(candidate, "#") {
				continue
			}
			if strings.HasPrefix(candidate, "cindy-ghost://") {
				return candidate, "", true, nil
			}
			break
		}
	}
	return "", "", false, nil
}

func (p *Chat) pluginMediaDrop(c *ui.Context, target ui.Element, locked bool) bool {
	options := transfer.DropOptions{Formats: []transfer.Format{pluginHandoverFormat, transfer.URIList, transfer.FileList}, Operations: transfer.Copy}
	drop, dropped := ui.DropData(target, options)
	if !dropped {
		return false
	}
	if locked {
		return true
	}
	uri, token, media, err := pluginMediaSource(drop.Data)
	if err != nil {
		p.err = err.Error()
		return true
	}
	if media {
		globalpages.For(p.env).AttachMedia(uri, p.activeID, "", "", token, func(err error) {
			if !p.closed() && err != nil {
				p.err = err.Error()
			}
		})
		return true
	}
	// Advertising URIList also matches Explorer's file list. Preserve the
	// ordinary native attachment import rather than swallowing those drops.
	paths, err := drop.Data.Files()
	if err != nil {
		p.err = err.Error()
		return true
	}
	if len(paths) > 0 {
		p.run(func(done func(error)) { p.addPaths(paths, done) })
	}
	return true
}

// Import receipts, not media metadata, finish the button/drop. The destination
// may be busy or loading; its requests remain queued until its draft is usable.
func (p *Chat) consumePluginAttachments() {
	if p.closed() || p.busy || p.opening || p.draftLoading || p.reconnecting || p.transferBusy || p.stopping {
		return
	}
	files, acknowledge := globalpages.For(p.env).TakeAttachments(p.activeID)
	if len(files) == 0 {
		return
	}
	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.Path
	}
	p.run(func(done func(error)) {
		p.addPaths(paths, func(err error) {
			// A new conversation keeps pending local paths until its first
			// prompt creates the session. Preserve the verified image identity.
			if err == nil && p.activeID == "" {
				for i, attached := range p.files {
					for _, file := range files {
						if attached.Path == file.Path {
							p.files[i] = file
						}
					}
				}
				p.updateText(p.text, p.files, "")
			}
			acknowledge(err)
			done(err)
		})
	})
}

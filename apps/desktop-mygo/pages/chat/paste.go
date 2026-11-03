package chat

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// Attachment byte limits, as the Host's upload and the React composer
// enforce them.
const (
	imageLimit = 100 << 20
	fileLimit  = 20 << 20
)

// pastedImage is a picture the clipboard holds as bytes, as a screenshot.
type pastedImage struct {
	name, mimeType string
	data           []byte
}

// paste takes a paste into the composer that carries files, as the React
// composer's paste (Cindy's ChatInput.handlePaste): Explorer's files and
// folders attach by their paths, folders as @path references; a picture
// without a path, as a screenshot, uploads its bytes. A paste of text
// alone stays the input's; text that comes with files is dropped, as the
// files are what was copied. While the draft cannot change (busy,
// paused), a paste changes nothing.
func (p *Chat) paste(locked bool) bool {
	if locked {
		return true
	}
	services := p.ui.services
	formats := services.ClipboardFormats()
	files := slices.Contains(formats, transfer.FileList)
	image := slices.Contains(formats, transfer.PNG)
	if !files && !image {
		return false
	}
	if files {
		paths, err := services.ReadClipboardData(transfer.FileList).Files()
		if err == nil && len(paths) > 0 {
			p.run(func(done func(error)) { p.addPaths(paths, done) })
			return true
		}
	}
	if !image {
		return true
	}
	data, err := services.ReadClipboardData(transfer.PNG).Read(transfer.PNG)
	if err != nil {
		p.err = err.Error()
		return true
	}
	p.run(func(done func(error)) {
		p.addImages([]pastedImage{{name: "image.png", mimeType: "image/png", data: data}}, done)
	})
	return true
}

// addImages attaches pictures from their bytes: an open conversation
// uploads them to its attachments; a chat not created yet keeps them as
// files of its own until the first message imports them.
func (p *Chat) addImages(images []pastedImage, done func(error)) {
	id := p.activeID
	if len(p.files)+len(images) > maxAttachments {
		done(core.Errorf(i18n.T("chat.attachmentCountLimit")))
		return
	}
	for _, image := range images {
		limit := fileLimit
		if strings.HasPrefix(image.mimeType, "image/") {
			limit = imageLimit
		}
		if len(image.data) > limit {
			done(core.Errorf(i18n.T("chat.attachmentSizeLimit")))
			return
		}
	}
	var next func(i int)
	next = func(i int) {
		if i == len(images) {
			done(nil)
			return
		}
		attach := func(added model.Attachment, err error) {
			if err == nil && p.selection != id {
				err = core.Errorf(i18n.T("chat.browserDraftBusy"))
			}
			if err != nil {
				done(err)
				return
			}
			p.updateText(p.text, mergeFiles(p.files, []model.Attachment{added}), id)
			next(i + 1)
		}
		if id == "" {
			p.keepImage(images[i], attach)
		} else {
			p.uploadAttachment(id, images[i], attach)
		}
	}
	next(0)
}

// uploadAttachment sends a file's bytes to the conversation as
// chat-attachment-input.ts does: the Host grants a one-time local
// address, the bytes go there by HTTP, never through the RPC pipe, and
// the Host answers with the attachment it stored.
func (p *Chat) uploadAttachment(id string, image pastedImage, done func(model.Attachment, error)) {
	chatCall(p, "attachments.upload", map[string]any{"id": id, "name": image.name}, func(r struct {
		URL string `json:"url"`
	}, err error) {
		if err != nil {
			done(model.Attachment{}, err)
			return
		}
		core.Go(p.env, func() (model.Attachment, error) {
			response, err := http.Post(r.URL, image.mimeType, bytes.NewReader(image.data))
			if err != nil {
				return model.Attachment{}, err
			}
			defer response.Body.Close()
			body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			if err != nil {
				return model.Attachment{}, err
			}
			if response.StatusCode != http.StatusOK {
				message := strings.TrimSpace(string(body))
				if len(message) > 300 {
					message = message[:300]
				}
				return model.Attachment{}, core.Errorf(first(message, i18n.T("chat.attachmentImportFailed")))
			}
			var result struct {
				Attachment *model.Attachment `json:"attachment"`
			}
			if err := json.Unmarshal(body, &result); err != nil || result.Attachment == nil {
				return model.Attachment{}, core.Errorf(i18n.T("chat.attachmentImportFailed"))
			}
			return *result.Attachment, nil
		}, done)
	})
}

// pasteFolder holds the pictures pasted into a chat not created yet.
func pasteFolder() string { return filepath.Join(os.TempDir(), "WinOne", "chat-paste") }

// keepImage writes a picture for a chat not created yet into a file of
// the page's own, which the first message imports like any other file
// (React keeps the bytes in memory; the native draft holds paths).
func (p *Chat) keepImage(image pastedImage, done func(model.Attachment, error)) {
	core.Go(p.env, func() (model.Attachment, error) {
		folder := pasteFolder()
		if err := os.MkdirAll(folder, 0o700); err != nil {
			return model.Attachment{}, err
		}
		file, err := os.CreateTemp(folder, "*-"+image.name)
		if err != nil {
			return model.Attachment{}, err
		}
		_, err = file.Write(image.data)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(file.Name())
			return model.Attachment{}, err
		}
		return model.Attachment{Name: image.name, Path: file.Name(), Size: float64(len(image.data)), MimeType: image.mimeType}, nil
	}, func(added model.Attachment, err error) {
		if err == nil {
			if p.ui.items.pasted == nil {
				p.ui.items.pasted = map[string]bool{}
			}
			p.ui.items.pasted[added.Path] = true
		}
		done(added, err)
	})
}

// releasePasted deletes the pasted pictures no draft holds any more: the
// new chat's draft, the composer's, or a message on its way. A send in
// progress keeps them, as it imports them once the chat exists.
func (p *Chat) releasePasted() {
	s := &p.ui.items
	if len(s.pasted) == 0 || p.busy {
		return
	}
	held := map[string]bool{}
	for _, list := range [][]model.Attachment{p.files, p.newFiles} {
		for _, f := range list {
			held[f.Path] = true
		}
	}
	if p.submitting != nil {
		for _, f := range p.submitting.Attachments {
			held[f.Path] = true
		}
	}
	for path := range s.pasted {
		if !held[path] {
			delete(s.pasted, path)
			os.Remove(path)
		}
	}
}

// onPaste wires the composer's paste.
func (p *Chat) onPaste(input ui.Element, locked bool) {
	input.OnPaste(func(string) bool { return p.paste(locked) })
}

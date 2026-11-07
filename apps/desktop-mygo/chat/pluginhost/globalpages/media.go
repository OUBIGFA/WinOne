package globalpages

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/chat/workspace"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// panelMedia is a Host-verified ledger result. Its path/URL/mime are never
// constructed from the plugin's HTML, suffix or self-reported metadata.
type panelMedia struct {
	Path string  `json:"path"`
	Name string  `json:"name"`
	Kind string  `json:"kind"`
	MIME string  `json:"mimeType"`
	Size float64 `json:"size"`
	URL  string  `json:"url"`
}
type mediaPreview struct {
	source, attachSource, session, plugin, pageID string
	body                                          *workspace.DocumentPane
	busy                                          bool
	err                                           string
}

func (c *Controller) closeMedia() {
	c.mediaGeneration++
	if c.media != nil && c.media.body != nil {
		c.media.body.Close()
	}
	c.media = nil
}

// PreviewMedia consumes only the Host's authenticated preview-media event.
// The upstream preview gate owns genuine focus, rate and ledger validation.
// This dialog is a standard media lightbox, not a substitute page container.
func (c *Controller) PreviewMedia(payload json.RawMessage) {
	var event struct{ Src, URI, URL, PluginID, PageID string }
	if json.Unmarshal(payload, &event) != nil {
		return
	}
	source := event.Src
	if source == "" {
		source = event.URL
	}
	if source == "" {
		return
	}
	c.closeMedia()
	media := &mediaPreview{source: source, attachSource: event.URI, session: c.session, plugin: event.PluginID, pageID: event.PageID}
	media.body = workspace.NewDocumentPane(c.env, workspace.DocumentOptions{
		Path: source,
		Preview: func() (workspace.FilePreview, error) {
			var preview workspace.FilePreview
			err := c.env.Host.CallInto("chat.ghosts.media-preview", map[string]string{"url": source}, &preview)
			return preview, err
		},
		FileAction: func(method string) error {
			if media.attachSource == "" {
				return core.Errorf(tr("unavailable"))
			}
			action := "reveal"
			if method != "file.reveal" {
				return core.Errorf(tr("unavailable"))
			}
			return c.env.Host.CallInto("chat.ghosts.panel-media-action", map[string]string{"uri": media.attachSource, "pageId": media.pageID, "sourcePluginId": media.plugin, "action": action}, nil)
		},
	})
	c.media = media
}

func (c *Controller) MediaDialogs(ctx *ui.Context) {
	media := c.media
	if media == nil {
		return
	}
	open := true
	kit.Dialog(ctx.Key("plugin-media-preview"), &open, kit.DialogOptions{Title: tr("mediaPreview"), Width: 920, Cancel: i18n.T("common.close"), Error: media.err,
		Actions: func() {
			if media.session != "" && media.attachSource != "" && kit.Button(ctx, tr("sendToChat"), icons.Paperclip, media.busy).Disabled(media.busy).Clicked() {
				media.busy = true
				c.AttachMedia(media.attachSource, media.session, media.plugin, media.pageID, "", func(err error) {
					if c.media != media {
						return
					}
					media.busy = false
					if err != nil {
						media.err = err.Error()
					} else {
						c.closeMedia()
					}
				})
			}
		}, Body: func() { ui.Column(ctx).FillWidth().Height(520).MinWidth(0).Children(func() { media.body.View(ctx) }) },
	})
	if !open {
		c.closeMedia()
	}
}

type attachmentRequest struct {
	file model.Attachment
	done func(error)
}

// AttachMedia is called only by a native preview button or OS data-drop path.
// The success callback runs after Chat imports the file and adds it to its
// existing draft, not merely after a ledger lookup has returned metadata.
func (c *Controller) AttachMedia(uri, session, plugin, pageID, sourceToken string, done func(error)) {
	params := map[string]string{"uri": uri, "sessionId": session, "purpose": "attach"}
	if plugin != "" {
		params["sourcePluginId"] = plugin
	}
	if pageID != "" {
		params["pageId"] = pageID
	}
	if sourceToken != "" {
		params["sourceToken"] = sourceToken
	}
	core.Go(c.env, func() (panelMedia, error) {
		var media panelMedia
		err := c.env.Host.CallInto("chat.ghosts.resolve-panel-media", params, &media)
		return media, err
	}, func(media panelMedia, err error) {
		if c.closed {
			return
		}
		if err != nil {
			done(err)
			return
		}
		if c.attachments == nil {
			c.attachments = map[string][]attachmentRequest{}
		}
		c.attachments[session] = append(c.attachments[session], attachmentRequest{file: model.Attachment{Path: media.Path, Name: media.Name, MimeType: media.MIME, Size: media.Size}, done: done})
	})
}

// TakeAttachments transfers ownership to the destination's native import
// path. Its acknowledgement preserves each originating button/drop's result.
func (c *Controller) TakeAttachments(session string) ([]model.Attachment, func(error)) {
	requests := c.attachments[session]
	delete(c.attachments, session)
	var files []model.Attachment
	for _, request := range requests {
		files = append(files, request.file)
	}
	return files, func(err error) {
		for _, request := range requests {
			request.done(err)
		}
	}
}

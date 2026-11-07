package chat

import (
	"context"
	"encoding/base64"
	"mime"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/chat/workspace"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// Match Cindy/Store.ImportRemoteMedia's 100 MiB byte-action budget. The model
// request payload keeps its separate limit; displaying a lawful image must not
// silently impose the older native preview's 64 MiB cap.
const inlineImageLimit = 100 << 20

// Capture the answer's owner before MyGo invokes LoadImage off the UI thread.
// Files keep the existing Host workspace/attachment boundary. Ledger media and
// remote pictures also require Host-issued previews; authored URLs never become
// a local filename or an unrestricted native fetch.
func (p *Chat) markdownImageLoader(key string) func(string) (ui.ImageSource, error) {
	id, directory, method := p.activeID, p.directory(), p.method("file.preview")
	files := append([]model.Attachment(nil), p.attachments()...)
	env := p.env
	return func(source string) (picture ui.ImageSource, err error) {
		defer func() {
			if err != nil {
				env.Post(func() {
					if !p.closed() && p.activeID == id {
						p.setItemError("image:"+key, err.Error())
					}
				})
			}
		}()
		var data []byte
		var imageMIME string
		if strings.HasPrefix(source, "data:") {
			header, encoded, ok := strings.Cut(strings.TrimPrefix(source, "data:"), ",")
			if !ok || len(encoded) > inlineImageLimit*4/3+4 {
				return nil, core.Errorf(i18n.T("chat.fileReadFailed"))
			}
			base64Encoded := strings.HasSuffix(header, ";base64")
			imageMIME, _, err = mime.ParseMediaType(strings.TrimSuffix(header, ";base64"))
			if err != nil || !strings.HasPrefix(imageMIME, "image/") {
				return nil, core.Errorf(i18n.T("chat.fileReadFailed"))
			}
			if base64Encoded {
				data, err = base64.StdEncoding.DecodeString(encoded)
			} else {
				var decoded string
				decoded, err = url.PathUnescape(encoded)
				data = []byte(decoded)
			}
		} else {
			var preview workspace.FilePreview
			switch {
			case strings.HasPrefix(source, "cindy-media://"):
				err = env.Host.CallInto("chat.ghosts.media-preview", map[string]string{"url": source}, &preview)
			case strings.HasPrefix(source, "https://"), strings.HasPrefix(source, "http://"):
				err = env.Host.CallInto(p.method("image.preview"), map[string]string{"id": id, "url": source}, &preview)
			default:
				target := model.TargetOf(source, directory, files)
				if !target.Local() || id == "" {
					return nil, core.Errorf(i18n.T("chat.fileReadFailed"))
				}
				err = env.Host.CallInto(method, map[string]string{"id": id, "path": target.Path}, &preview)
			}
			if err != nil {
				return nil, err
			}
			if preview.Kind != "image" || !strings.HasPrefix(preview.MIME, "image/") {
				return nil, core.Errorf(i18n.T("chat.fileReadFailed"))
			}
			imageMIME = preview.MIME
			data, err = workspace.FetchPreview(context.Background(), preview.URL, inlineImageLimit)
		}
		if err != nil || len(data) > inlineImageLimit {
			if err == nil {
				err = core.Errorf(i18n.T("chat.fileReadFailed"))
			}
			return nil, err
		}
		if imageMIME == "image/svg+xml" {
			return ui.ParseSVG(data)
		}
		return ui.DecodeBitmap(data)
	}
}

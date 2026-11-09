package studybook

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"github.com/egoist/mygo/ui"
	"html"
	"os"
	"path/filepath"
	"strings"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Anki fields retain their full HTML. These native controls edit the actual
// selected text; no lossy plain-text conversion is applied during save.
func (b *Book) fieldTools(c *ui.Context, input ui.Element, name string) {
	ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
		for _, entry := range []struct{ label, left, right string }{{"bold", "<b>", "</b>"}, {"italic", "<i>", "</i>"}, {"underline", "<u>", "</u>"}, {"superscript", "<sup>", "</sup>"}, {"subscript", "<sub>", "</sub>"}, {"cloze", "{{c1::", "}}"}} {
			entry := entry
			if kit.GhostButton(c, text(entry.label), nil, false).Clicked() {
				start, end := input.TextSelection()
				runes := []rune(b.edit.Fields[name])
				start = max(0, min(start, len(runes)))
				end = max(start, min(end, len(runes)))
				value := string(runes[:start]) + entry.left + string(runes[start:end]) + entry.right + string(runes[end:])
				b.edit.Fields[name] = value
				input.SetTextSelection(start+len([]rune(entry.left)), end+len([]rune(entry.left)))
			}
		}
		if kit.GhostButton(c, text("attachMedia"), icons.Paperclip, false).Clicked() {
			b.attachMedia(name)
		}
		if b.recording != nil && b.recording.input == nil {
			r := b.recording
			r.mu.Lock()
			available := len(r.pcm) > 0
			r.mu.Unlock()
			if available && kit.GhostButton(c, text("attachRecording"), icons.Mic, false).Clicked() {
				b.busy = true
				editor := b.editorID
				core.Go(b.env, func() (string, error) {
					if r.done != nil {
						<-r.done
					}
					r.mu.Lock()
					data := wavPCM(append([]byte(nil), r.pcm...))
					r.mu.Unlock()
					sum := sha256.Sum256(data)
					filename := fmt.Sprintf("winone-%x.wav", sum[:12])
					var result struct{ Filename string }
					err := b.request("media.store", map[string]any{"filename": filename, "source": map[string]string{"data": base64.StdEncoding.EncodeToString(data)}}, &result)
					return result.Filename, err
				}, func(filename string, err error) {
					b.busy = false
					if b.closed || b.editorID != editor {
						return
					}
					if err != nil {
						b.err = err.Error()
						return
					}
					b.edit.Fields[name] += "[sound:" + filename + "]"
				})
			}
		}
	})
}
func (b *Book) attachMedia(field string) {
	editor := b.editorID
	core.Go(b.env, func() (string, error) {
		return b.env.Dialogs.PickFile(text("attachMedia"), core.Filter{Name: text("mediaFiles"), Extensions: []string{"png", "jpg", "jpeg", "gif", "webp", "svg", "mp3", "wav", "ogg", "mp4", "webm"}})
	}, func(path string, err error) {
		if b.closed || editor != b.editorID {
			return
		}
		if err != nil {
			b.err = err.Error()
			return
		}
		if path == "" {
			return
		}
		b.busy = true
		core.Go(b.env, func() (string, error) {
			info, err := os.Stat(path)
			if err != nil {
				return "", err
			}
			if !info.Mode().IsRegular() || info.Size() > 64<<20 {
				return "", fmt.Errorf("%s", text("mediaTooLarge"))
			}
			name := filepath.Base(path)
			var result struct{ Filename string }
			err = b.request("media.store", map[string]any{"filename": name, "source": map[string]string{"path": path}, "collision": "rename"}, &result)
			return result.Filename, err
		}, func(name string, err error) {
			b.busy = false
			if b.closed || editor != b.editorID {
				return
			}
			if err != nil {
				b.err = err.Error()
				return
			}
			ext := strings.ToLower(filepath.Ext(name))
			tag := "[sound:" + name + "]"
			if strings.Contains(".png.jpg.jpeg.gif.webp.svg", ext) {
				tag = `<img src="` + html.EscapeString(name) + `">`
			}
			b.edit.Fields[field] += tag
		})
	})
}

// Package workspace contains the native MyGo work-panel components. File I/O
// stays in the existing authenticated Chat Host, including revision conflicts,
// attachment read-only rules and workspace/junction boundaries.
package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"winone/desktopmygo/chat/mathview"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type FilePreview struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	MIME     string `json:"mimeType"`
	URL      string `json:"url"`
	Revision string `json:"revision"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	ReadOnly bool   `json:"readOnly"`
}

// FetchPreview reads a Host-issued loopback URL, not a document-supplied URL.
// Errors intentionally omit the URL, whose query can contain a private grant.
func FetchPreview(ctx context.Context, address string, limit int64) ([]byte, error) {
	target, err := url.Parse(address)
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.User != nil || limit < 0 {
		return nil, errors.New(i18n.T("chat.fileReadFailed"))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errors.New(i18n.T("chat.fileReadFailed"))
	}
	response, err := (&http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
	if err != nil {
		return nil, errors.New(i18n.T("chat.fileReadFailed"))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s (HTTP %d)", i18n.T("chat.fileReadFailed"), response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New(i18n.T("chat.fileReadFailed"))
	}
	return data, nil
}

type TextDocumentOptions struct {
	InitialPreview *FilePreview
	// Prefix is the existing transport scope, "chat." or "chat.onebot.chat.".
	Prefix          string
	SessionID, Path string
	Line            int
	OnDirty         func(bool)
	OnLink          func(string)
	Preview         func() (FilePreview, error)
	FileAction      func(string) error
}

// TextDocument ports ChatFilePreview/FileDocument using MyGo's actual editor.
// It keeps undo, selections, viewport and a snapshot per accepted save. A late
// save or external refresh never marks newer typing saved or replaces it.
type TextDocument struct {
	env                     *core.Env
	options                 TextDocumentOptions
	ctx                     context.Context
	cancel                  context.CancelFunc
	initialPreview          *FilePreview
	file                    FilePreview
	state                   editor.State
	baseline                editor.Snapshot
	cachedVersion           uint64
	observedVersion         uint64
	cachedDirty             bool
	loading, saving, closed bool
	loaded                  bool
	generation              uint64
	err                     string
	rendered                bool
	markdown                markdown.Options
	autosave                *time.Timer
	refresh                 *time.Timer
	activeWaiters           []func(error)
	waiters                 []func(error)
	saveAgain               bool
}

func NewTextDocument(env *core.Env, options TextDocumentOptions) *TextDocument {
	ctx, cancel := context.WithCancel(context.Background())
	d := &TextDocument{env: env, options: options, ctx: ctx, cancel: cancel, rendered: options.Line == 0 && markdownFile(options.Path), initialPreview: options.InitialPreview}
	d.markdown.OnLink = options.OnLink
	d.Reload()
	return d
}
func markdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}
func (d *TextDocument) Buffer() *editor.Buffer { return d.state.Buffer }
func (d *TextDocument) Metadata() FilePreview  { return d.file }
func (d *TextDocument) Error() string          { return d.err }
func (d *TextDocument) Busy() bool             { return d.loading || d.saving }
func (d *TextDocument) Dirty() bool {
	if d.state.Buffer == nil {
		return false
	}
	current := d.state.Buffer.Snapshot()
	if current.Version() != d.cachedVersion {
		d.cachedVersion = current.Version()
		d.cachedDirty = current.Version() != d.baseline.Version() && current.String() != d.baseline.String()
	}
	return d.cachedDirty
}
func (d *TextDocument) reportDirty() {
	if d.options.OnDirty != nil {
		d.options.OnDirty(d.Dirty())
	}
}
func (d *TextDocument) Reload() {
	if d.closed || d.loading || d.saving {
		return
	}
	if d.Dirty() {
		d.err = i18n.T("chat.unsavedFile")
		return
	}
	d.loading = true
	d.generation++
	generation := d.generation
	type result struct {
		file FilePreview
		text string
	}
	initial := d.initialPreview
	d.initialPreview = nil
	core.Go(d.env, func() (result, error) {
		var value result
		if initial != nil {
			value.file = *initial
		} else if d.options.Preview != nil {
			var err error
			value.file, err = d.options.Preview()
			if err != nil {
				return value, err
			}
		} else if err := d.env.Host.CallInto(d.options.Prefix+"file.preview", map[string]string{"id": d.options.SessionID, "path": d.options.Path}, &value.file); err != nil {
			return value, err
		}
		if value.file.Kind != "text" {
			return value, errors.New(i18n.T("chat.previewUnsupported"))
		}
		data, err := FetchPreview(d.ctx, value.file.URL, 10<<20)
		value.text = string(data)
		return value, err
	}, func(value result, err error) {
		if d.closed || generation != d.generation {
			return
		}
		d.loading = false
		if err != nil {
			d.err = err.Error()
			return
		}
		// A user can edit an already-open buffer while a visible-file refresh reads.
		if d.Dirty() {
			return
		}
		d.file = value.file
		d.state.Buffer = editor.NewBuffer(value.text)
		d.state.Buffer.SetLanguage(editor.LanguageFor(d.options.Path))
		d.state.SoftWrap = true
		d.state.ReadOnly = value.file.ReadOnly
		d.state.Label = value.file.Name
		d.baseline = d.state.Buffer.Snapshot()
		d.cachedVersion = 0
		d.observedVersion = 0
		d.loaded = true
		d.err = ""
		if d.options.Line > 0 {
			d.state.GoTo(editor.Pos{Line: d.options.Line - 1})
		}
		d.reportDirty()
	})
}

// Save serializes explicit saves and Markdown autosave. An additional save
// while one is pending captures the latest buffer only after the first receipt.
func (d *TextDocument) Save(done func(error)) {
	if d.closed {
		if done != nil {
			done(context.Canceled)
		}
		return
	}
	if d.autosave != nil {
		d.autosave.Stop()
	}
	if done != nil {
		d.waiters = append(d.waiters, done)
	}
	if d.saving {
		d.saveAgain = true
		return
	}
	if !d.loaded || d.file.ReadOnly || !d.Dirty() {
		d.finishWaiters(nil)
		return
	}
	snapshot := d.state.Buffer.Snapshot()
	var content bytes.Buffer
	if _, err := snapshot.WriteTo(&content); err != nil {
		d.finishWaiters(err)
		return
	}
	text, revision := content.String(), d.file.Revision
	d.activeWaiters = d.waiters
	d.waiters = nil
	d.saving = true
	d.err = ""
	core.Go(d.env, func() (FilePreview, error) {
		var result FilePreview
		err := d.env.Host.CallInto(d.options.Prefix+"file.save", map[string]string{"id": d.options.SessionID, "path": d.options.Path, "revision": revision, "text": text}, &result)
		return result, err
	}, func(result FilePreview, err error) {
		if d.closed {
			return
		}
		d.saving = false
		if err == nil {
			result.URL = d.file.URL
			d.file = result
			d.baseline = snapshot
			d.cachedVersion = 0
			if !d.Dirty() {
				d.state.Buffer.MarkSaved()
			}
		} else {
			d.err = err.Error()
		}
		d.reportDirty()
		active := d.activeWaiters
		d.activeWaiters = nil
		for _, done := range active {
			done(err)
		}
		if err != nil {
			d.saveAgain = false
			d.finishWaiters(err)
			return
		}
		if d.saveAgain {
			d.saveAgain = false
			d.Save(nil)
		}
	})
}
func (d *TextDocument) finishWaiters(err error) {
	waiters := d.waiters
	d.waiters = nil
	for _, done := range waiters {
		done(err)
	}
}
func (d *TextDocument) Close() {
	if d.closed {
		return
	}
	d.closed = true
	d.generation++
	d.cancel()
	if d.autosave != nil {
		d.autosave.Stop()
	}
	if d.refresh != nil {
		d.refresh.Stop()
	}
	active := d.activeWaiters
	d.activeWaiters = nil
	for _, done := range active {
		done(context.Canceled)
	}
	d.finishWaiters(context.Canceled)
}

// Observe is called only for the active native tab. It shares the source
// viewer's 900 ms Markdown autosave, without polling hidden file contents.
func (d *TextDocument) Observe() {
	if d.closed || d.state.Buffer == nil {
		return
	}
	version := d.state.Buffer.Version()
	changed := version != d.observedVersion
	d.observedVersion = version
	dirty := d.Dirty()
	d.reportDirty()
	if changed && markdownFile(d.options.Path) && !d.file.ReadOnly {
		if d.autosave != nil {
			d.autosave.Stop()
		}
		if dirty {
			d.autosave = time.AfterFunc(900*time.Millisecond, func() {
				d.env.Post(func() {
					if !d.closed {
						d.Save(nil)
					}
				})
			})
		}
	}
}
func (d *TextDocument) View(c *ui.Context) {
	if d.closed {
		return
	}
	d.Observe()
	root := ui.Column(c).Fill().MinWidth(0).Gap(6)
	root.Children(func() {
		ui.Row(c).FillWidth().Gap(6).Padding(8).Wrap().Children(func() {
			if markdownFile(d.options.Path) {
				label := i18n.T("chat.fileRendered")
				if d.rendered {
					label = i18n.T("chat.fileSource")
				}
				if kit.Button(c, label, icons.Code, false).Clicked() {
					d.rendered = !d.rendered
				}
			}
			if kit.Button(c, i18n.T("chat.saveFile"), icons.Save, d.saving).Disabled(!d.loaded || d.file.ReadOnly || !d.Dirty()).Clicked() {
				d.Save(nil)
			}
			if kit.IconButton(c, icons.RefreshCw, i18n.T("chat.refresh")).Disabled(d.loading || d.Dirty()).Clicked() {
				d.Reload()
			}
			if kit.IconButton(c, icons.Copy, i18n.T("chat.copy")).Disabled(!d.loaded).Clicked() {
				c.Services().WriteClipboard(d.state.Buffer.String())
			}
			if kit.IconButton(c, icons.FolderOpen, i18n.T("chat.revealFile")).Clicked() {
				core.Do(d.env, func() error {
					if d.options.FileAction != nil {
						return d.options.FileAction("file.reveal")
					}
					return d.env.Host.CallInto(d.options.Prefix+"file.reveal", map[string]string{"id": d.options.SessionID, "path": d.options.Path}, nil)
				}, func(err error) {
					if !d.closed && err != nil {
						d.err = err.Error()
					}
				})
			}
		})
		kit.InlineError(c, d.err)
		if d.loading && !d.loaded {
			kit.Loader(c)
		}
		if !d.loaded {
			return
		}
		if d.rendered && markdownFile(d.options.Path) {
			ui.Scroll(c).Grow(1).MinHeight(0).FillWidth().Padding(12).Children(func() { markdown.View(c, d.state.Buffer.String(), mathview.For(d.env).Apply(&d.markdown)).FillWidth() })
		} else {
			editor.View(c, &d.state).Grow(1).FillWidth().MinHeight(0)
		}
	})
	if root.Shortcut(ui.Ctrl, ui.KeyS) {
		d.Save(nil)
	}
}

// DecodePreviewJSON reads a scoped file-browser response from the Host's data
// URL. It keeps all path authorization on the Host, not in a parallel local FS.
func DecodePreviewJSON(ctx context.Context, env *core.Env, prefix, method string, params, out any) error {
	var location struct {
		URL string `json:"url"`
	}
	if err := env.Host.CallInto(prefix+method, params, &location); err != nil {
		return err
	}
	data, err := FetchPreview(ctx, location.URL, 32<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

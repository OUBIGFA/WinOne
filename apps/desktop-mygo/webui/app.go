package webui

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// app is the bound service App: the methods of the Rust window's bridge
// (apps/desktop-rust/src/component_bridge.rs and component_ui.rs) that the
// Chat and OneBot React code calls, with the same limits. Only a Surface's
// page may call them; each call runs on its own goroutine.
type app struct{}

func (app) env(ctx context.Context) (*Surface, *core.Env, error) {
	s, err := caller(ctx)
	if err != nil {
		return nil, nil, err
	}
	return s, s.env, nil
}

// Chat calls the Host's chat methods the page may call, and returns the
// result as JSON text, as the bridge did. Export stays with the native
// page, which owns its save dialog.
func (a app) Chat(ctx context.Context, method, payload string) (string, error) {
	s, env, err := a.env(ctx)
	if err != nil {
		return "", err
	}
	if method == "export" || chatMutates(method) {
		defer s.mutating()()
	}
	if method == "export" {
		receipt, err := ExportTask(env, json.RawMessage(params(payload)))
		return string(receipt), err
	}
	if !chatAllowed(method) || method == "onebot.chat.export" {
		return "", errors.New(i18n.T("chat.unsupportedOperation"))
	}
	result, err := env.Host.Call("chat."+method, json.RawMessage(params(payload)))
	return string(result), err
}

// Capture reads what OneBot's pages read of the AI services.
func (a app) Capture(ctx context.Context, method, payload string) (string, error) {
	_, env, err := a.env(ctx)
	if err != nil {
		return "", err
	}
	if method != "ai.configuration.get" {
		return "", errors.New(i18n.T("chat.unsupportedOperation"))
	}
	result, err := env.Host.Call("capture."+method, json.RawMessage(params(payload)))
	return string(result), err
}

// params is a request's JSON parameters; an empty payload sends none.
func params(payload string) string {
	if strings.TrimSpace(payload) == "" {
		return "null"
	}
	return payload
}

// ChatBrowser drives the side panel's web pages.
func (a app) ChatBrowser(ctx context.Context, method, payload string) (string, error) {
	s, _, err := a.env(ctx)
	if err != nil {
		return "", err
	}
	if s.o.Browser == nil {
		return "", errors.New(i18n.T("chat.unsupportedOperation"))
	}
	var result any
	var callErr error
	if err := s.onUI(ctx, func() { result, callErr = s.o.Browser.request(s, method, json.RawMessage(params(payload))) }); err != nil {
		return "", err
	}
	if callErr != nil {
		return "", callErr
	}
	data, err := json.Marshal(result)
	return string(data), err
}

// ChatMedia copies or saves an image or a file of a conversation, which
// the Host resolves inside the conversation's boundary: the page never
// names an arbitrary path.
func (a app) ChatMedia(ctx context.Context, method, payload string) (string, error) {
	s, env, err := a.env(ctx)
	if err != nil {
		return "", err
	}
	defer s.mutating()()
	var p struct {
		ID            string `json:"id"`
		SuggestedName string `json:"suggestedName"`
		PlainText     string `json:"plainText"`
		OriginalPath  string `json:"originalPath"`
	}
	if err := json.Unmarshal([]byte(params(payload)), &p); err != nil {
		return "", err
	}
	describe := "chat.file.preview"
	group := strings.HasPrefix(p.ID, "bot-group:")
	if group {
		describe = "chat.onebot.groups.files.describe"
	}
	var file struct {
		Kind         string `json:"kind"`
		Name         string `json:"name"`
		Path         string `json:"path"`
		ResolvedPath string `json:"resolvedPath"`
		DownloadOnly bool   `json:"downloadOnly"`
	}
	read := func(method string) error {
		return env.Host.CallInto(method, json.RawMessage(params(payload)), &file)
	}
	switch method {
	case "save-file", "copy-file":
		resolve := "chat.file.resolve"
		if group {
			resolve = describe
		}
		if err := read(resolve); err != nil {
			return "", err
		}
		if file.Path == "" {
			return "", errors.New(i18n.T("chat.mediaInvalid"))
		}
		if method == "copy-file" {
			if file.DownloadOnly {
				return "", errors.New(i18n.T("chat.mediaSaveOnly"))
			}
			data, err := transfer.FileData(file.Path)
			if err != nil {
				return "", err
			}
			if err := mygo.Clipboard.Write(data); err != nil {
				return "", err
			}
			return `{"success":true}`, nil
		}
		name := first(p.SuggestedName, file.Name, "attachment")
		return saveCopy(env, file.Path, i18n.T("chat.saveAttachment"), core.Filter{}, name)
	case "copy", "save-as":
	default:
		return "", errors.New(i18n.T("chat.mediaUnsupported"))
	}
	if err := read(describe); err != nil {
		return "", err
	}
	if file.Kind != "image" {
		return "", errors.New(i18n.T("chat.mediaNotImage"))
	}
	path := first(file.ResolvedPath, file.Path)
	if method == "save-as" {
		name := first(file.Name, "image.png")
		ext := strings.TrimPrefix(filepath.Ext(name), ".")
		if ext == "" {
			ext = "png"
		}
		return saveCopy(env, path, i18n.T("chat.saveImage"), core.Filter{Name: "Image", Extensions: []string{ext}}, name)
	}
	png, err := readPNG(path)
	if err != nil {
		return "", err
	}
	formats := []transfer.Representation{transfer.Bytes(transfer.PNG, png)}
	if p.PlainText != "" {
		formats = append(formats, transfer.Bytes(transfer.Text, []byte(p.PlainText)))
	}
	items := []transfer.Item{transfer.NewItem(formats...)}
	if p.OriginalPath != "" {
		// The original file goes with the image, as the Rust bridge's
		// CF_HDROP did, once the Host shows it is an image of the chat.
		var original struct {
			Kind string `json:"kind"`
			Path string `json:"path"`
		}
		if err := env.Host.CallInto("chat.file.preview", map[string]any{"id": p.ID, "path": p.OriginalPath}, &original); err != nil {
			return "", err
		}
		if original.Kind != "image" {
			return "", errors.New(i18n.T("chat.mediaOriginalNotImage"))
		}
		files, err := transfer.FileData(original.Path)
		if err != nil {
			return "", err
		}
		items = append(items, files.Items()...)
	}
	if err := mygo.Clipboard.Write(transfer.New(items...)); err != nil {
		return "", err
	}
	return `{"success":true}`, nil
}

// readPNG reads an exported PNG within the limits the Rust bridge kept: at
// most 72 MiB, 16384 pixels a side and 4096×4096 pixels and a margin.
func readPNG(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 72<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) < 24 || len(data) > 72<<20 {
		return nil, errors.New(i18n.T("chat.mediaPNGSize"))
	}
	width, height := uint64(binary.BigEndian.Uint32(data[16:20])), uint64(binary.BigEndian.Uint32(data[20:24]))
	if width > 16384 || height > 16384 || width*height > 4096*4096+16384 {
		return nil, errors.New(i18n.T("chat.mediaPNGDimensions"))
	}
	return data, nil
}

// saveCopy asks where to save a copy of path and copies it there.
func saveCopy(env *core.Env, path, title string, filter core.Filter, name string) (string, error) {
	target, err := env.Dialogs.SaveFile(title, filter, name)
	if err != nil {
		return "", err
	}
	if target == "" {
		return `{"canceled":true}`, nil
	}
	if err := copyFile(path, target); err != nil {
		return "", err
	}
	data, err := json.Marshal(map[string]any{"canceled": false, "path": target})
	return string(data), err
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// PickDirectory asks for a folder; "" when cancelled.
func (a app) PickDirectory(ctx context.Context) (string, error) {
	_, env, err := a.env(ctx)
	if err != nil {
		return "", err
	}
	return env.Dialogs.PickFolder(i18n.T("chat.chooseFolder"))
}

// PickDirectories asks for folders.
func (a app) PickDirectories(ctx context.Context) ([]string, error) {
	_, env, err := a.env(ctx)
	if err != nil {
		return nil, err
	}
	return env.Dialogs.PickFolders(i18n.T("chat.chooseFolders"))
}

// PickFile asks for a file; "" when cancelled.
func (a app) PickFile(ctx context.Context) (string, error) {
	_, env, err := a.env(ctx)
	if err != nil {
		return "", err
	}
	return env.Dialogs.PickFile(i18n.T("chat.chooseFile"), core.Filter{})
}

// PickChatFiles asks for attachments.
func (a app) PickChatFiles(ctx context.Context) ([]string, error) {
	_, env, err := a.env(ctx)
	if err != nil {
		return nil, err
	}
	return env.Dialogs.PickFiles(i18n.T("chat.uploadAttachment"), core.Filter{})
}

// droppedPath is a dropped path and what it is.
type droppedPath struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// InspectDroppedPaths tells files from folders before a drop zone takes
// them; a path that cannot be read fails rather than passing as either.
func (a app) InspectDroppedPaths(ctx context.Context, paths []string) ([]droppedPath, error) {
	if _, _, err := a.env(ctx); err != nil {
		return nil, err
	}
	if len(paths) == 0 || len(paths) > 1024 {
		return nil, errors.New(i18n.T("chat.dropInvalid"))
	}
	entries := make([]droppedPath, 0, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, errors.New(i18n.T("chat.dropAbsolute"))
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		kind := "file"
		switch {
		case info.IsDir():
			kind = "directory"
		case !info.Mode().IsRegular():
			return nil, errors.New(i18n.T("chat.dropInvalid"))
		}
		entries = append(entries, droppedPath{Path: path, Kind: kind})
	}
	return entries, nil
}

// SetFileDropTarget is what drop zones tell the window as they mount. MyGo
// delivers every drop to the page, so there is nothing to change.
func (a app) SetFileDropTarget(ctx context.Context, enabled bool) error {
	_, _, err := a.env(ctx)
	return err
}

// Embed carries the page's requests of the native page that shows it.
func (a app) Embed(ctx context.Context, kind, payload string) (string, error) {
	s, _, err := a.env(ctx)
	if err != nil {
		return "", err
	}
	return s.embed(ctx, kind, payload)
}

// copyPNG puts a screenshot on the clipboard.
func copyPNG(data []byte) error {
	return mygo.Clipboard.Write(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, data))))
}

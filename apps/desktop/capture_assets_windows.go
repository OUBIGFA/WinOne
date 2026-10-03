//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

var capturePreviewPath = regexp.MustCompile(`^/capture-preview/([a-f0-9]{32})\.png$`)
var captureEditPreviewPath = regexp.MustCompile(`^/capture-edit-preview/([a-f0-9]{32})\.png$`)
var captureSourcePath = regexp.MustCompile(`^/capture-source/([a-f0-9]{32})\.png$`)

// Open with delete sharing so a completed image fetch cannot prevent the worker
// from replacing metadata or removing an obsolete preview.
func openCaptureAsset(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

func readCaptureMetadata(directory, name string, target any) error {
	file, err := openCaptureAsset(filepath.Join(directory, name))
	if err != nil {
		return err
	}
	defer file.Close()
	limit := int64(256 * 1024)
	if name == "capture-draft.json" {
		limit = 768 * 1024
	} else if name == "retired-drafts.json" {
		limit = 64 * 1024
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return io.ErrShortBuffer
	}
	if name == "capture-draft.json" {
		var draft struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
		}
		if err := json.Unmarshal(data, &draft); err != nil {
			return err
		}
		var retired []struct {
			ID                string `json:"id"`
			CompletedRevision *int64 `json:"completedRevision"`
		}
		if err := readCaptureMetadata(directory, "retired-drafts.json", &retired); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if len(retired) > 64 {
			return errors.New("too many retired capture records")
		}
		for _, entry := range retired {
			if entry.CompletedRevision != nil && *entry.CompletedRevision < 0 {
				return errors.New("invalid completed capture revision")
			}
			// Completion is durable even if a scanner temporarily prevents the
			// worker from deleting metadata. Do not reopen or serve that draft.
			if entry.ID == draft.ID && entry.CompletedRevision != nil && *entry.CompletedRevision == draft.Revision {
				return os.ErrNotExist
			}
		}
	}
	return json.Unmarshal(data, target)
}

// Assets are derived from current IDs only, never filesystem paths from the UI
// or metadata. Full-size source access is limited to the active editing draft.
func captureAssets(directory string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Allow", "GET, HEAD")
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		match := capturePreviewPath.FindStringSubmatch(request.URL.Path)
		editing := false
		source := false
		if match == nil {
			match = captureEditPreviewPath.FindStringSubmatch(request.URL.Path)
			editing = true
		}
		if match == nil {
			match = captureSourcePath.FindStringSubmatch(request.URL.Path)
			source, editing = true, false
		}
		if match == nil {
			http.NotFound(writer, request)
			return
		}
		var current struct {
			ID    string `json:"id"`
			Ready bool   `json:"previewReady"`
		}
		currentValid := readCaptureMetadata(directory, "last-capture.json", &current) == nil && current.ID != ""
		var draft struct {
			ID               string `json:"id"`
			Ready            bool   `json:"previewReady"`
			DesktopPreviewID string `json:"desktopPreviewId"`
			Width            int    `json:"width"`
			Height           int    `json:"height"`
		}
		draftValid := readCaptureMetadata(directory, "capture-draft.json", &draft) == nil && draft.ID != ""
		if source {
			if !draftValid || draft.ID != match[1] || draft.Width < 1 || draft.Height < 1 {
				http.NotFound(writer, request)
				return
			}
		} else if editing {
			var preview struct {
				ID       string `json:"id"`
				SourceID string `json:"sourceId"`
			}
			if readCaptureMetadata(directory, "edit-preview.json", &preview) != nil || preview.ID != match[1] || preview.SourceID == "" || !(currentValid && preview.SourceID == current.ID || draftValid && preview.SourceID == draft.ID) {
				http.NotFound(writer, request)
				return
			}
		} else if !(currentValid && current.ID == match[1] && current.Ready || draftValid && draft.Ready && (draft.ID == match[1] || draft.DesktopPreviewID == match[1])) {
			http.NotFound(writer, request)
			return
		}
		root := filepath.Join(directory, "previews")
		if source {
			root = filepath.Join(directory, "drafts")
		}
		rootInfo, err := os.Lstat(root)
		if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
			http.NotFound(writer, request)
			return
		}
		path := filepath.Join(root, match[1]+".png")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(writer, request)
			return
		}
		file, err := openCaptureAsset(path)
		if err != nil {
			http.NotFound(writer, request)
			return
		}
		defer file.Close()
		info, err = file.Stat()
		maximumBytes := int64(16 << 20)
		if source || editing {
			maximumBytes = 512 << 20
		}
		if err != nil || info.Size() > maximumBytes {
			writer.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		config, err := png.DecodeConfig(file)
		if err != nil {
			http.NotFound(writer, request)
			return
		}
		if source && (config.Width != draft.Width || config.Height != draft.Height) {
			http.NotFound(writer, request)
			return
		}
		// Keep thumbnail responses small. Editing images retain physical pixels,
		// with a separate decoded-size cap checked before handing them to WebView.
		tooLarge := config.Width > 1600 || config.Height > 1600
		if source || editing {
			tooLarge = int64(config.Width)*int64(config.Height) > 128*1024*1024
		}
		if tooLarge {
			writer.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "image/png")
		writer.Header().Set("Cache-Control", "private, no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		if request.Method == http.MethodGet {
			_, _ = io.Copy(writer, file)
		}
	})
}

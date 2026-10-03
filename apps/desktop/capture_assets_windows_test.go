//go:build windows

package main

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureSourceServesOnlyMatchingActiveDraftPixels(t *testing.T) {
	directory := t.TempDir()
	id := strings.Repeat("a", 32)
	if err := os.MkdirAll(filepath.Join(directory, "drafts"), 0700); err != nil {
		t.Fatal(err)
	}
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 2001, 2))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "drafts", id+".png"), pixels.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(directory, "capture-draft.json")
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(metadata, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(want int) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		captureAssets(directory).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/capture-source/"+id+".png", nil))
		if response.Code != want {
			t.Fatalf("source status %d, want %d", response.Code, want)
		}
		return response
	}
	write(`{"id":"` + id + `","width":2001,"height":2}`)
	response := check(http.StatusOK)
	if !bytes.Equal(response.Body.Bytes(), pixels.Bytes()) {
		t.Fatal("source pixels were changed")
	}
	write(`{"id":"` + id + `","width":2000,"height":2}`)
	check(http.StatusNotFound)
	write(`{"id":"` + strings.Repeat("b", 32) + `","width":2001,"height":2}`)
	check(http.StatusNotFound)
}

func TestDesktopBackgroundAssetRequiresCurrentDraft(t *testing.T) {
	directory := t.TempDir()
	id, background := strings.Repeat("a", 32), strings.Repeat("b", 32)
	if err := os.MkdirAll(filepath.Join(directory, "previews"), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, "previews", background+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	file.Close()
	metadata := filepath.Join(directory, "capture-draft.json")
	if err = os.WriteFile(metadata, []byte(`{"id":"`+id+`","previewReady":true,"desktopPreviewId":"`+background+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	handler := captureAssets(directory)
	check := func(want int) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/capture-preview/"+background+".png", nil))
		if response.Code != want {
			t.Fatalf("got %d, want %d", response.Code, want)
		}
	}
	check(http.StatusOK)
	if err = os.WriteFile(metadata, []byte(`{"id":"`+id+`","previewReady":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	check(http.StatusNotFound)
}

func TestCaptureEditAssetsRequireLatestPreviewAndCurrentSource(t *testing.T) {
	directory := t.TempDir()
	sourceID, previewID := strings.Repeat("a", 32), strings.Repeat("b", 32)
	if err := os.MkdirAll(filepath.Join(directory, "previews"), 0700); err != nil {
		t.Fatal(err)
	}
	writeMetadata := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeMetadata("last-capture.json", `{"id":"`+sourceID+`"}`)
	writeMetadata("edit-preview.json", `{"id":"`+previewID+`","sourceId":"`+sourceID+`"}`)
	file, err := os.Create(filepath.Join(directory, "previews", previewID+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	file.Close()
	handler := captureAssets(directory)
	check := func(path string, want int) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != want {
			t.Fatalf("%s: got %d want %d", path, response.Code, want)
		}
	}
	path := "/capture-edit-preview/" + previewID + ".png"
	check(path, http.StatusOK)
	check("/capture-preview/"+previewID+".png", http.StatusNotFound)
	writeMetadata("edit-preview.json", `{"id":"`+strings.Repeat("c", 32)+`","sourceId":"`+sourceID+`"}`)
	check(path, http.StatusNotFound)
	writeMetadata("edit-preview.json", `{"id":"`+previewID+`","sourceId":"`+sourceID+`"}`)
	writeMetadata("last-capture.json", `{"id":"`+strings.Repeat("d", 32)+`"}`)
	check(path, http.StatusNotFound)
}

func TestCaptureAssetsOnlyServeCurrentBoundedPreview(t *testing.T) {
	directory := t.TempDir()
	id := strings.Repeat("a", 32)
	if err := os.MkdirAll(filepath.Join(directory, "previews"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "last-capture.json"), []byte(`{"id":"`+id+`","previewReady":true,"path":"must-not-be-used.png"}`), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, "previews", id+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	handler := captureAssets(directory)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/capture-preview/"+id+".png", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" || !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
		t.Fatal(response.Code, response.Header())
	}
	decoded, err := png.Decode(response.Body)
	if err != nil || decoded.Bounds().Dx() != 4 {
		t.Fatal(err)
	}
	for _, path := range []string{"/capture-preview/" + strings.Repeat("b", 32) + ".png", "/capture-preview/../../last-capture.json"} {
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatal("unexpected asset access", path, response.Code)
		}
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/capture-preview/"+id+".png", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatal(response.Code)
	}
	file, err = os.Create(filepath.Join(directory, "previews", id+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, image.NewRGBA(image.Rect(0, 0, 1601, 1))); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/capture-preview/"+id+".png", nil))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatal("oversized preview was served", response.Code)
	}
}

func TestCaptureAssetsAllowOnlyTheActiveDraft(t *testing.T) {
	directory := t.TempDir()
	id := strings.Repeat("c", 32)
	if err := os.MkdirAll(filepath.Join(directory, "previews"), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, "previews", id+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	file.Close()
	metadata := filepath.Join(directory, "capture-draft.json")
	write := func(data string) {
		t.Helper()
		if err := os.WriteFile(metadata, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(want int) {
		t.Helper()
		response := httptest.NewRecorder()
		captureAssets(directory).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/capture-preview/"+id+".png", nil))
		if response.Code != want {
			t.Fatal(response.Code, want)
		}
	}
	write(`{"id":"` + id + `","previewReady":true,"document":{"text":"` + strings.Repeat("x", 300*1024) + `"}}`)
	check(http.StatusOK)
	write(`{"id":"` + id + `","previewReady":"invalid"}`)
	check(http.StatusNotFound)
	write(`{"id":"` + strings.Repeat("d", 32) + `","previewReady":true}`)
	check(http.StatusNotFound)
}

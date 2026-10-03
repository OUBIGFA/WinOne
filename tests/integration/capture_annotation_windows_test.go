//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/transport"
)

func TestCaptureAnnotationExportsOriginalResolution(t *testing.T) {
	testCaptureAnnotationExport(t, false, false)
}

func TestCaptureAnnotationExportsCrop(t *testing.T) {
	testCaptureAnnotationExport(t, true, false)
}

func TestCaptureAnnotationPreviewKeepsSource(t *testing.T) {
	testCaptureAnnotationExport(t, true, true)
}

func testCaptureAnnotationExport(t *testing.T, crop, preview bool) {
	t.Helper()
	exe := os.Getenv("WINONE_CAPTURE_EXE")
	if exe == "" {
		t.Skip("set WINONE_CAPTURE_EXE")
	}
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.png")
	targetPath := filepath.Join(directory, "edited.png")
	source := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			source.Set(x, y, color.RGBA{R: 90, G: 100, B: 110, A: 255})
		}
	}
	file, err := os.Create(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, source); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("1", 32)
	metadata, _ := json.Marshal(map[string]any{"id": id, "path": sourcePath, "width": 100, "height": 80, "copied": false, "pinned": false})
	if err = os.WriteFile(filepath.Join(directory, "last-capture.json"), metadata, 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-annotation-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	call := func(method string, params any, out any) error {
		data, _ := json.Marshal(params)
		response, err := transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "annotation", Method: method, Params: data})
		if err != nil {
			return err
		}
		if response.Error != nil {
			return fmt.Errorf("%s", response.Error.Message)
		}
		return json.Unmarshal(response.Result, out)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var state any
		if call("snapshot", nil, &state) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	var job struct {
		ID     string `json:"id"`
		State  string `json:"state"`
		Error  string `json:"error"`
		Result struct {
			ID       string `json:"id"`
			SourceID string `json:"sourceId"`
			Width    int    `json:"width"`
			Height   int    `json:"height"`
		} `json:"result"`
	}
	// Shared serializer metadata must retain strict schema validation across
	// requests, including after a rejected document.
	invalid := map[string]any{"path": targetPath, "expectedId": id, "document": map[string]any{"version": 1, "width": 100, "height": 80, "marks": []any{}, "unexpectedField": true}}
	if err = call("edit.apply", invalid, &job); err == nil {
		t.Fatal("unknown annotation field accepted")
	}
	params := map[string]any{"path": targetPath, "expectedId": id, "document": map[string]any{"version": 1, "width": 100, "height": 80, "marks": []map[string]any{{"kind": "cover", "x": 10, "y": 10, "width": 20, "height": 20, "color": "#000000"}}}}
	if crop {
		document := params["document"].(map[string]any)
		document["version"] = 2
		document["crop"] = map[string]int{"x": 15, "y": 15, "width": 40, "height": 30}
	}
	method := "edit.apply"
	if preview {
		method = "edit.preview"
		delete(params, "path")
	}
	if err = call(method, params, &job); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for job.State == "running" {
		if time.Now().After(deadline) {
			t.Fatal("render timed out")
		}
		time.Sleep(50 * time.Millisecond)
		if err = call("jobs.get", map[string]string{"id": job.ID}, &job); err != nil {
			t.Fatal(err)
		}
	}
	if job.State != "completed" {
		t.Fatal(job)
	}
	if preview {
		if len(job.Result.ID) != 32 || job.Result.SourceID != id || job.Result.Width != 40 || job.Result.Height != 30 {
			t.Fatal("unexpected edit preview identity", job.Result)
		}
		targetPath = filepath.Join(directory, "previews", job.Result.ID+".png")
		data, readErr := os.ReadFile(filepath.Join(directory, "last-capture.json"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var last struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		}
		if json.Unmarshal(data, &last) != nil || last.ID != id || last.Path != sourcePath {
			t.Fatal("edit preview replaced source metadata")
		}
	}
	file, err = os.Open(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	output, err := png.Decode(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	wantWidth, wantHeight := 100, 80
	markedPoint, untouchedPoint := image.Pt(20, 20), image.Pt(70, 60)
	if crop {
		wantWidth, wantHeight = 40, 30
		markedPoint, untouchedPoint = image.Pt(0, 0), image.Pt(20, 20)
	}
	if output.Bounds().Dx() != wantWidth || output.Bounds().Dy() != wantHeight || color.RGBAModel.Convert(output.At(markedPoint.X, markedPoint.Y)) != (color.RGBA{A: 255}) || color.RGBAModel.Convert(output.At(untouchedPoint.X, untouchedPoint.Y)) != (color.RGBA{R: 90, G: 100, B: 110, A: 255}) {
		t.Fatal("annotation export changed resolution or unrelated pixels")
	}
}

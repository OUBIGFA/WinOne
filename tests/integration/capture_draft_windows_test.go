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

type captureDraftJob struct {
	ID     string          `json:"id"`
	State  string          `json:"state"`
	Error  string          `json:"error"`
	Result json.RawMessage `json:"result"`
}

func TestCaptureDraftSurvivesRestartAndCompletes(t *testing.T) {
	exe := os.Getenv("WINONE_CAPTURE_EXE")
	if exe == "" {
		t.Skip("set WINONE_CAPTURE_EXE")
	}
	directory := t.TempDir()
	id := strings.Repeat("1", 32)
	previousID := strings.Repeat("2", 32)
	source := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			source.Set(x, y, color.RGBA{R: 80, G: 90, B: 100, A: 255})
		}
	}
	writeImage := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(file, source); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON := func(name string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	previousPath := filepath.Join(directory, "previous.png")
	writeImage(previousPath)
	writeImage(filepath.Join(directory, "drafts", id+".png"))
	writeImage(filepath.Join(directory, "previews", id+".png"))
	writeJSON("last-capture.json", map[string]any{"id": previousID, "path": previousPath, "width": 32, "height": 24})
	writeJSON("capture-draft.json", map[string]any{"schemaVersion": 1, "id": id, "revision": 0, "width": 32, "height": 24, "previewReady": true, "sourceKind": "scroll", "replaces": map[string]any{"id": previousID, "revision": 7}, "settings": map[string]any{"outputDirectory": directory, "copyToClipboard": false, "askWhereToSave": false, "openEditor": true}, "document": map[string]any{"version": 1, "width": 32, "height": 24, "marks": []any{}}})

	start := func() (func(string, any, any) error, func()) {
		t.Helper()
		pipe := fmt.Sprintf(`\\.\pipe\WinOne-draft-test-%d`, time.Now().UnixNano())
		cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		stopped := false
		stop := func() {
			if !stopped {
				stopped = true
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}
		t.Cleanup(stop)
		call := func(method string, params any, target any) error {
			data, err := json.Marshal(params)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			response, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "draft-test", Method: method, Params: data})
			if err != nil {
				return err
			}
			if response.Error != nil {
				return fmt.Errorf("%s", response.Error.Message)
			}
			return json.Unmarshal(response.Result, target)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			var ready any
			if call("snapshot", nil, &ready) == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("draft worker did not start")
			}
			time.Sleep(20 * time.Millisecond)
		}
		return call, stop
	}
	call, stop := start()
	verifyRetakeReplay := func() {
		t.Helper()
		var replay captureDraftJob
		if err := call("scroll.retake", map[string]any{"expectedId": previousID, "revision": 7}, &replay); err != nil {
			t.Fatal(err)
		}
		var result struct {
			DraftID string `json:"draftId"`
		}
		if replay.State != "completed" || json.Unmarshal(replay.Result, &result) != nil || result.DraftID != id {
			t.Fatal("committed retake was not recovered", replay)
		}
		var snapshot struct {
			Busy bool `json:"busy"`
		}
		if err := call("snapshot", nil, &snapshot); err != nil || snapshot.Busy {
			t.Fatal("replay started another capture", snapshot, err)
		}
	}
	verifyRetakeReplay()
	finish := func(method string, params any) captureDraftJob {
		t.Helper()
		var job captureDraftJob
		if err := call(method, params, &job); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for job.State == "running" {
			if time.Now().After(deadline) {
				t.Fatal("draft job timed out")
			}
			time.Sleep(20 * time.Millisecond)
			if err := call("jobs.get", map[string]string{"id": job.ID}, &job); err != nil {
				t.Fatal(err)
			}
		}
		return job
	}

	// A large valid scene must round-trip through the one-MiB pipe without
	// pretty-print expansion or being retained in every completed job result.
	points := make([]map[string]int, 4096)
	for i := range points {
		points[i] = map[string]int{"x": 1 + i%20, "y": 1 + i%10}
	}
	marks := make([]map[string]any, 4)
	for i := range marks {
		marks[i] = map[string]any{"kind": "pen", "points": points}
	}
	large := map[string]any{"version": 1, "width": 32, "height": 24, "marks": marks}
	job := finish("draft.update", map[string]any{"expectedId": id, "revision": 0, "document": large})
	if job.State != "completed" || len(job.Result) > 256 {
		t.Fatal("draft update retained the full scene", job.State, len(job.Result), job.Error)
	}
	var acknowledgement struct {
		DraftID  string `json:"draftId"`
		Revision int    `json:"revision"`
	}
	if json.Unmarshal(job.Result, &acknowledgement) != nil || acknowledgement.DraftID != id || acknowledgement.Revision != 1 {
		t.Fatal("draft acknowledgement did not identify the saved revision")
	}
	var reply struct {
		Draft *struct {
			ID       string `json:"id"`
			Revision int    `json:"revision"`
			Document struct {
				Marks []json.RawMessage `json:"marks"`
			} `json:"document"`
		} `json:"draft"`
	}
	if err := call("draft.get", map[string]string{"expectedId": id}, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Draft == nil || reply.Draft.Revision != 1 || len(reply.Draft.Document.Marks) != 4 {
		t.Fatal("large draft did not round-trip")
	}
	for _, mark := range reply.Draft.Document.Marks {
		var stroke struct {
			Points []struct{ X, Y int } `json:"points"`
		}
		if json.Unmarshal(mark, &stroke) != nil || len(stroke.Points) != 4096 || stroke.Points[4095].X != 16 || stroke.Points[4095].Y != 6 {
			t.Fatal("large draft lost stroke points")
		}
	}
	document := map[string]any{"version": 1, "width": 32, "height": 24, "marks": []map[string]any{{"kind": "cover", "x": 2, "y": 3, "width": 10, "height": 8, "color": "#000000"}}}
	job = finish("draft.update", map[string]any{"expectedId": id, "revision": 1, "document": document})
	if job.State != "completed" {
		t.Fatal(job)
	}
	job = finish("draft.cancel", map[string]any{"expectedId": id, "revision": 1})
	if job.State != "failed" {
		t.Fatal("stale cancellation was accepted")
	}
	stop()
	call, stop = start()
	verifyRetakeReplay()
	if err := call("draft.get", map[string]string{"expectedId": id}, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Draft == nil || reply.Draft.Revision != 2 || len(reply.Draft.Document.Marks) != 1 {
		t.Fatal("draft was lost on restart")
	}
	output := filepath.Join(directory, "completed.png")
	job = finish("draft.complete", map[string]any{"expectedId": id, "revision": 2, "path": output})
	if job.State != "completed" {
		t.Fatal(job)
	}
	if err := call("draft.get", nil, &reply); err != nil || reply.Draft != nil {
		t.Fatal("completed draft was not removed", err)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	result, err := png.Decode(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if color.RGBAModel.Convert(result.At(4, 4)) != (color.RGBA{A: 255}) {
		t.Fatal("draft annotations were not exported")
	}
	if _, err = os.Stat(previousPath); err != nil {
		t.Fatal("previous capture was lost", err)
	}
	stop()
}

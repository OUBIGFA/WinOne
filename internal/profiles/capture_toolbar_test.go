package profiles

import (
	"encoding/json"
	"testing"
)

func TestCaptureToolbarRoundTrip(t *testing.T) {
	raw := json.RawMessage(`{"schemaVersion":1,"revision":0,"outputDirectory":"C:\\Pictures","copyToClipboard":true,"askWhereToSave":false,"openEditor":false,"pinAfterCapture":false,"scrollDelay":300,"scrollFromTop":false,"toolbarTools":["mosaic","arrow"],"fileNameTemplate":"work-{date}"}`)
	exported, err := ExportBuiltinSettings("capture", raw, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := PrepareBuiltinImport("capture", exported.Module, exported.Paths, map[string]string{"capture.output": `D:\Pictures`})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ToolbarTools     []string `json:"toolbarTools"`
		FileNameTemplate string   `json:"fileNameTemplate"`
	}
	if err = json.Unmarshal(imported.Patch, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.ToolbarTools) != 2 || result.ToolbarTools[0] != "mosaic" || result.ToolbarTools[1] != "arrow" {
		t.Fatal("toolbar layout lost", result)
	}
	if result.FileNameTemplate != "work-{date}" {
		t.Fatal("filename template lost")
	}
}

func TestCaptureRecordingAndLocalKeysRemainPortable(t *testing.T) {
	raw := json.RawMessage(`{"schemaVersion":1,"revision":0,"outputDirectory":"C:\\Pictures","copyToClipboard":true,"askWhereToSave":false,"openEditor":true,"pinAfterCapture":false,"scrollDelay":300,"scrollFromTop":false,"videoAskWhereToSave":false,"gifAskWhereToSave":true,"videoOutputDirectory":"C:\\PrivateVideos","gifOutputDirectory":"","canvasShortcuts":{"number":"F8"}}`)
	exported, err := ExportBuiltinSettings("capture", raw, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := PrepareBuiltinImport("capture", exported.Module, exported.Paths, map[string]string{"capture.output": `D:\Pictures`, "capture.video": `D:\Videos`})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		VideoAskWhereToSave  bool
		GifAskWhereToSave    bool
		VideoOutputDirectory string
		GifOutputDirectory   string
		CanvasShortcuts      map[string]string
	}
	if err = json.Unmarshal(imported.Patch, &result); err != nil {
		t.Fatal(err)
	}
	if result.VideoAskWhereToSave || !result.GifAskWhereToSave || result.VideoOutputDirectory != `D:\Videos` || result.GifOutputDirectory != "" || result.CanvasShortcuts["number"] != "F8" {
		t.Fatalf("recording preferences lost: %+v", result)
	}
}

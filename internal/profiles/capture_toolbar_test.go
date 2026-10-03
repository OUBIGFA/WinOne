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

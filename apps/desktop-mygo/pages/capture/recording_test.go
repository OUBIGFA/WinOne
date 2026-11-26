package capture

import (
	"encoding/json"
	"testing"
)

func TestRecordingDefaultsAndExplicitOptOut(t *testing.T) {
	var old settings
	if err := json.Unmarshal([]byte(`{"outputDirectory":"C:/captures"}`), &old); err != nil {
		t.Fatal(err)
	}
	if !old.VideoAskWhereToSave || !old.GifAskWhereToSave {
		t.Fatal("old profile bypassed destination confirmation")
	}
	var customized settings
	if err := json.Unmarshal([]byte(`{"videoAskWhereToSave":false,"gifAskWhereToSave":true,"videoOutputDirectory":"C:/videos","gifOutputDirectory":"C:/gifs"}`), &customized); err != nil {
		t.Fatal(err)
	}
	if customized.VideoAskWhereToSave || !customized.GifAskWhereToSave || customized.VideoOutputDirectory != "C:/videos" || customized.GifOutputDirectory != "C:/gifs" {
		t.Fatal("per-format settings changed")
	}
}

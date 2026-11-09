package interconnect

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The phone probe supplies real worker output and decodes these rendered pixels.
func TestPhoneQRRendering(t *testing.T) {
	input := os.Getenv("WINONE_PHONE_QR_INPUT")
	if input == "" {
		t.Skip("run through the phone QR probe")
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var info struct{ URL, QR string }
	if err = json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	task := &peerTask{route: "lan"}
	var copied string
	tt := ui.NewTester(func(c *ui.Context) {
		task.routes(c, "probe", taskState{}, nil, nil, info.URL, false, nil, func(value string) { copied = value })
	}, 480, 420)
	tt.Frame()
	if err = tt.Click(tr("copyPhoneLink")); err != nil || copied != info.URL {
		t.Fatalf("copy link mismatch: %v", err)
	}
	file, err := os.Create(filepath.Join(filepath.Dir(input), "qr-native.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

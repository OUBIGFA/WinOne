package workspace

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"encoding/json"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/testhost"
)

// Explicit native entry only: use Windows' real owned media process. Playback
// is muted before it starts, so this does not change the user's audio output.
func TestRealWindowsMediaPlaybackSeekRateMuteFrameAndRelease(t *testing.T) {
	worker := os.Getenv("WINONE_MEDIA_WORKER")
	if worker == "" {
		t.Skip("explicit repository-built media worker required")
	}
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(36+16000*2*3))
	wav.WriteString("WAVEfmt ")
	for _, value := range []any{uint32(16), uint16(1), uint16(1), uint32(16000), uint32(32000), uint16(2), uint16(16)} {
		_ = binary.Write(&wav, binary.LittleEndian, value)
	}
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(16000*2*3))
	wav.Write(make([]byte, 16000*2*3))
	for _, kind := range []string{"audio", "video"} {
		t.Run(kind, func(t *testing.T) {
			data, name := wav.Bytes(), "silent.wav"
			if kind == "video" {
				path := os.Getenv("WINONE_MEDIA_TEST_VIDEO")
				if path == "" {
					t.Skip("explicit task-local encoded video required")
				}
				var err error
				data, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				name = "silent.mp4"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
			}))
			defer server.Close()
			// This is the production Panel's default file path: it deliberately
			// supplies no Media override. Opening must still use the real helper.
			h := testhost.New(t, func(string, json.RawMessage) (any, error) {
				return FilePreview{Kind: kind, Name: name, URL: server.URL + "/" + name}, nil
			})
			document := NewDocumentPane(h.Env, DocumentOptions{Prefix: "chat.", SessionID: "owned", Path: name})
			tt := ui.NewTester(func(*ui.Context) {}, 600, 400)
			h.Settle(tt)
			player, ok := document.body.(*MediaPane)
			if !ok {
				document.Close()
				t.Fatalf("default native file path did not create a player: %s", document.err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for !player.state.Opened && player.error == "" && time.Now().Before(deadline) {
				time.Sleep(30 * time.Millisecond)
				h.Settle(tt)
			}
			if !player.state.Opened || player.error != "" {
				document.Close()
				t.Fatalf("default document did not decode: %s", player.error)
			}
			document.SetVisible(false)
			h.Settle(tt)
			if player.visible || player.state.Playing {
				t.Fatal("hidden default media remained active")
			}
			document.Close()
			h.Settle(tt)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			client, err := startMediaClient(ctx, worker)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			call := func(method string, values map[string]any) mediaState {
				t.Helper()
				state, err := client.call(ctx, method, values)
				if err != nil {
					t.Fatal(err)
				}
				return state
			}
			call("open", map[string]any{"url": server.URL + "/" + name})
			state := call("mute", map[string]any{"muted": true})
			deadline = time.Now().Add(10 * time.Second)
			for !state.Opened && time.Now().Before(deadline) {
				time.Sleep(30 * time.Millisecond)
				state = call("status", nil)
			}
			if !state.Opened || state.Duration < 2 || !state.Muted {
				t.Fatalf("media never decoded: %+v", state)
			}
			state = call("rate", map[string]any{"rate": 1.5})
			if state.Rate != 1.5 {
				t.Fatal("rate did not reach the real decoder")
			}
			state = call("volume", map[string]any{"volume": 0.25})
			if state.Volume != 0.25 {
				t.Fatal("volume did not reach the real decoder")
			}
			call("seek", map[string]any{"position": 1.0})
			state = call("play", nil)
			if !state.Playing {
				t.Fatal("play was not accepted")
			}
			time.Sleep(150 * time.Millisecond)
			state = call("pause", nil)
			if state.Playing || state.Position < 0.9 {
				t.Fatalf("seek/play/pause failed: %+v", state)
			}
			if kind == "video" {
				state = call("status", map[string]any{"frame": true, "width": 64, "height": 48})
				if state.VideoWidth != 64 || state.VideoHeight != 48 || state.Frame == "" {
					t.Fatalf("no decoded video frame: %+v", state)
				}
			}
			call("close", nil)
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
			if client.command.ProcessState == nil || !client.command.ProcessState.Exited() {
				t.Fatal("owned decoder did not exit")
			}
		})
	}
}

package resultui

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

func TestSpeechChunksPreserveAllUnicodeAndEscapedSSML(t *testing.T) {
	text := strings.Repeat("中文🙂 & <tag> \" '。 English text. ", 1200)
	chunks := speechChunks(text, speechChunkBytes)
	if len(chunks) < 2 || strings.Join(chunks, "") != text {
		t.Fatal("long text truncated or changed")
	}
	for _, chunk := range chunks {
		if !utf8.ValidString(chunk) || len(chunk) > speechChunkBytes {
			t.Fatal("invalid UTF-8 chunk or limit")
		}
		decoder := xml.NewDecoder(strings.NewReader(speechSSML(chunk)))
		var spoken strings.Builder
		voices := 0
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if start, ok := token.(xml.StartElement); ok && start.Name.Local == "voice" {
				voices++
			}
			if value, ok := token.(xml.CharData); ok {
				spoken.Write(value)
			}
		}
		if voices != 1 || spoken.String() != chunk {
			t.Fatal("text escaped incorrectly or injected SSML")
		}
	}
}

func speechTestFrame(payload []byte) []byte {
	header := []byte("Path:audio\r\nContent-Type:audio/mpeg\r\n")
	frame := make([]byte, 2+len(header))
	binary.BigEndian.PutUint16(frame, uint16(len(header)))
	copy(frame[2:], header)
	return append(frame, payload...)
}

func TestEdgeSpeechProtocolAndFailures(t *testing.T) {
	for _, mode := range []string{"complete", "truncated", "empty", "malformed", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			received := make(chan string, 1)
			released := make(chan struct{})
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(released)
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				_, config, err := conn.ReadMessage()
				if err != nil {
					return
				}
				_, ssml, err := conn.ReadMessage()
				if err != nil {
					return
				}
				received <- string(config) + "\n" + string(ssml)
				if mode == "cancel" {
					_, _, _ = conn.ReadMessage()
					return
				}
				if mode == "malformed" {
					_ = conn.WriteMessage(websocket.BinaryMessage, []byte{0, 42})
					return
				}
				if mode != "empty" {
					_ = conn.WriteMessage(websocket.BinaryMessage, speechTestFrame([]byte("MP3 fixture")))
				}
				if mode != "truncated" {
					_ = conn.WriteMessage(websocket.TextMessage, []byte("Path:turn.end\r\n\r\n{}"))
				}
			}))
			defer server.Close()
			s := &edgeSynthesizer{endpoint: "ws" + strings.TrimPrefix(server.URL, "http")}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			type outcome struct {
				audio []byte
				err   error
			}
			done := make(chan outcome, 1)
			go func() { audio, err := s.synthesize(ctx, "Text <voice> & 中文"); done <- outcome{audio, err} }()
			select {
			case request := <-received:
				if !strings.Contains(request, "audio-24khz-48kbitrate-mono-mp3") || !strings.Contains(request, "Text &lt;voice&gt; &amp; 中文") {
					t.Fatal("wrong synthesis request")
				}
			case <-ctx.Done():
				t.Fatal("server never received synthesis")
			}
			if mode == "cancel" {
				cancel()
			}
			result := <-done
			if mode == "complete" {
				if result.err != nil || string(result.audio) != "MP3 fixture" {
					t.Fatal(result.err, string(result.audio))
				}
			} else if result.err == nil || len(result.audio) != 0 {
				t.Fatal("incomplete synthesis was accepted")
			}
			if mode == "cancel" && !errors.Is(result.err, context.Canceled) {
				t.Fatal(result.err)
			}
			select {
			case <-released:
			case <-time.After(time.Second):
				t.Fatal("cancellation retained WebSocket")
			}
		})
	}
}

func TestEdgeSpeechCorrectsClockFromRejectedHandshake(t *testing.T) {
	serverTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 2 && r.URL.Query().Get("Sec-MS-GEC") != speechGEC(serverTime) {
			t.Error("retry did not use server clock")
		}
		w.Header().Set("Date", serverTime.Format(http.TimeFormat))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	s := &edgeSynthesizer{endpoint: "ws" + strings.TrimPrefix(server.URL, "http")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := s.synthesize(ctx, "public fixture"); err == nil || attempts != 2 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}

// Explicit opt-in: sends only these public fixtures to Microsoft. Optional
// playback emits sound through Windows and verifies completion/cancel/cleanup.
func TestEdgeSpeechOnlineAndWindowsPlayback(t *testing.T) {
	if os.Getenv("WINONE_TEST_SPEECH_ONLINE") != "1" {
		t.Skip("online speech is opt-in")
	}
	t.Setenv("TMP", t.TempDir())
	t.Setenv("TEMP", os.Getenv("TMP"))
	for _, text := range []string{"你好，这是朗读测试。", "Hello. This is a reading test."} {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		audio, err := edgeSpeech.synthesize(ctx, text)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if len(audio) < 1000 || !bytes.HasPrefix(audio, []byte("ID3")) && !(audio[0] == 0xff && audio[1]&0xe0 == 0xe0) {
			t.Fatalf("invalid MP3 (%d bytes)", len(audio))
		}
		t.Logf("Edge synthesized %q: %d MP3 bytes", text, len(audio))
		if os.Getenv("WINONE_TEST_SPEECH_PLAYBACK") == "1" {
			ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
			err = playSpeech(ctx, audio)
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel = context.WithCancel(context.Background())
			time.AfterFunc(200*time.Millisecond, cancel)
			started := time.Now()
			err = playSpeech(ctx, audio)
			cancel()
			if !errors.Is(err, context.Canceled) || time.Since(started) > 2*time.Second {
				t.Fatalf("playback did not cancel promptly: %v", err)
			}
			files, err := filepath.Glob(filepath.Join(os.TempDir(), "winone-speech-*.mp3"))
			if err != nil || len(files) > 0 {
				t.Fatal("playback retained temporary audio", files, err)
			}
		}
	}
}

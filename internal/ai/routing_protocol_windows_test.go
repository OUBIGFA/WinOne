//go:build windows

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Use actual protocol handlers and their vendor boundary: a successful compile
// alone cannot show whether multipart, binary replies or native paths survive.
func TestRoutingAudioNativeAndDiscoveryProtocols(t *testing.T) {
	exe := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated AI bundle")
	}
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer vendor-fixture" {
			t.Error("incorrect vendor credential")
		}
		switch r.URL.Path {
		case "/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write([]byte{0xff, 0xfb, 0x00, 0x01})
		case "/audio/transcriptions", "/audio/translations":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			if r.FormValue("model") != "whisper-1" {
				t.Errorf("multipart model: %s", r.FormValue("model"))
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"text":"Hello world"}`)
		case "/v2/jobs/fixture":
			if r.Method != http.MethodPatch || r.URL.Query().Get("mode") != "detail" {
				t.Error("native method/query lost")
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"native":true}`)
		default:
			t.Errorf("unexpected vendor path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer vendor.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tr := Translator{Executable: exe, DataDirectory: filepath.Join(t.TempDir(), "runtime")}
	providers := []RuntimeProvider{{Provider: Provider{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"tts-1", "whisper-1"}}, Key: "vendor-fixture"}}
	router, err := tr.StartRouting(ctx, &MagpieSettings{NoStats: true, NoUsageStats: true}, providers)
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	call := func(method, path, ct string, body []byte, models []string) (int, []byte) {
		t.Helper()
		r := httptest.NewRequest(method, "http://fixture"+path, bytes.NewReader(body)).WithContext(ctx)
		r.Header.Set("Content-Type", ct)
		w := httptest.NewRecorder()
		if models == nil {
			router.ServeHTTP(w, r)
		} else {
			router.ServeMedia(w, r, models)
		}
		return w.Code, w.Body.Bytes()
	}
	for _, path := range []string{"/api/hello", "/muse-code/models", "/v1/models/fixture/tts-1"} {
		code, body := call("GET", path, "", nil, []string{"fixture/tts-1", "fixture/whisper-1"})
		if code != 200 {
			t.Fatalf("%s: %d %s", path, code, body)
		}
	}
	code, body := call("POST", "/v1/audio/speech", "application/json", []byte(`{"model":"fixture/tts-1","input":"Hello","voice":"alloy"}`), []string{"fixture/tts-1"})
	if code != 200 || !bytes.Equal(body, []byte{0xff, 0xfb, 0, 1}) {
		t.Fatalf("speech: %d %q", code, body)
	}
	for _, path := range []string{"/v1/audio/transcriptions", "/v1/audio/translations"} {
		var form bytes.Buffer
		mw := multipart.NewWriter(&form)
		mw.WriteField("model", "fixture/whisper-1")
		f, _ := mw.CreateFormFile("file", "speech.wav")
		f.Write([]byte("RIFF fixture audio"))
		mw.Close()
		code, body = call("POST", path, mw.FormDataContentType(), form.Bytes(), []string{"fixture/whisper-1"})
		if code != 200 || !strings.Contains(string(body), "Hello world") {
			t.Fatalf("%s: %d %s", path, code, body)
		}
	}
	code, body = call("PATCH", "/v1/magpie/native/fixture/v2/jobs/fixture?mode=detail", "application/json", []byte(`{"status":"ready"}`), nil)
	if code != 200 || !json.Valid(body) {
		t.Fatalf("native: %d %s", code, body)
	}
	code, _ = call("PATCH", "/v1/magpie/native/fixture/v2/jobs/fixture?mode=detail", "application/json", []byte(`{}`), []string{"fixture/tts-1"})
	if code != 403 {
		t.Fatalf("model-restricted native request: %d", code)
	}
}

package learning

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

func TestPronunciationCachesWithoutAnotherRequest(t *testing.T) {
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("type") != "1" || r.URL.Query().Get("audio") != "take care" {
			t.Error(r.URL.String())
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte("ID3test-audio"))
	}))
	defer server.Close()
	source := Online{VoiceURL: server.URL, CacheDirectory: t.TempDir()}
	for i := 0; i < 2; i++ {
		data, err := source.Pronunciation(context.Background(), "take care", "uk")
		if err != nil || len(data) == 0 {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("requests=%d", calls.Load())
	}
}
func TestOnlineDictionaryRejectsHTTPFailure(t *testing.T) {
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "not found", 404) }))
	defer server.Close()
	if _, err := (Online{DictionaryURL: server.URL}).Lookup(context.Background(), "invented-word"); err == nil {
		t.Fatal("failure hidden")
	}
}

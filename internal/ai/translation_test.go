package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGooglePreservesFullTextAndDecodesOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body) != 2 {
			t.Fatal("wrong provider payload")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[["你好\n&lt;world&gt; &amp;lt;"]]`))
	}))
	defer server.Close()
	got, err := translateGoogle(context.Background(), server.Client(), server.URL, "Hello\n<world>", "zh-CN")
	if err != nil || got != "你好\n<world> &lt;" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestSmartTarget(t *testing.T) {
	for _, test := range []struct{ text, want string }{{"你好，WinOne", "en"}, {"Hello world", "zh-CN"}, {"こんにちは世界", "zh-CN"}} {
		if got := targetLanguage("auto", test.text); got != test.want {
			t.Fatalf("%s -> %s", test.text, got)
		}
	}
}

func TestInvalidTranslationDoesNotBecomeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[[]]`)) }))
	defer server.Close()
	if _, err := translateGoogle(context.Background(), server.Client(), server.URL, "Hello", "zh-CN"); err == nil {
		t.Fatal("empty result accepted")
	}
}

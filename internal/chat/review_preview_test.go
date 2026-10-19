package chat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"winone/internal/ai"
)

func TestReviewPreviewCarriesLargeTextAndRechecksItsRevision(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", "", "Review", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := NewPreviewServer(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	content := strings.Repeat("+review line\n", 100000)
	var stale atomic.Bool
	address, err := server.OpenContent(conversation.ID, "review.diff", "text/plain; charset=utf-8", func(context.Context) ([]byte, error) {
		if stale.Load() {
			return nil, errors.New("revision changed")
		}
		return []byte(content), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || string(body) != content {
		t.Fatalf("large diff lost: %d, %d bytes", response.StatusCode, len(body))
	}
	if response.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("generated content can be interpreted as active markup")
	}
	stale.Store(true)
	response, err = http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatal("stale content was served", response.StatusCode)
	}
	for _, invalid := range []string{address + "/other", strings.Replace(address, "review.diff", "other", 1)} {
		response, err = http.Get(invalid)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 404 {
			t.Fatal("grant exposed another path", response.StatusCode)
		}
	}
}
